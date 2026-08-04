// Finding the audio m-line's mid must not depend on receiver.track, which is
// not reliably populated right after setRemoteDescription. When it was null the
// viewer adopted nothing, and the microphone was later attached to nothing —
// silent, with every visible sign of working.
import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/webrtc_service.dart';

void main() {
  test('finds the mid of the audio section, not the video one', () {
    const sdp = 'v=0\r\n'
        'm=video 9 UDP/TLS/RTP/SAVPF 96\r\n'
        'a=mid:0\r\n'
        'a=sendonly\r\n'
        'm=audio 9 UDP/TLS/RTP/SAVPF 0\r\n'
        'a=mid:1\r\n'
        'a=sendrecv\r\n';
    expect(WebRTCService.debugAudioMidOf(sdp), '1');
  });

  test('returns null when the offer carries no audio at all', () {
    // A host on an older build offers video only; the viewer must not claim a
    // voice channel that does not exist.
    const sdp = 'v=0\r\n'
        'm=video 9 UDP/TLS/RTP/SAVPF 96\r\n'
        'a=mid:0\r\n';
    expect(WebRTCService.debugAudioMidOf(sdp), isNull);
  });

  test('handles bare newlines as well as CRLF', () {
    const sdp = 'v=0\nm=audio 9 UDP/TLS/RTP/SAVPF 0\na=mid:aud\n';
    expect(WebRTCService.debugAudioMidOf(sdp), 'aud');
  });

  test('does not mistake a later video mid for the audio one', () {
    const sdp = 'v=0\r\n'
        'm=audio 9 UDP/TLS/RTP/SAVPF 0\r\n'
        'a=mid:7\r\n'
        'm=video 9 UDP/TLS/RTP/SAVPF 96\r\n'
        'a=mid:8\r\n';
    expect(WebRTCService.debugAudioMidOf(sdp), '7');
  });
}
