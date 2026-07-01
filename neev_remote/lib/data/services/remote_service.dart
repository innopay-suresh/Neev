import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:file_selector/file_selector.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart' show Clipboard, ClipboardData;
import 'package:flutter_webrtc/flutter_webrtc.dart';
import 'package:http/http.dart' as http;
import 'package:shared_preferences/shared_preferences.dart';

import '../../core/constants/app_constants.dart';
import 'auth_service.dart';
import 'file_store.dart';
import 'file_transfer_service.dart';
import 'input_event.dart';
import 'input_injector.dart';
import 'screen_capture_service.dart';
import 'signaling_service.dart';
import 'uac_bridge.dart';
import 'webrtc_service.dart';

/// Flip to true to emit verbose input/clipboard diagnostics to the console.
/// Off in shipping builds so the log stays quiet.
const bool kRemoteVerboseLog = false;

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

  // ICE servers resolved from the signaling server at connect time. The server
  // advertises STUN + a reachable TURN relay; without this the app would only
  // ever have STUN and could never relay when the direct path is dead.
  List<Map<String, dynamic>>? _resolvedIce;

  /// Fetch ICE servers (STUN + TURN) from the deployment server. Falls back to
  /// the built-in STUN list if the server is unreachable or returns nothing.
  Future<List<Map<String, dynamic>>> _resolveIceServers(String relayUrl) async {
    try {
      final ws = Uri.parse(relayUrl);
      final scheme = (ws.scheme == 'wss' || ws.scheme == 'https') ? 'https' : 'http';
      final base = '$scheme://${ws.authority}';
      final res = await http
          .get(Uri.parse('$base/api/v1/session/ice-servers'))
          .timeout(const Duration(seconds: 5));
      if (res.statusCode == 200) {
        final body = jsonDecode(res.body) as Map<String, dynamic>;
        final list = (body['ice_servers'] as List?)
                ?.whereType<Map>()
                .map((e) => e.cast<String, dynamic>())
                .toList() ??
            const [];
        if (list.isNotEmpty) {
          if (kRemoteVerboseLog) {
            debugPrint('[ice] resolved ${list.length} server(s) from $base');
          }
          return list;
        }
      }
    } catch (e) {
      if (kRemoteVerboseLog) debugPrint('[ice] resolve failed, using STUN: $e');
    }
    return iceServers;
  }


  // ---- Host state ----
  SignalingService? _hostSignaling;
  final ScreenCaptureService _capture = ScreenCaptureService();
  final InputInjector _injector = InputInjector();

  // Privileged UAC helper bridge (Windows host only; no-op elsewhere). Streams
  // the secure desktop to viewers and injects their Yes/No into consent.exe.
  final UacBridge _uac = UacBridge();

  // Bidirectional file transfer over the peer's dedicated 'file' data channel.
  late final FileTransferManager _files = FileTransferManager(
    send: _sendFileData,
    buffered: _fileBuffered,
    store: FileStore(),
    onChange: notifyListeners,
    onRequest: _onFileRequest,
  );

  /// Active + recent file transfers (for the session UI).
  List<FileTransfer> get fileTransfers => _files.transfers;

  /// Export: send a picked file to the connected peer (viewer→host or
  /// host→viewers).
  Future<FileTransfer?> sendFile(String name, Uint8List bytes) =>
      _files.sendFile(name, bytes);

  /// Import: ask the connected peer to pick a file and send it to us.
  void requestFileFromPeer() {
    _sendFileData(jsonEncode({'k': 'ft', 't': 'request'}));
  }

  // The peer sent an import request — open a picker here and send the choice.
  Future<void> _onFileRequest() async {
    try {
      final f = await openFile();
      if (f == null) return;
      final bytes = await f.readAsBytes();
      await _files.sendFile(f.name, bytes);
    } catch (_) {}
  }

  void clearFinishedTransfers() => _files.clearFinished();

  // Route file bytes to the active peer: the host if we're viewing, else all
  // connected viewers if we're hosting.
  void _sendFileData(String data) {
    final v = _viewerPeer;
    if (v != null) {
      v.sendFileData(data);
      return;
    }
    for (final p in _hostPeers.values) {
      p.sendFileData(data);
    }
  }

  int _fileBuffered() => _viewerPeer?.fileChannelBufferedAmount ?? 0;

  // ---- Viewer-side UAC overlay state (driven by host 'uac' messages) ----
  bool uacActive = false;
  Uint8List? uacFrame;
  int uacW = 0;
  int uacH = 0;
  // A secure-desktop frame is base64'd and split into ordered chunks so it fits
  // the WebRTC data-channel per-message limit (a full-res frame base64s to
  // ~300 KB, over the ~256 KB cap, and was being silently dropped). Reassembled
  // here in order; the reliable/ordered channel guarantees no gaps.
  final StringBuffer _uacChunkBuf = StringBuffer();
  int _uacChunkNext = 0;
  int _uacChunkTotal = 0;
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
  String? _remoteHostOs;
  MediaStream? _remoteStream;
  SessionStats _stats = const SessionStats();
  Timer? _statsTimer;

  // ---- Clipboard sync (shared across roles) ----
  Timer? _clipTimer;
  String? _lastClip;

  // ---- Host dead-man's switch: release stuck buttons if input goes silent
  // (viewer minimized / frozen / disconnected) so the host mouse never freezes.
  final Set<int> _heldButtons = {};
  final Stopwatch _inputClock = Stopwatch()..start();
  int _lastInputMs = 0;
  Timer? _hostInputWatchdog;

  // AnyDesk/TeamViewer model: when the SYSTEM helper agent is connected, ALL
  // input is injected by it (it runs at SYSTEM integrity, so UIPI never blocks
  // it — it reaches elevated windows, the UAC secure desktop, and the login
  // screen alike). Our own Medium-integrity injector is only the fallback for
  // when the helper isn't installed/running. [_routeToHelper] latches while a
  // button is held so a drag never splits across the two injectors.
  bool _routeToHelper = false;

  ViewerStatus get viewerStatus => _viewerStatus;
  bool get isViewing =>
      _viewerStatus == ViewerStatus.connecting ||
      _viewerStatus == ViewerStatus.connected;
  String? get targetId => _targetId;
  String? get viewerError => _viewerError;
  /// The remote host's OS ('windows' | 'macos' | 'linux'), learned over the
  /// control channel. Null until the host announces it. Used by the viewer to
  /// translate the primary command modifier across platforms.
  String? get remoteHostOs => _remoteHostOs;
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
    _resolvedIce = await _resolveIceServers(relayUrl);
    _setupUacBridge();  // Windows host: stream UAC to viewers (no-op elsewhere)

    final pw = (password == null || password.isEmpty)
        ? AuthService.generatePassword()
        : password;
    _password = pw;
    // Stable per-install ID: generated once, persisted, and reused on every
    // launch so the ID a user shares keeps working. The password still rotates
    // each session. Only a reinstall (cleared prefs) yields a new ID.
    final agentId = fixedAgentId ?? await _persistentAgentId();
    _hostStatus = HostStatus.starting;
    _hostError = null;
    notifyListeners();

    final signaling = SignalingService(
      serverUrl: relayUrl,
      onMessage: _onHostMessage,
      onConnected: () {
        _hostSignaling?.registerHost(
          passwordHash: AuthService.hashPassword(pw),
          agentId: agentId,
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
    _stopHostInputWatchdog();
    _routeToHelper = false;
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
    // Cap the resolution: capturing a Retina display at full native pixels
    // (e.g. 2880×1800) produces large frames that add encode + network
    // latency. 1920-wide keeps text readable while noticeably cutting lag.
    final stream = _capture.stream ??
        await _capture.startCapture(fps: 30, maxWidth: 1920, maxHeight: 1200);
    if (stream == null) {
      _hostError = 'Screen capture failed (permission denied?)';
      notifyListeners();
      return;
    }

    final peer = WebRTCService();
    peer.onDataMessage = (raw) => _handleData(raw, isHost: true);
    // Announce our OS so the viewer can translate its primary command modifier
    // (⌘ on macOS ↔ Ctrl on Windows/Linux) for copy/paste and other shortcuts.
    peer.onDataChannelOpen = () =>
        peer.sendData(jsonEncode({'k': 'os', 'v': _osName()}));
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

    // Use iceTransportPolicy 'all': direct path for same-network peers (e.g.
    // <->Mac), automatic TURN-relay fallback when no direct path exists (e.g.
    // Win<->Win across AP-isolated clients). Forcing relay broke the working
    // direct paths, so we let ICE choose.
    await peer.initialize(
      iceServers: _resolvedIce ?? iceServers,
      isOfferer: true,
    );
    await peer.addLocalStream(stream);
    final offer = await peer.createOffer();
    _hostSignaling?.sendOffer(controllerId, _sdpMap(offer));
    _ensureClipboardSync();
    _startHostInputWatchdog();
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
    _resolvedIce = await _resolveIceServers(relayUrl);

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

  /// Sends a remote-control input event to the host. Mouse MOVES go on the
  /// low-latency unreliable channel (stale moves are dropped, so the cursor
  /// doesn't lag); buttons, wheel and keys stay on the reliable channel so they
  /// are never lost or reordered.
  void sendViewerInput(InputEvent event) {
    if (event.kind == 'mv') {
      _viewerPeer?.sendCursor(event.encode());
    } else {
      _viewerPeer?.sendData(event.encode());
    }
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
    peer.onDataMessage = (raw) => _handleData(raw, isHost: false);
    peer.onRemoteStream = (stream) {
      _remoteStream = stream;
      _viewerStatus = ViewerStatus.connected;
      _startStatsTimer();
      _ensureClipboardSync();
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

    await peer.initialize(
      iceServers: _resolvedIce ?? iceServers,
      isOfferer: false,
    );
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
  // Clipboard sync + data-channel routing
  // =========================================================================

  /// Routes an incoming data-channel message. Clipboard messages update the
  /// local clipboard on both roles; input events are injected on the host only.
  Future<void> _handleData(String raw, {required bool isHost}) async {
    Map<String, dynamic>? m;
    try {
      final decoded = jsonDecode(raw);
      if (decoded is Map<String, dynamic>) m = decoded;
    } catch (_) {}
    if (m == null) return;

    if (m['k'] == 'clip') {
      final text = m['t'] as String?;
      if (text != null) {
        _lastClip = text; // avoid echoing it straight back
        await Clipboard.setData(ClipboardData(text: text));
        if (kRemoteVerboseLog) {
          debugPrint('[clip] received ${text.length} chars -> local clipboard');
        }
      }
      return;
    }

    // Host announces its OS so the viewer can map ⌘ ↔ Ctrl.
    if (m['k'] == 'os') {
      _remoteHostOs = m['v'] as String?;
      if (kRemoteVerboseLog) debugPrint('[os] remote host is $_remoteHostOs');
      notifyListeners();
      return;
    }

    // UAC secure-desktop stream (host -> viewer).
    if (m['k'] == 'uac') {
      _onUacMessage(m);
      return;
    }
    // UAC viewer input (viewer -> host) -> inject via the helper agent.
    if (m['k'] == 'uacin') {
      if (isHost) _onUacInput(m);
      return;
    }
    // File transfer (either direction).
    if (m['k'] == 'ft') {
      _files.handleMessage(m);
      return;
    }

    if (isHost) {
      final event = InputEvent.decode(raw);
      if (event != null) {
        _trackHeldButton(event);
        _lastInputMs = _inputClock.elapsedMilliseconds;
        _logHostInput(event);
        _routeInput(event);
      }
    }
  }

  // Inject one host-side input event. Routes through the SYSTEM helper whenever
  // it's connected (so elevated windows / the secure desktop are reachable),
  // else falls back to the in-app injector. The route is only re-evaluated while
  // nothing is held, so a press/drag started on one injector finishes on it.
  void _routeInput(InputEvent event) {
    if (_heldButtons.isEmpty) _routeToHelper = _uac.isConnected;
    if (_routeToHelper) {
      _uac.sendInput(event.data);
    } else {
      _injector.inject(event);
    }
  }

  // Viewer side: a UAC frame/state arrived from the host.
  void _onUacMessage(Map<String, dynamic> m) {
    final t = m['t'] as String?;
    if (t == 'active') {
      uacActive = true;
      uacW = (m['w'] as int?) ?? 0;
      uacH = (m['h'] as int?) ?? 0;
    } else if (t == 'frame') {
      final d = m['d'] as String?;
      if (d == null) return;
      final idx = m['i'] as int?;
      final total = m['n'] as int?;
      if (idx == null || total == null) {
        // Legacy single-message frame.
        uacFrame = base64Decode(d);
        uacActive = true;
      } else {
        if (idx == 0) {
          _uacChunkBuf.clear();
          _uacChunkNext = 0;
          _uacChunkTotal = total;
        }
        if (idx == _uacChunkNext && total == _uacChunkTotal) {
          _uacChunkBuf.write(d);
          _uacChunkNext++;
          if (_uacChunkNext == _uacChunkTotal) {
            try {
              uacFrame = base64Decode(_uacChunkBuf.toString());
              uacActive = true;
            } catch (_) {}
            _uacChunkBuf.clear();
            _uacChunkNext = 0;
          }
        } else {
          // A chunk arrived out of sequence — drop this partial frame and wait
          // for the next one to start fresh at idx 0.
          _uacChunkBuf.clear();
          _uacChunkNext = 0;
        }
        if (idx != _uacChunkTotal - 1) return; // no repaint mid-frame
      }
    } else if (t == 'gone') {
      uacActive = false;
      uacFrame = null;
      _uacChunkBuf.clear();
      _uacChunkNext = 0;
    }
    notifyListeners();
  }

  // Host side: a viewer's UAC click/key -> inject onto the secure desktop.
  void _onUacInput(Map<String, dynamic> m) {
    final a = m['a'] as String?;
    if (a == 'click') {
      _uac.sendClick((m['b'] as int?) ?? 0, (m['x'] as num?)?.toDouble() ?? 0,
          (m['y'] as num?)?.toDouble() ?? 0);
    } else if (a == 'key') {
      _uac.sendKey((m['vk'] as int?) ?? 0);
    }
  }

  /// Viewer: send a click on the UAC overlay (normalized 0..1) to the host.
  void sendUacClick(int button, double x, double y) {
    _viewerPeer?.sendData(
        jsonEncode({'k': 'uacin', 'a': 'click', 'b': button, 'x': x, 'y': y}));
  }

  /// Viewer: send a key (Win32 VK code) to the UAC prompt on the host.
  void sendUacKey(int vk) {
    _viewerPeer?.sendData(jsonEncode({'k': 'uacin', 'a': 'key', 'vk': vk}));
  }

  /// Viewer: APPROVE the UAC prompt. Uses the proven keyboard path — Left moves
  /// focus from the default No to Yes, then Enter activates it (200ms apart so
  /// the focus change registers). More reliable than a coordinate mouse click.
  void sendUacApprove() {
    _viewerPeer?.sendData(jsonEncode({'k': 'uacin', 'a': 'key', 'vk': 0x25})); // VK_LEFT
    Future.delayed(const Duration(milliseconds: 220), () {
      _viewerPeer?.sendData(jsonEncode({'k': 'uacin', 'a': 'key', 'vk': 0x0D})); // VK_RETURN
    });
  }

  /// Viewer: DECLINE the UAC prompt (Esc).
  void sendUacDecline() {
    _viewerPeer?.sendData(jsonEncode({'k': 'uacin', 'a': 'key', 'vk': 0x1B})); // VK_ESCAPE
  }

  // Host: wire the helper-agent UAC stream to all connected viewers.
  void _setupUacBridge() {
    if (!_uac.isSupported) return;
    _uac.onActive = (w, h) => _broadcastToPeers(
        jsonEncode({'k': 'uac', 't': 'active', 'w': w, 'h': h}));
    _uac.onFrame = _broadcastUacFrame;
    _uac.onGone = () => _broadcastToPeers(jsonEncode({'k': 'uac', 't': 'gone'}));
    _uac.start();
  }

  // Base64 a secure-desktop frame and send it in ordered chunks small enough for
  // one WebRTC data-channel message. A full-res frame base64s to ~300 KB, which
  // overran the ~256 KB per-message limit and was dropped whole — so a high-DPI
  // host's UAC prompt never appeared in the viewer.
  void _broadcastUacFrame(Uint8List png) {
    final b64 = base64Encode(png);
    const chunkLen = 48 * 1024; // 48 KB/message — safely under the DC limit
    final total = (b64.length / chunkLen).ceil().clamp(1, 1 << 20);
    for (var i = 0; i < total; i++) {
      final start = i * chunkLen;
      final end = start + chunkLen < b64.length ? start + chunkLen : b64.length;
      _broadcastToPeers(jsonEncode({
        'k': 'uac',
        't': 'frame',
        'i': i,
        'n': total,
        'd': b64.substring(start, end),
      }));
    }
  }

  void _broadcastToPeers(String msg) {
    for (final peer in _hostPeers.values) {
      peer.sendData(msg);
    }
  }

  void _trackHeldButton(InputEvent e) {
    if (e.data['k'] != 'btn') return;
    final b = (e.data['b'] as int?) ?? 0;
    if (e.data['d'] == true) {
      _heldButtons.add(b);
    } else {
      _heldButtons.remove(b);
    }
  }

  void _startHostInputWatchdog() {
    _hostInputWatchdog?.cancel();
    _hostInputWatchdog = Timer.periodic(const Duration(milliseconds: 500), (_) {
      if (_heldButtons.isEmpty) return;
      if (_inputClock.elapsedMilliseconds - _lastInputMs < 1500) return;
      // Input went silent while a button was held — release it so the host's
      // mouse doesn't stay stuck (fixes the minimize/maximize freeze). Route it
      // the same way live input goes so the release reaches whichever injector
      // is holding the button.
      for (final b in _heldButtons.toList()) {
        _routeInput(InputEvent.button(b, false));
      }
      _heldButtons.clear();
    });
  }

  void _stopHostInputWatchdog() {
    _hostInputWatchdog?.cancel();
    _hostInputWatchdog = null;
    _heldButtons.clear();
  }

  // Host-side receive heartbeat: confirms whether input keeps arriving after a
  // click (host stops receiving = viewer/data-channel issue; host receives but
  // cursor frozen = native injection issue).
  int _hostMoveCount = 0;
  final Stopwatch _hostInputClock = Stopwatch()..start();
  int _hostInputHeartbeatMs = 0;
  void _logHostInput(InputEvent e) {
    if (!kRemoteVerboseLog) return;
    final kind = e.kind;
    if (kind == 'mv') {
      _hostMoveCount++;
      final now = _hostInputClock.elapsedMilliseconds;
      if (now - _hostInputHeartbeatMs >= 1000) {
        debugPrint('[host-input] moves received ~1s: $_hostMoveCount');
        _hostMoveCount = 0;
        _hostInputHeartbeatMs = now;
      }
    } else {
      debugPrint('[host-input] $kind ${e.data}');
    }
  }

  void _ensureClipboardSync() {
    if (_clipTimer != null) return;
    // Prime _lastClip so we don't immediately broadcast the existing clipboard.
    Clipboard.getData('text/plain').then((d) => _lastClip = d?.text);
    _clipTimer = Timer.periodic(const Duration(milliseconds: 600), (_) async {
      if (_hostPeers.isEmpty && _viewerPeer == null) {
        _stopClipboardSync();
        return;
      }
      String? text;
      try {
        final data = await Clipboard.getData('text/plain');
        text = data?.text;
      } catch (e) {
        if (kRemoteVerboseLog) debugPrint('[clip] read failed: $e');
        return;
      }
      if (text == null || text.isEmpty || text == _lastClip) return;
      _lastClip = text;
      if (kRemoteVerboseLog) {
        debugPrint('[clip] local change ${text.length} chars -> broadcasting');
      }
      _broadcastClip(text);
    });
  }

  void _stopClipboardSync() {
    _clipTimer?.cancel();
    _clipTimer = null;
  }

  void _broadcastClip(String text) {
    final msg = jsonEncode({'k': 'clip', 't': text});
    for (final peer in _hostPeers.values) {
      peer.sendData(msg);
    }
    _viewerPeer?.sendData(msg);
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

  static const _kPersistentAgentId = 'persistentAgentId';

  /// Returns this install's stable agent ID, generating and persisting one the
  /// first time. Format matches the server's `%03d-%03d-%03d` (e.g. 123-456-789)
  /// so existing routing/UI conventions keep working.
  Future<String> _persistentAgentId() async {
    final prefs = await SharedPreferences.getInstance();
    var id = prefs.getString(_kPersistentAgentId);
    if (id == null || id.isEmpty) {
      id = _generateAgentId();
      await prefs.setString(_kPersistentAgentId, id);
    }
    return id;
  }

  String _generateAgentId() {
    final n = Random.secure().nextInt(1000000000); // 0 .. 999,999,999
    final s = n.toString().padLeft(9, '0');
    return '${s.substring(0, 3)}-${s.substring(3, 6)}-${s.substring(6, 9)}';
  }

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
    _stopClipboardSync();
    _uac.dispose();
    stopHosting();
    disconnectViewer();
    super.dispose();
  }
}
