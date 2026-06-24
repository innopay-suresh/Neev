import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';

import '../../core/constants/app_constants.dart';
import 'auth_service.dart';
import 'input_event.dart';
import 'input_injector.dart';
import 'screen_capture_service.dart';
import 'signaling_service.dart';
import 'webrtc_service.dart';

enum HostStatus { offline, starting, online, error }

enum ViewerStatus { idle, connecting, connected, failed }

/// Central orchestrator that turns the signaling + WebRTC + capture services
/// into a working remote-desktop session, for both roles:
///
///  * **Host** (agent): registers with the Go signaling server, waits for an
///    incoming `connect`, captures the screen and becomes the WebRTC offerer.
///  * **Viewer** (controller): sends `connect`, answers the host's offer and
///    renders the incoming video stream.
///
/// Both roles use independent signaling connections so a single app instance
/// can host and view at the same time (like AnyDesk).
class RemoteService extends ChangeNotifier {
  RemoteService({this.iceServers = AppConstants.iceServers});

  final List<Map<String, dynamic>> iceServers;

  // ---- Host state ----
  SignalingService? _hostSignaling;
  final ScreenCaptureService _capture = ScreenCaptureService();
  final InputInjector _injector = InputInjector();
  final Map<String, WebRTCService> _hostPeers = {};
  HostStatus _hostStatus = HostStatus.offline;
  String? _agentId;
  String? _password;
  String? _hostError;

  HostStatus get hostStatus => _hostStatus;
  bool get isHosting =>
      _hostStatus == HostStatus.online || _hostStatus == HostStatus.starting;
  String? get agentId => _agentId;
  String? get password => _password;
  String? get hostError => _hostError;
  int get connectedViewers => _hostPeers.length;

  // ---- Viewer state ----
  SignalingService? _viewerSignaling;
  WebRTCService? _viewerPeer;
  ViewerStatus _viewerStatus = ViewerStatus.idle;
  String? _targetId;
  String? _viewerError;
  MediaStream? _remoteStream;
  SessionStats _stats = const SessionStats();
  Timer? _statsTimer;

  ViewerStatus get viewerStatus => _viewerStatus;
  bool get isViewing =>
      _viewerStatus == ViewerStatus.connecting ||
      _viewerStatus == ViewerStatus.connected;
  String? get targetId => _targetId;
  String? get viewerError => _viewerError;
  MediaStream? get remoteStream => _remoteStream;
  SessionStats get stats => _stats;

  // =========================================================================
  // HOST
  // =========================================================================

  /// Starts hosting. Returns the generated/used password so the UI can show it.
  Future<String> startHosting({
    required String relayUrl,
    String? password,
    String? fixedAgentId,
  }) async {
    await stopHosting();

    final pw = (password == null || password.isEmpty)
        ? AuthService.generatePassword()
        : password;
    _password = pw;
    _hostStatus = HostStatus.starting;
    _hostError = null;
    notifyListeners();

    final signaling = SignalingService(
      serverUrl: relayUrl,
      onMessage: _onHostMessage,
      onConnected: () {
        _hostSignaling?.registerHost(
          passwordHash: AuthService.hashPassword(pw),
          agentId: fixedAgentId,
          hostname: _hostname(),
          os: _osName(),
          version: AppConstants.appVersion,
        );
      },
      onDisconnected: () {
        if (_hostStatus != HostStatus.offline) {
          _hostStatus = HostStatus.error;
          _hostError = 'Disconnected from signaling server';
          notifyListeners();
        }
      },
    );
    _hostSignaling = signaling;

    try {
      await signaling.connect();
    } catch (e) {
      _hostStatus = HostStatus.error;
      _hostError = 'Cannot reach signaling server: $e';
      notifyListeners();
      rethrow;
    }
    return pw;
  }

  Future<void> stopHosting() async {
    _statsTimerMaybeStop();
    for (final peer in _hostPeers.values) {
      await peer.close();
    }
    _hostPeers.clear();
    await _capture.stopCapture();
    await _hostSignaling?.disconnect();
    _hostSignaling = null;
    _agentId = null;
    _hostStatus = HostStatus.offline;
    notifyListeners();
  }

