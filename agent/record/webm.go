// Package record writes a session's video to a file the host can keep.
//
// The frames are already VP8 — the capture worker encodes them and the
// transport forwards them to viewers — so recording is a MUX, not a re-encode.
// That is the whole reason this is cheap enough to run during a live session:
// no second encoder, no extra CPU competing with the one keeping the stream
// smooth.
//
// WebM (a Matroska subset) because VP8 lives in it natively and it opens in a
// browser, VLC, and QuickTime-with-plugins without conversion. A raw stream
// dump would be smaller to write and useless to double-click.
package record

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"sync"
	"time"
)

// EBML element IDs used here.
const (
	idEBML            = 0x1A45DFA3
	idEBMLVersion     = 0x4286
	idEBMLReadVersion = 0x42F7
	idEBMLMaxIDLength = 0x42F2
	idEBMLMaxSizeLen  = 0x42F3
	idDocType         = 0x4282
	idDocTypeVersion  = 0x4287
	idDocTypeReadVer  = 0x4285

	idSegment       = 0x18538067
	idInfo          = 0x1549A966
	idTimecodeScale = 0x2AD7B1
	idMuxingApp     = 0x4D80
	idWritingApp    = 0x5741
	idDuration      = 0x4489

	idTracks       = 0x1654AE6B
	idTrackEntry   = 0xAE
	idTrackNumber  = 0xD7
	idTrackUID     = 0x73C5
	idTrackType    = 0x83
	idCodecID      = 0x86
	idVideo        = 0xE0
	idPixelWidth   = 0xB0
	idPixelHeight  = 0xBA
	idFlagLacing   = 0x9C
	idDefaultDurat = 0x23E383

	idCluster     = 0x1F43B675
	idTimecode    = 0xE7
	idSimpleBlock = 0xA3
)

// clusterSpan bounds how much video goes in one cluster. Matroska block
// timecodes are 16-bit signed relative to the cluster, so a long cluster would
// overflow and scramble playback timing.
const clusterSpan = 2 * time.Second

// Recorder muxes VP8 frames into a WebM file.
//
// Safe for concurrent Write calls: frames arrive from the transport's pump
// while Stop may be called from a control message.
type Recorder struct {
	mu      sync.Mutex
	f       *os.File
	path    string
	started time.Time

	clusterOpen  bool
	clusterStart time.Duration
	lastTS       time.Duration
	closed       bool
	frames       int
}

// Path is where the recording is being written.
func (r *Recorder) Path() string { return r.path }

// Frames is how many video frames have been recorded so far.
func (r *Recorder) Frames() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.frames
}

// New starts a recording at path for a stream of the given size.
func New(path string, width, height int) (*Recorder, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create recording: %w", err)
	}
	r := &Recorder{f: f, path: path, started: time.Now()}
	if err := r.writeHeader(width, height); err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}
	return r, nil
}

func (r *Recorder) writeHeader(width, height int) error {
	hdr := elem(idEBML,
		concat(
			uintElem(idEBMLVersion, 1),
			uintElem(idEBMLReadVersion, 1),
			uintElem(idEBMLMaxIDLength, 4),
			uintElem(idEBMLMaxSizeLen, 8),
			strElem(idDocType, "webm"),
			uintElem(idDocTypeVersion, 2),
			uintElem(idDocTypeReadVer, 2),
		))
	if _, err := r.f.Write(hdr); err != nil {
		return err
	}

	// Segment with UNKNOWN size. The length is not known while recording, and a
	// session that ends in a crash or a power cut still has to leave a playable
	// file — an unknown-size segment plays to wherever the data stops.
	if _, err := r.f.Write(append(encodeID(idSegment), unknownSize()...)); err != nil {
		return err
	}

	info := elem(idInfo, concat(
		// 1 ms ticks: matches how frame times are measured here.
		uintElem(idTimecodeScale, 1000000),
		strElem(idMuxingApp, "Neev Remote"),
		strElem(idWritingApp, "Neev Remote"),
	))
	if _, err := r.f.Write(info); err != nil {
		return err
	}

	video := elem(idVideo, concat(
		uintElem(idPixelWidth, uint64(width)),
		uintElem(idPixelHeight, uint64(height)),
	))
	track := elem(idTrackEntry, concat(
		uintElem(idTrackNumber, 1),
		uintElem(idTrackUID, 1),
		uintElem(idTrackType, 1), // video
		uintElem(idFlagLacing, 0),
		strElem(idCodecID, "V_VP8"),
		video,
	))
	_, err := r.f.Write(elem(idTracks, track))
	return err
}

