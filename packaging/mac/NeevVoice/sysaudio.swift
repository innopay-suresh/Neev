// System-audio capture on macOS, via ScreenCaptureKit.
//
// This does NOT need a virtual audio device (BlackHole and friends). macOS 13
// added audio capture to SCStream, so the host's system sound can be shared the
// same way Windows does it with WASAPI loopback.
//
// It runs HERE, in the helper, rather than in the Go worker: ScreenCaptureKit is
// an Objective-C/Swift framework, and TCC attributes capture to the bundle that
// asks — which this is, and a bare Go binary is not.

import AVFoundation
import ScreenCaptureKit

@available(macOS 13.0, *)
final class SystemAudioTap: NSObject, SCStreamOutput, SCStreamDelegate {
    private var stream: SCStream?
    private let onFrame: ([UInt8]) -> Void
    private let queue = DispatchQueue(label: "com.neev.remote.sysaudio")

    /// Capture runs at 48 kHz and the wire format is 8 kHz, so every 6 input
    /// samples become one output sample.
    private static let decim = 6

    init(onFrame: @escaping ([UInt8]) -> Void) {
        self.onFrame = onFrame
        super.init()
    }

    func start() async throws {
        let content = try await SCShareableContent.excludingDesktopWindows(
            false, onScreenWindowsOnly: false)
        guard let display = content.displays.first else {
            throw NSError(domain: "NeevVoice", code: 1, userInfo:
                [NSLocalizedDescriptionKey: "no display available for audio capture"])
        }

        let cfg = SCStreamConfiguration()
        cfg.capturesAudio = true
        cfg.sampleRate = 48000
        cfg.channelCount = 1
        // Video is not wanted, but SCStream always produces it. Ask for the
        // smallest, slowest frames the API allows so screen capture for AUDIO
        // costs almost nothing — the real screen share is the Go worker's job.
        cfg.width = 2
        cfg.height = 2
        cfg.minimumFrameInterval = CMTime(value: 1, timescale: 1)
        cfg.showsCursor = false

        let filter = SCContentFilter(display: display, excludingWindows: [])
        let s = SCStream(filter: filter, configuration: cfg, delegate: self)
        try s.addStreamOutput(self, type: .audio, sampleHandlerQueue: queue)
        try await s.startCapture()
        stream = s
    }

    func stop() async {
        guard let s = stream else { return }
        stream = nil
        try? await s.stopCapture()
    }

    // MARK: - SCStreamOutput

    func stream(_ stream: SCStream, didOutputSampleBuffer sb: CMSampleBuffer,
                of type: SCStreamOutputType) {
        guard type == .audio, CMSampleBufferIsValid(sb) else { return }
        guard let samples = Self.floatSamples(from: sb) else { return }

        // Average each group of 6 rather than taking every 6th. Plain decimation
        // folds everything above 4 kHz back into the audible band as aliasing,
        // which is heard as a metallic warble on music and speech alike.
        var out = [UInt8]()
        out.reserveCapacity(samples.count / Self.decim + 1)
        var i = 0
        while i + Self.decim <= samples.count {
            var acc: Float = 0
            for j in 0..<Self.decim { acc += samples[i + j] }
            let avg = acc / Float(Self.decim)
            out.append(Self.muLaw(Int16(max(-32768, min(32767, avg * 32767)))))
            i += Self.decim
        }
        if !out.isEmpty { onFrame(out) }
    }

    func stream(_ stream: SCStream, didStopWithError error: Error) {
        NSLog("NeevVoice: system audio stopped: \(error.localizedDescription)")
    }

    // MARK: - helpers

    /// Pulls mono Float32 samples out of a CMSampleBuffer.
    private static func floatSamples(from sb: CMSampleBuffer) -> [Float]? {
        var blockBuffer: CMBlockBuffer?
        var abl = AudioBufferList()
        let status = CMSampleBufferGetAudioBufferListWithRetainedBlockBuffer(
            sb,
            bufferListSizeNeededOut: nil,
            bufferListOut: &abl,
            bufferListSize: MemoryLayout<AudioBufferList>.size,
            blockBufferAllocator: nil,
            blockBufferMemoryAllocator: nil,
            flags: 0,
            blockBufferOut: &blockBuffer)
        guard status == noErr, let data = abl.mBuffers.mData else { return nil }
        let count = Int(abl.mBuffers.mDataByteSize) / MemoryLayout<Float>.size
        guard count > 0 else { return nil }
        let ptr = data.bindMemory(to: Float.self, capacity: count)
        return Array(UnsafeBufferPointer(start: ptr, count: count))
    }

    /// G.711 mu-law, matching agent/audio/pcmu.go byte for byte — the Go side
    /// decodes these, so the two implementations must agree exactly.
    static func muLaw(_ sample: Int16) -> UInt8 {
        let bias = 0x84, clip = 32635
        var s = Int(sample)
        var sign: UInt8 = 0
        if s < 0 { s = -s; sign = 0x80 }
        if s > clip { s = clip }
        s += bias
        var exponent = 7
        var mask = 0x4000
        while exponent > 0 && (s & mask) == 0 { exponent -= 1; mask >>= 1 }
        let mantissa = (s >> (exponent + 3)) & 0x0F
        return ~(sign | UInt8(exponent << 4) | UInt8(mantissa))
    }
}