  Future<void> _onHostMessage(SignalingMessage msg) async {
    switch (msg.type) {
      case SignalingMessageType.registered:
        _agentId = msg.payload?['agent_id'] as String?;
        _hostStatus = HostStatus.online;
        notifyListeners();
        break;
      case SignalingMessageType.connect:
        // A controller wants in. msg.from is the controller's routing id.
        final controllerId = msg.from;
        if (controllerId != null) await _startHostOffer(controllerId);
        break;
      case SignalingMessageType.answer:
        final peer = _hostPeers[msg.from];
        if (peer != null && msg.payload != null) {
          await peer.setRemoteDescription(_sdpFrom(msg.payload));
        }
        break;
      case SignalingMessageType.candidate:
        final peer = _hostPeers[msg.from];
        if (peer != null && msg.payload != null) {
          await peer.addIceCandidate(_candidateFrom(msg.payload));
        }
        break;
      case SignalingMessageType.bye:
        final peer = _hostPeers.remove(msg.from);
        await peer?.close();
        notifyListeners();
        break;
      case SignalingMessageType.error:
        _hostError = msg.error ?? 'Signaling error';
        notifyListeners();
        break;
      default:
        break;
    }
  }

  Future<void> _startHostOffer(String controllerId) async {
    // Capture the screen once and reuse the stream across viewers.
    final stream = _capture.stream ?? await _capture.startCapture();
    if (stream == null) {
      _hostError = 'Screen capture failed (permission denied?)';
      notifyListeners();
      return;
    }

    final peer = WebRTCService();
    peer.onDataMessage = (raw) {
      final event = InputEvent.decode(raw);
      if (event != null) _injector.inject(event);
    };
    peer.onIceCandidate = (c) =>
        _hostSignaling?.sendCandidate(controllerId, _candidateMap(c));
    peer.onConnectionStateChange = (state) {
      if (state ==
              RTCPeerConnectionState.RTCPeerConnectionStateFailed ||
          state == RTCPeerConnectionState.RTCPeerConnectionStateClosed) {
        _hostPeers.remove(controllerId)?.close();
        notifyListeners();
      }
    };
    _hostPeers[controllerId] = peer;

    await peer.initialize(iceServers: iceServers, isOfferer: true);
    await peer.addLocalStream(stream);
    final offer = await peer.createOffer();
    _hostSignaling?.sendOffer(controllerId, _sdpMap(offer));
    notifyListeners();
  }

  // =========================================================================
  // VIEWER
  // =========================================================================

  Future<void> connectToHost({
    required String relayUrl,
    required String targetId,
    required String password,
  }) async {
    await disconnectViewer();

    _targetId = targetId;
    _viewerStatus = ViewerStatus.connecting;
    _viewerError = null;
    notifyListeners();

    final signaling = SignalingService(
      serverUrl: relayUrl,
      onMessage: _onViewerMessage,
      onConnected: () {
        // The viewer (controller) does not register; it just requests a peer.
        _viewerSignaling?.sendConnect(targetId, password);
      },
      onDisconnected: () {
        if (_viewerStatus != ViewerStatus.idle) {
          _viewerStatus = ViewerStatus.failed;
          _viewerError = 'Disconnected from signaling server';
          notifyListeners();
        }
      },
    );
    _viewerSignaling = signaling;

    try {
      await signaling.connect();
    } catch (e) {
      _viewerStatus = ViewerStatus.failed;
      _viewerError = 'Cannot reach signaling server: $e';
      notifyListeners();
    }
  }

  /// Sends a remote-control input event to the host. No-op until the control
  /// data channel is open.
  void sendViewerInput(InputEvent event) {
    _viewerPeer?.sendData(event.encode());
  }

  Future<void> disconnectViewer() async {
    _statsTimerMaybeStop();
    final id = _targetId;
    if (id != null) _viewerSignaling?.sendBye(id);
    await _viewerPeer?.close();
    _viewerPeer = null;
    await _viewerSignaling?.disconnect();
    _viewerSignaling = null;
    _remoteStream = null;
    _targetId = null;
    _stats = const SessionStats();
    _viewerStatus = ViewerStatus.idle;
    notifyListeners();
  }

