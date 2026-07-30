package session

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// setupFileLog tees the global zerolog logger to
// C:\ProgramData\NeevRemote\<name> (plus stderr) so the seamless backend is
// observable when the SYSTEM service launches it with CREATE_NO_WINDOW — stderr
// alone is discarded there, which left TransportMode undiagnosable in the field.
// Best-effort: on any error it leaves the existing (stderr) logger in place.
func setupFileLog(name string) {
	dir := dataDir()
	path := filepath.Join(dir, name)
	// Roll if it grew past ~4 MB so it can't fill the disk over long uptimes.
	if fi, err := os.Stat(path); err == nil && fi.Size() > 4*1024*1024 {
		_ = os.Truncate(path, 0)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		// The shared ProgramData file is created by whichever USER's worker ran
		// first; after a profile switch the second account often cannot open it
		// (inherited ACL), and the new session then runs completely unlogged —
		// exactly what made the post-switch clipboard reports undiagnosable.
		// Fall back to a per-user file so every session is always observable.
		alt := filepath.Join(os.TempDir(), "NeevRemote-"+name)
		f, err = os.OpenFile(alt, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return
		}
		path = alt
	}
	fileW := zerolog.ConsoleWriter{Out: f, TimeFormat: time.RFC3339, NoColor: true}
	stderrW := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}
	log.Logger = zerolog.New(io.MultiWriter(fileW, stderrW)).
		With().Timestamp().Logger()
	log.Info().Str("log", path).Msg("file logging started")
}
