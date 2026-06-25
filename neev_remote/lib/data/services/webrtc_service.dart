import 'dart:async';

import 'package:flutter_webrtc/flutter_webrtc.dart';

/// Lightweight stats snapshot surfaced to the UI.
class SessionStats {
  final int? bitrateKbps;
  final int? fps;
  final int? latencyMs;

  const SessionStats({this.bitrateKbps, this.fps, this.latencyMs});
}

/// Wraps a single `RTCPeerConnection`, abstracting the offerer (host) and
/// answerer (viewer) flows used by [RemoteService].
///
/// Responsibilities handled here that the previous version got wrong:
///  * ICE candidates that arrive before the remote description is applied are
///    queued and flushed afterwards (otherwise `addCandidate` throws).
///  * The offerer (host) owns the `control` data channel; the answerer
///    receives it via `onDataChannel`.
///  * `getStats` is parsed into a [SessionStats] for the status bar.
class WebRTCService {
  RTCPeerConnection? _pc;
  RTCDataChannel? _dataChannel;
  MediaStream? _remoteStream;

  bool _remoteDescriptionSet = false;
  final List<RTCIceCandidate> _pendingCandidates = [];

  // For bitrate delta calculation.
  int _lastBytes = 0;
  DateTime? _lastStatsAt;

  // Callbacks wired by the orchestrator.
  void Function(MediaStream stream)? onRemoteStream;
  void Function(RTCIceCandidate candidate)? onIceCandidate;
  void Function(String message)? onDataMessage;
  void Function(RTCPeerConnectionState state)? onConnectionStateChange;
  void Function()? onDataChannelOpen;

  RTCPeerConnection? get peerConnection => _pc;
  MediaStream? get remoteStream => _remoteStream;
  bool get isDataChannelOpen =>
      _dataChannel?.state == RTCDataChannelState.RTCDataChannelOpen;

  /// Creates the peer connection. The host passes [isOfferer] = true so it
  /// owns the control data channel.
  Future<void> initialize({
    required List<Map<String, dynamic>> iceServers,
    required bool isOfferer,
  }) async {
    final config = <String, dynamic>{
      'sdpSemantics': 'unified-plan',
      'iceServers': iceServers,
    };

    _pc = await createPeerConnection(config);

    _pc!.onIceCandidate = (candidate) {
      if (candidate.candidate != null) onIceCandidate?.call(candidate);
    };

    _pc!.onConnectionState = (state) => onConnectionStateChange?.call(state);

    _pc!.onTrack = (event) {
      if (event.streams.isNotEmpty) {
        _remoteStream = event.streams.first;
        onRemoteStream?.call(_remoteStream!);
      }
    };

    if (isOfferer) {
      final init = RTCDataChannelInit()
        ..ordered = true
        ..id = 1;
      _dataChannel = await _pc!.createDataChannel('control', init);
      _bindDataChannel(_dataChannel!);
    } else {
      _pc!.onDataChannel = (channel) {
        _dataChannel = channel;
        _bindDataChannel(channel);
      };
    }
  }

  void _bindDataChannel(RTCDataChannel channel) {
    channel.onMessage = (msg) {
      if (!msg.isBinary) onDataMessage?.call(msg.text);
    };
    channel.onDataChannelState = (state) {
      if (state == RTCDataChannelState.RTCDataChannelOpen) {
        onDataChannelOpen?.call();
      }
    };
    // The channel may already be open by the time we bind (offerer side).
    if (channel.state == RTCDataChannelState.RTCDataChannelOpen) {
      onDataChannelOpen?.call();
    }
  }

  /// Adds the captured screen stream (host side) to the connection, then
  /// restricts the video transceiver to VP8 so the generated offer is VP8-only.
  Future<void> addLocalStream(MediaStream stream) async {
    for (final track in stream.getTracks()) {
      await _pc!.addTrack(track, stream);
    }
    await _forceVp8();
  }

  Future<RTCSessionDescription> createOffer() async {
    final offer = await _pc!.createOffer();
    await _pc!.setLocalDescription(offer);
    return offer;
  }

  Future<RTCSessionDescription> createAnswer() async {
    final answer = await _pc!.createAnswer();
    await _pc!.setLocalDescription(answer);
    return answer;
  }