  Future<void> _onViewerMessage(SignalingMessage msg) async {
    switch (msg.type) {
      case SignalingMessageType.connect:
        // Server confirmed the request was accepted; await the host's offer.
        break;
      case SignalingMessageType.offer:
        await _answerHostOffer(msg);
        break;
      case SignalingMessageType.candidate:
        if (_viewerPeer != null && msg.payload != null) {
          await _viewerPeer!.addIceCandidate(_candidateFrom(msg.payload));
        }
        break;
      case SignalingMessageType.bye:
        await disconnectViewer();
        break;
      case SignalingMessageType.error:
        _viewerStatus = ViewerStatus.failed;
        _viewerError = msg.error ?? 'Connection rejected';
        notifyListeners();
        break;
      default:
        break;
    }
  }

  Future<void> _answerHostOffer(SignalingMessage msg) async {
    final hostId = msg.from;
    if (hostId == null || msg.payload == null) return;

    final peer = WebRTCService();
    _viewerPeer = peer;
    peer.onRemoteStream = (stream) {
      _remoteStream = stream;
      _viewerStatus = ViewerStatus.connected;
      _startStatsTimer();
      notifyListeners();
    };
    peer.onIceCandidate = (c) =>
        _viewerSignaling?.sendCandidate(hostId, _candidateMap(c));
    peer.onConnectionStateChange = (state) {
      if (state == RTCPeerConnectionState.RTCPeerConnectionStateFailed) {
        _viewerStatus = ViewerStatus.failed;
        _viewerError = 'Connection failed';
        notifyListeners();
      }
    };

    await peer.initialize(iceServers: iceServers, isOfferer: false);
    await peer.setRemoteDescription(_sdpFrom(msg.payload));
    final answer = await peer.createAnswer();
    _viewerSignaling?.sendAnswer(hostId, _sdpMap(answer));
  }

  // =========================================================================
  // Stats
  // =========================================================================

  void _startStatsTimer() {
    _statsTimerMaybeStop();
    _statsTimer = Timer.periodic(const Duration(seconds: 1), (_) async {
      final peer = _viewerPeer;
      if (peer == null) return;
      _stats = await peer.sampleStats();
      notifyListeners();
    });
  }

  void _statsTimerMaybeStop() {
    _statsTimer?.cancel();
    _statsTimer = null;
  }

  // =========================================================================
  // Helpers
  // =========================================================================

  Map<String, dynamic> _sdpMap(RTCSessionDescription d) =>
      {'sdp': d.sdp, 'type': d.type};

  RTCSessionDescription _sdpFrom(dynamic p) =>
      RTCSessionDescription(p['sdp'] as String?, p['type'] as String?);

  Map<String, dynamic> _candidateMap(RTCIceCandidate c) => {
        'candidate': c.candidate,
        'sdpMid': c.sdpMid,
        'sdpMLineIndex': c.sdpMLineIndex,
      };

  RTCIceCandidate _candidateFrom(dynamic p) => RTCIceCandidate(
        p['candidate'] as String?,
        p['sdpMid'] as String?,
        p['sdpMLineIndex'] as int?,
      );

  /// A best-effort hostname. The platform host name is only available via
  /// dart:io on native targets; to keep the orchestrator web-safe we derive a
  /// label from the platform instead.
  String _hostname() => '${_osName()}-host';

  String _osName() {
    if (kIsWeb) return 'web';
    switch (defaultTargetPlatform) {
      case TargetPlatform.windows:
        return 'windows';
      case TargetPlatform.macOS:
        return 'macos';
      case TargetPlatform.linux:
        return 'linux';
      case TargetPlatform.android:
        return 'android';
      case TargetPlatform.iOS:
        return 'ios';
      default:
        return 'unknown';
    }
  }

  @override
  void dispose() {
    _statsTimerMaybeStop();
    stopHosting();
    disconnectViewer();
    super.dispose();
  }
}