// Write appends one VP8 frame. keyframe marks a frame that can be decoded on
// its own.
func (r *Recorder) Write(vp8 []byte, keyframe bool) error {
	if len(vp8) == 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}

	ts := time.Since(r.started)
	// A new cluster on each keyframe (and whenever the span is exceeded) so
	// seeking lands somewhere decodable rather than mid-GOP.
	if !r.clusterOpen || (keyframe && ts-r.clusterStart >= clusterSpan) ||
		ts-r.clusterStart >= clusterSpan {
		if err := r.startClusterLocked(ts); err != nil {
			return err
		}
	}

	rel := (ts - r.clusterStart).Milliseconds()
	// Guard the 16-bit signed block timecode: rather than write a value that
	// wraps and scrambles playback order, open a fresh cluster.
	if rel > math.MaxInt16 {
		if err := r.startClusterLocked(ts); err != nil {
			return err
		}
		rel = 0
	}

	var flags byte
	if keyframe {
		flags = 0x80
	}
	body := make([]byte, 0, len(vp8)+4)
	body = append(body, 0x81) // track number 1, as a 1-byte vint
	body = append(body, byte(rel>>8), byte(rel))
	body = append(body, flags)
	body = append(body, vp8...)

	if _, err := r.f.Write(elem(idSimpleBlock, body)); err != nil {
		return err
	}
	r.frames++
	r.lastTS = ts
	return nil
}

func (r *Recorder) startClusterLocked(ts time.Duration) error {
	r.clusterStart = ts
	r.clusterOpen = true
	// Unknown-size cluster, for the same reason as the segment: a recording cut
	// short by a crash stays playable.
	hdr := append(encodeID(idCluster), unknownSize()...)
	hdr = append(hdr, uintElem(idTimecode, uint64(ts.Milliseconds()))...)
	_, err := r.f.Write(hdr)
	return err
}

// Stop finishes the recording and returns its path.
func (r *Recorder) Stop() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r.path, nil
	}
	r.closed = true
	err := r.f.Close()
	return r.path, err
}

// ---- EBML primitives ----

func encodeID(id uint32) []byte {
	switch {
	case id > 0x00FFFFFF:
		b := make([]byte, 4)
		binary.BigEndian.PutUint32(b, id)
		return b
	case id > 0x0000FFFF:
		return []byte{byte(id >> 16), byte(id >> 8), byte(id)}
	case id > 0x000000FF:
		return []byte{byte(id >> 8), byte(id)}
	default:
		return []byte{byte(id)}
	}
}

// encodeSize writes a length as an EBML variable-size integer.
func encodeSize(n uint64) []byte {
	switch {
	case n < 1<<7-1:
		return []byte{0x80 | byte(n)}
	case n < 1<<14-1:
		return []byte{0x40 | byte(n>>8), byte(n)}
	case n < 1<<21-1:
		return []byte{0x20 | byte(n>>16), byte(n >> 8), byte(n)}
	case n < 1<<28-1:
		return []byte{0x10 | byte(n>>24), byte(n >> 16), byte(n >> 8), byte(n)}
	default:
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, n)
		b[0] = 0x01
		return b
	}
}

// unknownSize is the all-ones vint meaning "length not known".
func unknownSize() []byte { return []byte{0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF} }

func elem(id uint32, body []byte) []byte {
	out := encodeID(id)
	out = append(out, encodeSize(uint64(len(body)))...)
	return append(out, body...)
}

func uintElem(id uint32, v uint64) []byte {
	var b []byte
	if v == 0 {
		b = []byte{0}
	} else {
		full := make([]byte, 8)
		binary.BigEndian.PutUint64(full, v)
		i := 0
		for i < 7 && full[i] == 0 {
			i++
		}
		b = full[i:]
	}
	return elem(id, b)
}

func strElem(id uint32, s string) []byte { return elem(id, []byte(s)) }

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

var _ io.Writer = (*os.File)(nil)