  /// Restricts the sending video transceiver to VP8 (+ RTX/FEC) via the proper
  /// `setCodecPreferences` API on the host (offerer). This makes the OFFER
  /// VP8-only, so the answerer is forced to VP8 — fixing Windows→Windows blank
  /// video (H.264 negotiated but undecodable) — WITHOUT rewriting any SDP.
  /// SDP munging the answer broke the data channel (all input) on Windows, so
  /// this codec-preference approach is used instead. Best-effort: on failure it
  /// falls back to default negotiation.
  Future<void> _forceVp8() async {
    try {
      final caps = await getRtpSenderCapabilities('video');
      final codecs = caps.codecs ?? const <RTCRtpCodecCapability>[];
      bool keep(RTCRtpCodecCapability c) {
        final m = c.mimeType.toLowerCase();
        return m == 'video/vp8' ||
            m == 'video/rtx' ||
            m == 'video/red' ||
            m == 'video/ulpfec' ||
            m == 'video/flexfec-03';
      }

      final preferred = codecs.where(keep).toList();
      final hasVp8 =
          preferred.any((c) => c.mimeType.toLowerCase() == 'video/vp8');
      if (!hasVp8) return; // don't strip everything if VP8 isn't available

      for (final t in await _pc!.getTransceivers()) {
        if (t.sender.track?.kind == 'video') {
          await t.setCodecPreferences(preferred);
        }
      }
    } catch (_) {
      // Best-effort; leaves default codec negotiation in place.
    }
  }

  Future<void> setRemoteDescription(RTCSessionDescription sdp) async {
    await _pc!.setRemoteDescription(sdp);
    _remoteDescriptionSet = true;
    for (final c in _pendingCandidates) {
      await _pc!.addCandidate(c);
    }
    _pendingCandidates.clear();
  }

  Future<void> addIceCandidate(RTCIceCandidate candidate) async {
    if (!_remoteDescriptionSet) {
      _pendingCandidates.add(candidate);
      return;
    }
    await _pc!.addCandidate(candidate);
  }

  bool sendData(String data) {
    if (!isDataChannelOpen) return false;
    _dataChannel!.send(RTCDataChannelMessage(data));
    return true;
  }

  /// Samples inbound/outbound RTP stats. Bitrate is derived from the byte
  /// delta since the previous call.
  Future<SessionStats> sampleStats() async {
    if (_pc == null) return const SessionStats();
    final reports = await _pc!.getStats();
    int? fps;
    int? bytes;
    int? rttMs;

    for (final r in reports) {
      final v = r.values;
      if (r.type == 'inbound-rtp' && v['kind'] == 'video') {
        fps = (v['framesPerSecond'] as num?)?.round();
        bytes = (v['bytesReceived'] as num?)?.toInt();
      } else if (r.type == 'outbound-rtp' && v['kind'] == 'video') {
        fps ??= (v['framesPerSecond'] as num?)?.round();
        bytes ??= (v['bytesSent'] as num?)?.toInt();
      } else if (r.type == 'candidate-pair' &&
          (v['state'] == 'succeeded' || v['nominated'] == true)) {
        final rtt = v['currentRoundTripTime'] as num?;
        if (rtt != null) rttMs = (rtt * 1000).round();
      }
    }

    int? bitrateKbps;
    final now = DateTime.now();
    if (bytes != null && _lastStatsAt != null) {
      final seconds = now.difference(_lastStatsAt!).inMilliseconds / 1000.0;
      if (seconds > 0) {
        bitrateKbps = (((bytes - _lastBytes) * 8) / 1000 / seconds).round();
      }
    }
    if (bytes != null) {
      _lastBytes = bytes;
      _lastStatsAt = now;
    }

    return SessionStats(bitrateKbps: bitrateKbps, fps: fps, latencyMs: rttMs);
  }

  Future<void> close() async {
    _remoteDescriptionSet = false;
    _pendingCandidates.clear();
    _lastBytes = 0;
    _lastStatsAt = null;
    await _dataChannel?.close();
    await _pc?.close();
    _dataChannel = null;
    _pc = null;
    // The capture service owns the local stream's lifecycle.
    _remoteStream = null;
  }
}
