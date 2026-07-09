import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:file_selector/file_selector.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart' show Clipboard, ClipboardData;
import 'package:flutter_webrtc/flutter_webrtc.dart';
import 'package:http/http.dart' as http;
import 'package:pasteboard/pasteboard.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../core/constants/app_constants.dart';
import '../../core/diag_log.dart';
import 'auth_service.dart';
import 'clip_agent_bridge.dart';
import 'clipboard_writer.dart';
import 'discovery_model.dart';
import 'file_store.dart';
import 'file_transfer_service.dart';
import 'host_mode.dart';
import 'host_name.dart' as host_name;
import 'input_event.dart';
import 'input_injector.dart';
import 'keyboard_hook.dart';
import 'privacy_mode.dart';
import 'screen_capture_service.dart';
import 'signaling_service.dart';
import 'system_command.dart';
import 'uac_bridge.dart';
import 'webrtc_service.dart';

/// Flip to true to emit verbose input/clipboard diagnostics to the console.
/// Off in shipping builds so the log stays quiet.
const bool kRemoteVerboseLog = false;

enum HostStatus { offline, starting, online, error }

enum ViewerStatus { idle, connecting, connected, failed }

/// One in-session chat line.
class ChatMessage {
  final String text;
  final bool mine;
  ChatMessage(this.text, {required this.mine});
}

/// A pending incoming-connection request awaiting the host user's consent.
class ConsentRequest {
  final String controllerId;
  ConsentRequest(this.controllerId);
}

/// Reassembly state for one incoming clipboard file's chunked bytes.
class _ClipRecv {
  final int total;
  int next = 0;
  final StringBuffer buf = StringBuffer();
  _ClipRecv(this.total);
}

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
    onClipboardFile: _onClipboardFileReceived,
  );

  /// Active + recent file transfers (for the session UI). Clipboard-copied
  /// files ARE shown now — a visible confirmation that the copy went through
  /// (and where it landed) is more reliable than silent CF_HDROP paste.
  List<FileTransfer> get fileTransfers => _files.transfers;

  // A clipboard file finished arriving: put it on THIS machine's clipboard so
  // Ctrl+V pastes the real file. Suppress our own poller so we don't echo it.
  Future<void> _onClipboardFileReceived(String path) async {
    if (!clipboardSyncEnabled) return;
    try {
      _clipFileSuppress = 3;
      _lastClipFiles = [path];
      // Write with an explicit COPY drop-effect so Ctrl+V copies (not moves) the
      // file — otherwise the mirrored file vanishes from its folder on paste.
      //  1. SYSTEM host → the user-context clip agent (writes COPY).
      //  2. Attended Windows → our runner's native writer (writes COPY).
      //  3. Anything else (macOS/Linux) → Pasteboard; no move semantics there.
      final ok = await _clipAgent.writeFiles([path]) ||
          await ClipboardWriter.writeFilesCopy([path]);
      if (!ok) await Pasteboard.writeFiles([path]);
    } catch (_) {}
  }

  /// Export: send a picked file to the connected peer (viewer→host or
  /// host→viewers).
  Future<FileTransfer?> sendFile(String name, Uint8List bytes) =>
      _files.sendFile(name, bytes);

  /// Import: ask the connected peer to pick a file and send it to us.
  void requestFileFromPeer() {
    _sendFileData(jsonEncode({'k': 'ft', 't': 'request'}));
  }

  /// In-session view-only: when true the viewer watches without sending input
  /// (separate from the persisted view-only setting; either one disables input).
  bool viewerViewOnly = false;
  void setViewOnly(bool value) {
    if (viewerViewOnly == value) return;
    viewerViewOnly = value;
    notifyListeners();
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
  // Which secure desktop is showing: 0=UAC prompt, 1=login screen, 2=locked.
  int uacKind = 0;
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

  // Server-assisted discovery: the relay groups hosts by public IP and tells us
  // our LAN-mates, so discovery works even where UDP broadcast is blocked.
  Timer? _discoverTimer;
  final Map<String, DiscoveredDevice> _serverPeers = {};

  /// Hosts the relay reports on our network (from the last `peers` reply).
  List<DiscoveredDevice> get serverPeers => _serverPeers.values.toList();

  // ---- Incoming-connection consent + per-session permissions (host) --------
  /// When true, an incoming connection prompts the host user (Accept/Dismiss).
  /// Set false for unattended access. The app wires this from settings.
  bool promptOnConnect = true;
  ConsentRequest? _pendingConsent;
  ConsentRequest? get pendingConsent => _pendingConsent;
  // Permissions granted to the current session (host → viewer).
  bool permControl = true;
  bool permClipboard = true;
  bool permFiles = true;
  // Defaults pushed from settings — pre-fill the consent dialog + used when
  // accepting silently (unattended / never-ask).
  bool defaultPermControl = true;
  bool defaultPermClipboard = true;
  bool defaultPermFiles = true;

  /// Master user toggle (Settings) for clipboard mirroring. When false, nothing
  /// is polled from the local clipboard and nothing incoming is written to it —
  /// a hard off switch in both directions. Pushed from [AppSettings.clipboardSync].
  bool clipboardSyncEnabled = true;

  // ---- Clipboard files: announce-on-copy → deliver-on-paste --------------
  // Source side: announced sets we can still serve, token -> local file paths.
  int _clipOutToken = 0;
  final Map<String, List<String>> _clipOutFiles = {};
  // Destination side: poll the native delayed-render object for paste requests
  // (Windows attended only) and reassemble incoming file bytes.
  Timer? _clipFetchPoller;
  final Map<String, _ClipRecv> _clipRecv = {}; // key 'token#index'
  final Set<String> _clipNativeTokens = {}; // tokens handed to delayed-render
  final Map<String, List<String>> _clipRecvNames = {}; // token -> file names

  /// Host: accept the pending incoming connection with the chosen permissions.
  Future<void> acceptConnection(
      {bool control = true, bool clipboard = true, bool files = true}) async {
    final req = _pendingConsent;
    if (req == null) return;
    permControl = control;
    permClipboard = clipboard;
    permFiles = files;
    _pendingConsent = null;
    notifyListeners();
    await _startHostOffer(req.controllerId);
  }

  /// Host: decline the pending incoming connection.
  void rejectConnection() {
    final req = _pendingConsent;
    if (req == null) return;
    _pendingConsent = null;
    notifyListeners();
    _hostSignaling?.sendBye(req.controllerId);
  }
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

  // Auto-reconnect: after a remote reboot (or any unexpected drop) keep re-dialing
  // the same host for a while. Params persist across disconnectViewer so a retry
  // can reuse them. NOTE: for the host to reappear after a reboot it must be set
  // to auto-start + share on boot (unattended access — a later feature).
  String? _lastRelayUrl;
  String? _lastTargetId;
  String? _lastPassword;
  bool autoReconnect = false;
  Timer? _reconnectTimer;
  int _reconnectTries = 0;
  // Grace timer for the ICE 'disconnected' state before we declare the peer lost
  // (a killed host on user-switch shows as 'disconnected', not 'failed').
  Timer? _disconnectGrace;

  /// Host monitors available to switch between (viewer side; empty if the host
  /// has a single monitor). Each entry: {'id':..., 'n': name}.
  List<Map<String, String>> hostMonitors = const [];
  String? _remoteHostOs;
  MediaStream? _remoteStream;
  SessionStats _stats = const SessionStats();
  Timer? _statsTimer;

  // ---- Clipboard sync (shared across roles) ----
  Timer? _clipTimer;
  String? _lastClip;
  // Clipboard image sync (chunked, since images are large).
  int _lastClipImgHash = 0;
  int _clipTick = 0;
  final StringBuffer _clipImgBuf = StringBuffer();
  int _clipImgNext = 0;
  int _clipImgTotal = 0;
  // Clipboard FILE sync: copying a file mirrors it to the peer's CLIPBOARD (via
  // a temp file over the reliable file channel), so Ctrl+V on the other machine
  // pastes the actual file.
  List<String> _lastClipFiles = const [];
  int _clipFileSuppress = 0; // ticks to skip re-sending a just-received file
  // User-context clipboard agent (SYSTEM helper): reads/writes the interactive
  // FILE clipboard that a SYSTEM host can't touch itself. Falls back to
  // Pasteboard when absent (attended install).
  final ClipAgentBridge _clipAgent = ClipAgentBridge();

  // ---- Host dead-man's switch: release stuck buttons if input goes silent
  // (viewer minimized / frozen / disconnected) so the host mouse never freezes.
  final Set<int> _heldButtons = {};
  final Stopwatch _inputClock = Stopwatch()..start();
  int _lastInputMs = 0;
  Timer? _hostInputWatchdog;

  // The SYSTEM helper was meant to inject ALL normal input (SYSTEM integrity →
  // reaches UIPI-elevated windows), but its normal-desktop injection has proven
  // unreliable in the field: the helper accepts the events yet they never land.
  // On old builds only the button-up rode the helper — every click became an
  // endless drag; once the whole click was routed there (r22), clicks went
  // fully dead while in-app moves kept working. Until helper injection is
  // debugged on a real Windows box, ALL normal input goes through the in-app
  // injector — one serial channel, strictly ordered, demonstrably working.
  // The helper still does what only it can: UAC secure-desktop clicks ('C'),
  // credential typing ('T'), and Ctrl+Alt+Del ('S'). Known trade-off: clicks
  // can't reach UIPI-elevated windows until this is re-enabled.
  static const bool _kRouteNormalInputViaHelper = false;
  bool _routeToHelper = false;
  // True while THIS host is showing a secure desktop (UAC / sign-in / lock /
  // switch-user). Only the SYSTEM helper can inject into Winlogon, so input is
  // forced through it while this is set — independent of the flag above, which
  // only governs normal-desktop (elevated-window) routing. Set from the helper's
  // onActive/onGone in [_setupUacBridge].
  bool _hostSecureActive = false;
  // True while the host's foreground window is an ELEVATED (High-IL) window.
  // The Medium-integrity in-app injector is UIPI-blocked from such windows, so
  // input is force-routed through the SYSTEM helper agent while this is set.
  // Driven by the helper's onElevated (see [_setupUacBridge]).
  bool _hostElevatedActive = false;
  // While set (ms on [_inputClock]), mouse moves follow button events onto the
  // helper channel so a faster in-app move can't overtake an in-flight click.
  int _helperMoveGraceUntilMs = 0;

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
    // Single-host guarantee: when the SYSTEM service transport owns hosting
    // (TransportMode), this app must NEVER register as a second connectable host
    // — the service transport is the one machine identity (capture + SYSTEM
    // input + secure desktop). Guard here so EVERY caller (auto-host, settings,
    // Share button, fixed-password) is covered, not just auto-host. We still
    // surface the machine id+password for the UI to dial, but do not register.
    if (await HostMode.serviceOwnsHosting()) {
      await _showServiceIdentity();
      DiagLog.log('host', 'startHosting suppressed — service transport owns '
          'hosting (TransportMode); app is UI-only');
      return _password ?? '';
    }
    await stopHosting();
    _resolvedIce = await _resolveIceServers(relayUrl);
    _setupUacBridge();  // Windows host: stream UAC to viewers (no-op elsewhere)

    // Machine-wide identity (multi-user / cross-session): when the SYSTEM helper
    // is installed, it owns a single id + password for the whole machine — every
    // user account shares them, so the box is reachable with the same
    // credentials no matter which user is logged in / active. Falls back to the
    // per-install id + a fresh password when the helper isn't present.
    ({String id, String password})? machine;
    if (_uac.isSupported) {
      machine = await _uac.fetchMachineCreds();
    }

    final pw = (password != null && password.isNotEmpty)
        ? password
        : (machine != null && machine.password.isNotEmpty)
            ? machine.password
            : AuthService.generatePassword();
    _password = pw;
    // Prefer the machine-wide id; else a stable per-install ID (generated once,
    // persisted, reused each launch). Only a reinstall yields a new per-install
    // id; the machine id survives reinstalls (it lives in ProgramData).
    final agentId = fixedAgentId ??
        (machine != null && machine.id.isNotEmpty
            ? machine.id
            : await _persistentAgentId());
    _hostStatus = HostStatus.starting;
    _hostError = null;
    DiagLog.log('host', 'startHosting relay=$relayUrl agentId=$agentId '
        'promptOnConnect=$promptOnConnect unattended=${password != null}');
    notifyListeners();

    final signaling = SignalingService(
      serverUrl: relayUrl,
      onMessage: _onHostMessage,
      onConnected: () {
        DiagLog.log('host', 'signaling connected; registering agentId=$agentId '
            'machineCreds=${machine != null} relay=$relayUrl');
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

  /// Display-only: populate the machine id + password from the SYSTEM helper so
  /// the UI shows the single service-owned host to dial, WITHOUT registering a
  /// host. Used when the service transport owns hosting (TransportMode).
  Future<void> _showServiceIdentity() async {
    if (!_uac.isSupported) return;
    try {
      final machine = await _uac.fetchMachineCreds();
      if (machine != null && machine.id.isNotEmpty) {
        _agentId = machine.id;
        if (machine.password.isNotEmpty) _password = machine.password;
        _hostStatus = HostStatus.online; // reachable via the service transport
        notifyListeners();
      }
    } catch (_) {}
  }

  Future<void> stopHosting() async {
    _statsTimerMaybeStop();
    _stopHostInputWatchdog();
    _routeToHelper = false;
    _helperMoveGraceUntilMs = 0;
    _hostSecureActive = false;
    _hostElevatedActive = false;
    PrivacyMode.set(false); // never leave the host blanked/locked
    for (final peer in _hostPeers.values) {
      await peer.close();
    }
    _hostPeers.clear();
    await _capture.stopCapture();
    await _hostSignaling?.disconnect();
    _hostSignaling = null;
    _agentId = null;
    _discoverTimer?.cancel();
    _discoverTimer = null;
    _serverPeers.clear();
    _hostStatus = HostStatus.offline;
    notifyListeners();
  }

  Future<void> _onHostMessage(SignalingMessage msg) async {
    switch (msg.type) {
      case SignalingMessageType.registered:
        _agentId = msg.payload?['agent_id'] as String?;
        _hostStatus = HostStatus.online;
        DiagLog.log('host', 'registered ok agentId=$_agentId — reachable');
        _startServerDiscovery();
        notifyListeners();
        break;
      case SignalingMessageType.peers:
        _onServerPeers(msg.payload);
        break;
      case SignalingMessageType.connect:
        // A controller wants in. msg.from is the controller's routing id.
        final controllerId = msg.from;
        if (controllerId == null) break;
        DiagLog.log('host', 'incoming connect from=$controllerId '
            'promptOnConnect=$promptOnConnect');
        // Attended: ask the host user first (AnyDesk-style). Unattended access
        // (promptOnConnect=false) accepts immediately with full permissions.
        if (promptOnConnect) {
          // NOTE: the service-host runs HEADLESS as SYSTEM — a consent dialog
          // here is invisible and can never be accepted, so an incoming connect
          // (incl. a viewer auto-reconnecting after a user switch) would hang.
          DiagLog.log('host', 'WARN showing consent prompt — if headless this '
              'cannot be accepted; enable unattended access');
          _pendingConsent = ConsentRequest(controllerId);
          notifyListeners();
        } else {
          // Silent accept (unattended / never-ask) uses the default permissions.
          permControl = defaultPermControl;
          permClipboard = defaultPermClipboard;
          permFiles = defaultPermFiles;
          await _startHostOffer(controllerId);
        }
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
        _disablePrivacyIfNoViewers();
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

  // Poll the relay for LAN-mates every few seconds while we're registered.
  void _startServerDiscovery() {
    _discoverTimer?.cancel();
    _hostSignaling?.sendDiscover();
    _discoverTimer = Timer.periodic(const Duration(seconds: 5), (_) {
      final s = _hostSignaling;
      if (s == null) {
        _discoverTimer?.cancel();
        _discoverTimer = null;
        return;
      }
      s.sendDiscover();
    });
  }

  /// Force an immediate relay discovery poll (the Discovery page refresh button).
  void refreshDiscovery() {
    _serverPeers.clear();
    notifyListeners();
    _hostSignaling?.sendDiscover();
  }

  void _onServerPeers(dynamic payload) {
    if (payload is! Map) return;
    final list = payload['peers'];
    if (list is! List) return;
    final now = DateTime.now();
    final seen = <String>{};
    for (final p in list) {
      if (p is! Map) continue;
      final id = (p['id'] as String?)?.trim() ?? '';
      if (id.isEmpty || id == _agentId) continue;
      seen.add(id);
      final name = (p['hostname'] as String?)?.trim();
      _serverPeers[id] = DiscoveredDevice(
        id: id,
        name: (name == null || name.isEmpty) ? id : name,
        os: (p['os'] as String?) ?? '',
        ip: '',
        lastSeen: now,
      );
    }
    // Drop machines the relay no longer lists (went offline / left the network).
    _serverPeers.removeWhere((id, _) => !seen.contains(id));
    notifyListeners();
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
    // Announce our OS (so the viewer can translate ⌘↔Ctrl) and, if there's more
    // than one monitor, the monitor list so the viewer can switch between them.
    peer.onDataChannelOpen = () async {
      peer.sendData(jsonEncode({'k': 'os', 'v': _osName()}));
      try {
        final mons = await _capture.getSources();
        if (mons.length > 1) {
          peer.sendData(jsonEncode({
            'k': 'mons',
            'l': [
              for (final s in mons) {'id': s.id, 'n': s.name}
            ],
          }));
        }
      } catch (_) {}
    };
    peer.onIceCandidate = (c) =>
        _hostSignaling?.sendCandidate(controllerId, _candidateMap(c));
    peer.onConnectionStateChange = (state) {
      if (state ==
              RTCPeerConnectionState.RTCPeerConnectionStateFailed ||
          state == RTCPeerConnectionState.RTCPeerConnectionStateClosed) {
        _hostPeers.remove(controllerId)?.close();
        _disablePrivacyIfNoViewers();
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
    await disconnectViewer(keepAutoReconnect: true);
    _resolvedIce = await _resolveIceServers(relayUrl);

    // Remember for auto-reconnect.
    _lastRelayUrl = relayUrl;
    _lastTargetId = targetId;
    _lastPassword = password;

    _targetId = targetId;
    _viewerStatus = ViewerStatus.connecting;
    _viewerError = null;
    DiagLog.log('viewer', 'connectToHost target=$targetId relay=$relayUrl '
        'autoReconnect=$autoReconnect tries=$_reconnectTries');
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
          _maybeScheduleReconnect();
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
      _maybeScheduleReconnect();
    }
  }

  /// Sends a remote-control input event to the host. Mouse MOVES go on the
  /// low-latency unreliable channel (stale moves are dropped, so the cursor
  /// doesn't lag); buttons, wheel and keys stay on the reliable channel so they
  /// are never lost or reordered.
  void sendViewerInput(InputEvent event) {
    // Track which keys the host currently believes are held so we can force a
    // release if our window loses focus (see [releaseHeldViewerKeys]). Both
    // input paths — the video's Focus handler and the Windows keyboard hook —
    // funnel through here, so this is the one place that sees every key.
    if (event.kind == 'key') {
      final u = event.data['u'] as int?;
      if (u != null) {
        if (event.data['d'] == true) {
          _heldViewerKeys.add(u);
        } else {
          _heldViewerKeys.remove(u);
        }
      }
    }
    if (event.kind == 'mv') {
      _viewerPeer?.sendCursor(event.encode());
    } else {
      _viewerPeer?.sendData(event.encode());
    }
  }

  // Keys the host currently believes are pressed (by HID usage). Used to release
  // a modifier whose key-up was swallowed by a focus change.
  final Set<int> _heldViewerKeys = {};

  /// Releases every key the host currently thinks is held. Called when the
  /// viewer window loses focus / input is paused, so a modifier (Alt/Ctrl/Shift/
  /// ⌘) whose key-up never arrived can't stay stuck on the host — a stuck Alt
  /// turns every double-click into Alt+double-click, which opens Properties
  /// instead of the file.
  void releaseHeldViewerKeys() {
    if (_heldViewerKeys.isEmpty) return;
    for (final u in _heldViewerKeys.toList()) {
      _viewerPeer?.sendData(InputEvent.key(u, false).encode());
    }
    _heldViewerKeys.clear();
  }

  /// Sends a system key combo to the host by explicit HID usage codes (e.g.
  /// [0xE3, 0x15] = Win+R). Used for shortcuts the LOCAL OS would otherwise
  /// intercept (Win+*, Alt+Tab, …). Codes are sent verbatim — no ⌘↔Ctrl remap
  /// and independent of the local keyboard layout/brand. Press in order,
  /// release in reverse.
  Future<void> sendKeyCombo(List<int> hidUsages) async {
    for (final u in hidUsages) {
      sendViewerInput(InputEvent.key(u, true));
    }
    await Future<void>.delayed(const Duration(milliseconds: 40));
    for (final u in hidUsages.reversed) {
      sendViewerInput(InputEvent.key(u, false));
    }
  }

  Future<void> disconnectViewer({bool keepAutoReconnect = false}) async {
    // A user-initiated disconnect cancels any pending auto-reconnect.
    if (!keepAutoReconnect) {
      autoReconnect = false;
      _reconnectTimer?.cancel();
    }
    _disconnectGrace?.cancel();
    _disconnectGrace = null;
    if (keyboardCapture) {
      keyboardCapture = false;
      _keyHook.setCapture(false);
    }
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
    if (keepAutoReconnect) _maybeScheduleReconnect();
  }

  /// Viewer: reboot the remote host and keep re-dialing until it's back.
  void rebootHost() {
    _viewerPeer?.sendData(jsonEncode({'k': 'cmd', 'c': 'reboot'}));
    autoReconnect = true;
    _reconnectTries = 0;
  }

  // ---- Actions menu (viewer → host), AnyDesk-parity ------------------------

  /// Viewer: lock the remote machine (its sign-in screen).
  void lockRemote() =>
      _viewerPeer?.sendData(jsonEncode({'k': 'cmd', 'c': 'lock'}));

  /// Viewer: sign the remote user out (log off).
  void signOutRemote() =>
      _viewerPeer?.sendData(jsonEncode({'k': 'cmd', 'c': 'logoff'}));

  /// Viewer: send Ctrl+Alt+Del to the remote (routed through its SYSTEM helper
  /// so the real Secure Attention Sequence fires, not an ignored synthetic one).
  void sendCtrlAltDel() =>
      _viewerPeer?.sendData(jsonEncode({'k': 'cmd', 'c': 'sas'}));

  /// Viewer: paste the local clipboard text into the remote's focused field
  /// ("Insert from clipboard"). Types via the host helper so it reaches secure
  /// / elevated windows too.
  Future<bool> insertClipboardToRemote() async {
    try {
      final data = await Clipboard.getData('text/plain');
      final text = data?.text ?? '';
      if (text.isEmpty) return false;
      transmitText(text);
      return true;
    } catch (_) {
      return false;
    }
  }

  /// Viewer: grab the current remote frame as a PNG and save it to Downloads.
  /// Returns the saved path, or null if unavailable.
  Future<String?> captureRemoteScreenshot() async {
    try {
      final track = _remoteStream?.getVideoTracks();
      if (track == null || track.isEmpty) return null;
      final buffer = await track.first.captureFrame();
      final bytes = buffer.asUint8List();
      if (bytes.isEmpty) return null;
      final ts = DateTime.now();
      final name =
          'neev-screenshot-${ts.year}${_two(ts.month)}${_two(ts.day)}-'
          '${_two(ts.hour)}${_two(ts.minute)}${_two(ts.second)}.png';
      final store = FileStore();
      if (!store.supported) return null;
      return await store.saveToDownloads(name, bytes);
    } catch (_) {
      return null;
    }
  }

  String _two(int n) => n.toString().padLeft(2, '0');

  // The viewer's peer dropped (host killed / network lost). Mark failed and
  // start re-dialing the same machine-id. Idempotent — safe to call from both
  // the 'failed'/'closed' path and the 'disconnected' grace timeout.
  void _onViewerConnectionLost() {
    if (_viewerStatus == ViewerStatus.idle) return; // user disconnected on purpose
    DiagLog.log('viewer', 'connection lost — will attempt reconnect');
    _viewerStatus = ViewerStatus.failed;
    _viewerError = 'Connection lost — reconnecting…';
    notifyListeners();
    _maybeScheduleReconnect();
  }

  // Re-dial the same host after an unexpected drop while auto-reconnect is on.
  void _maybeScheduleReconnect() {
    if (!autoReconnect) {
      DiagLog.log('reconnect', 'skipped — autoReconnect off');
      return;
    }
    if (_lastRelayUrl == null || _lastTargetId == null || _lastPassword == null) {
      DiagLog.log('reconnect', 'skipped — no saved target');
      return;
    }
    if (_reconnectTimer?.isActive ?? false) return;
    _reconnectTries++;
    if (_reconnectTries > 90) {
      autoReconnect = false; // give up after a few minutes
      DiagLog.log('reconnect', 'gave up after $_reconnectTries tries');
      return;
    }
    // Fast retries first (snappy user-switch / brief-drop recovery), then back
    // off so a longer outage (e.g. a remote reboot) is still ridden out.
    final delay = _reconnectTries <= 15 ? 2 : 5;
    DiagLog.log('reconnect', 'scheduling try #$_reconnectTries in ${delay}s '
        'target=$_lastTargetId');
    _reconnectTimer = Timer(Duration(seconds: delay), () async {
      if (!autoReconnect || _viewerStatus == ViewerStatus.connected) return;
      try {
        await connectToHost(
          relayUrl: _lastRelayUrl!,
          targetId: _lastTargetId!,
          password: _lastPassword!,
        );
      } catch (_) {
        _maybeScheduleReconnect();
      }
    });
  }

  // Host: run a command sent by the controlling viewer.
  void _onHostCommand(Map<String, dynamic> m) {
    switch (m['c']) {
      case 'reboot':
        rebootMachine();
        break;
      case 'privacy':
        PrivacyMode.set(m['on'] == true);
        break;
      case 'lock':
        lockMachine();
        break;
      case 'logoff':
        signOutMachine();
        break;
      case 'sas': // Ctrl+Alt+Del via the SYSTEM helper (SAS).
        _uac.sendSas();
        break;
    }
  }

  // Safety: never leave the host blanked + input-blocked with no one watching.
  /// Lock this device when the last viewer disconnects (Settings → Security).
  bool lockOnSessionEnd = false;

  void _disablePrivacyIfNoViewers() {
    if (_hostPeers.isEmpty) {
      PrivacyMode.set(false);
      if (lockOnSessionEnd) lockMachine();
    }
  }

  // ---- In-session chat (works both directions over the control channel) ----
  final List<ChatMessage> chatMessages = [];
  int unreadChat = 0;

  /// True when there's a peer to chat with (viewing a host, or hosting with at
  /// least one connected viewer).
  bool get hasChatPeer => _viewerPeer != null || _hostPeers.isNotEmpty;

  /// Send a chat line to the connected peer (host<->viewer).
  void sendChat(String text) {
    final t = text.trim();
    if (t.isEmpty) return;
    chatMessages.add(ChatMessage(t, mine: true));
    final msg = jsonEncode({'k': 'chat', 't': t});
    if (_viewerPeer != null) {
      _viewerPeer!.sendData(msg);
    } else {
      for (final p in _hostPeers.values) {
        p.sendData(msg);
      }
    }
    notifyListeners();
  }

  void markChatRead() {
    if (unreadChat == 0) return;
    unreadChat = 0;
    notifyListeners();
  }

  void _onChat(Map<String, dynamic> m) {
    final t = (m['t'] as String?)?.trim();
    if (t == null || t.isEmpty) return;
    chatMessages.add(ChatMessage(t, mine: false));
    unreadChat++;
    notifyListeners();
  }

  /// Viewer: transmit text to be typed into the host's currently-focused field
  /// (e.g. a UAC / Windows login credential prompt). [tab] presses Tab after
  /// (to jump to the next field), [enter] submits. The host injects it through
  /// the SYSTEM helper so it reaches the secure desktop / elevated windows.
  void transmitText(String text, {bool tab = false, bool enter = false}) {
    if (text.isEmpty && !tab && !enter) return;
    _viewerPeer?.sendData(jsonEncode(
        {'k': 'type', 't': text, 'tab': tab, 'enter': enter}));
  }

  /// Viewer: toggle privacy mode on the host (blank its screen + block its
  /// local input while you control it).
  bool privacyMode = false;
  void setPrivacyMode(bool on) {
    privacyMode = on;
    _viewerPeer?.sendData(jsonEncode({'k': 'cmd', 'c': 'privacy', 'on': on}));
    notifyListeners();
  }

  // Windows viewer: seamless capture of OS-reserved key combos (Win+R, Alt+Tab…)
  // and forward them to the host. Only active while the app is focused.
  late final KeyboardHook _keyHook =
      KeyboardHook((hid, down) => sendViewerInput(InputEvent.key(hid, down)));
  bool keyboardCapture = false;
  bool get keyboardCaptureSupported => KeyboardHook.supported;
  void setKeyboardCapture(bool on) {
    keyboardCapture = on;
    _keyHook.setCapture(on);
    notifyListeners();
  }

  /// Temporarily silence the native key hook + input forwarding while an in-app
  /// text field needs the keyboard (chat, transmit-login dialog), WITHOUT
  /// changing the user's keyboardCapture preference. Restores it on release.
  void pauseKeyboardCapture(bool pause) {
    _keyHook.setCapture(pause ? false : keyboardCapture);
  }

  /// Viewer: ask the host to stream a different monitor.
  void setMonitor(String id) {
    _viewerPeer?.sendData(jsonEncode({'k': 'setmon', 'id': id}));
  }

  // ---- Stream quality presets (viewer-selected → host encoder) -------------
  // 0 = best quality, 1 = balanced, 2 = best performance.
  int _streamQuality = 1;
  int get streamQuality => _streamQuality;

  /// Viewer: pick a quality preset; the host caps its encoder accordingly.
  void setStreamQuality(int preset) {
    _streamQuality = preset.clamp(0, 2);
    notifyListeners();
    _viewerPeer?.sendData(jsonEncode({'k': 'quality', 'p': _streamQuality}));
  }

  // Host: map the viewer's preset to encoder limits and apply to every viewer.
  void _applyHostQuality(int preset) {
    int kbps;
    int fps;
    double scale;
    switch (preset) {
      case 0: // best quality
        kbps = 4000;
        fps = 30;
        scale = 1.0;
        break;
      case 2: // best performance
        kbps = 600;
        fps = 15;
        scale = 1.5;
        break;
      default: // balanced
        kbps = 1500;
        fps = 25;
        scale = 1.0;
    }
    for (final p in _hostPeers.values) {
      p.applyQuality(maxBitrateKbps: kbps, maxFps: fps, scaleDown: scale);
    }
  }

  // Host: re-capture the chosen monitor and hot-swap the video track on every
  // connected viewer (no renegotiation).
  Future<void> _switchMonitor(String? id) async {
    if (id == null) return;
    try {
      final stream = await _capture.startCapture(
          sourceId: id, fps: 30, maxWidth: 1920, maxHeight: 1200);
      final track = stream?.getVideoTracks().isNotEmpty == true
          ? stream!.getVideoTracks().first
          : _capture.videoTrack;
      if (track == null) return;
      for (final peer in _hostPeers.values) {
        await peer.replaceVideoTrack(track);
      }
    } catch (_) {}
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
        DiagLog.log('viewer', 'recv bye reason=${msg.error} '
            'autoReconnect=$autoReconnect status=$_viewerStatus');
        // The relay sends a synthetic bye whenever the HOST's socket drops —
        // which is exactly what a user switch looks like (the SYSTEM service
        // kills + relaunches the host in the new session). That is NOT a
        // deliberate "session ended", so while auto-reconnect is armed treat
        // it as an unexpected drop and keep re-dialing the same machine-id.
        // A deliberate end ('peer_left', e.g. the host rejected the request)
        // still tears down for good — and rejections also arrive before
        // autoReconnect is armed, so old servers without a reason are safe.
        if (autoReconnect && msg.error != 'peer_left') {
          _onViewerConnectionLost();
        } else {
          await disconnectViewer();
        }
        break;
      case SignalingMessageType.error:
        _viewerStatus = ViewerStatus.failed;
        _viewerError = msg.error ?? 'Connection rejected';
        DiagLog.log('viewer', 'recv error="${msg.error}" '
            'autoReconnect=$autoReconnect tries=$_reconnectTries');
        notifyListeners();
        // While riding out a host relaunch (user switch / reboot) the relay
        // answers "agent disconnected" / "agent not found or offline" until
        // the new host re-registers — keep re-dialing through those. Auth
        // failures are terminal: stop, or the retry loop would hammer the
        // relay into its 5-strike password lockout.
        final err = (msg.error ?? '').toLowerCase();
        if (err.contains('password') || err.contains('too many')) {
          autoReconnect = false;
          _reconnectTimer?.cancel();
        } else {
          _maybeScheduleReconnect();
        }
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
      DiagLog.log('viewer', 'connected — remote stream up (session live)');
      // Once a session is actually up, keep it alive across unexpected host
      // drops — most importantly a user switch, where the SYSTEM service kills
      // and relaunches the host in the new session under the SAME machine id.
      // Re-dialing that id reconnects into the new user's desktop (brief drop,
      // then continues). Enabled only after a successful connect, so a bad
      // password never retry-loops. Reset the retry budget on each success.
      autoReconnect = true;
      _reconnectTries = 0;
      _startStatsTimer();
      _ensureClipboardSync();
      // Ask the host to apply our chosen quality preset once streaming starts.
      _viewerPeer?.sendData(jsonEncode({'k': 'quality', 'p': _streamQuality}));
      notifyListeners();
    };
    peer.onIceCandidate = (c) =>
        _viewerSignaling?.sendCandidate(hostId, _candidateMap(c));
    peer.onConnectionStateChange = (state) {
      DiagLog.log('viewer', 'peer state=$state');
      if (state == RTCPeerConnectionState.RTCPeerConnectionStateConnected) {
        // Recovered (or (re)connected) — cancel any pending disconnect grace.
        _disconnectGrace?.cancel();
        _disconnectGrace = null;
        return;
      }
      if (state == RTCPeerConnectionState.RTCPeerConnectionStateFailed ||
          state == RTCPeerConnectionState.RTCPeerConnectionStateClosed) {
        _disconnectGrace?.cancel();
        _disconnectGrace = null;
        _onViewerConnectionLost();
        return;
      }
      // ICE 'disconnected' fires when the host process is killed (e.g. the
      // service relaunches the host on a user switch). WebRTC can linger in this
      // state forever when the peer is truly gone, and it does NOT progress to
      // 'failed' — so without this the viewer stayed disconnected forever. Give
      // it a short grace to self-heal a transient blip; if it hasn't recovered,
      // treat it as lost and let the reconnect loop re-dial the machine-id.
      if (state ==
          RTCPeerConnectionState.RTCPeerConnectionStateDisconnected) {
        _disconnectGrace?.cancel();
        _disconnectGrace = Timer(const Duration(seconds: 3), () {
          if (_viewerStatus != ViewerStatus.connected) {
            _onViewerConnectionLost();
          }
        });
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
      // Master off switch: ignore incoming clipboard when the user disabled sync.
      if (!clipboardSyncEnabled) return;
      if (m['img'] == 1) {
        _recvClipImage(m);
        return;
      }
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

    // Clipboard files (announce-on-copy → deliver-on-paste).
    if (m['k'] == 'clipfann') {
      if (clipboardSyncEnabled) _onClipFilesAnnounced(m);
      return;
    }
    if (m['k'] == 'clipfreq') {
      _onClipFileRequested(m);
      return;
    }
    if (m['k'] == 'clipfdat') {
      _onClipFileData(m);
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
    // Host command (viewer -> host), e.g. reboot.
    if (m['k'] == 'cmd') {
      if (isHost) _onHostCommand(m);
      return;
    }
    // Host's monitor list (host -> viewer).
    if (m['k'] == 'mons') {
      if (!isHost) {
        final l = (m['l'] as List?) ?? const [];
        hostMonitors = [
          for (final e in l)
            if (e is Map)
              {'id': '${e['id']}', 'n': '${e['n']}'}
        ];
        notifyListeners();
      }
      return;
    }
    // Viewer asked to switch the streamed monitor (viewer -> host).
    if (m['k'] == 'setmon') {
      if (isHost) _switchMonitor(m['id'] as String?);
      return;
    }
    // Viewer picked a quality preset (viewer -> host).
    if (m['k'] == 'quality') {
      if (isHost) _applyHostQuality((m['p'] as int?) ?? 1);
      return;
    }
    // In-session chat (either direction).
    if (m['k'] == 'chat') {
      _onChat(m);
      return;
    }
    // Transmit credentials: viewer sends text to type into the host's focused
    // field (UAC / login prompt). Routed through the SYSTEM helper so it reaches
    // the secure desktop / elevated windows.
    if (m['k'] == 'type') {
      if (isHost) {
        _uac.sendTypeText(
          (m['t'] as String?) ?? '',
          tab: m['tab'] == true,
          enter: m['enter'] == true,
        );
      }
      return;
    }

    if (isHost) {
      // NOTE: no host-side "control permission" gate here — it silently dropped
      // ALL input if the flag was ever false (a footgun that broke clicking).
      // View-only is enforced on the VIEWER side (it simply doesn't send input),
      // which is the reliable place for it.
      final event = InputEvent.decode(raw);
      if (event != null) {
        _lastInputMs = _inputClock.elapsedMilliseconds;
        _logHostInput(event);
        // Route BEFORE tracking: _routeInput must see the pre-event held state,
        // so the helper-vs-injector route is latched on the button-DOWN and a
        // down and its matching up can never split across the two injectors.
        _routeInput(event);
        _trackHeldButton(event);
      }
    }
  }

  // Inject one host-side input event. Hover mouse MOVES go to the fast in-app
  // injector: cursor positioning isn't integrity-blocked, and routing the
  // high-rate move stream through the SYSTEM helper (per-event desktop switch +
  // localhost hop) was stalling the cursor. Clicks/keys/wheel go through the
  // helper when connected so they still reach elevated windows.
  //
  // Ordering between the two channels is critical: the helper path has real
  // latency, so a move injected by the fast path can overtake a click still in
  // flight to the helper. The OS then sees down → move(s) → (late) up and turns
  // every click into a drag — apps/icons stick to the cursor instead of
  // opening. So while a button is held, and for a short grace window after the
  // last helper-routed button event, moves ride the helper channel too: the
  // whole gesture stays on one serial, ordered pipe. The click/key route is
  // only re-evaluated while nothing is held (the caller routes button events
  // before tracking them), so a drag never splits across injectors.
  void _routeInput(InputEvent event) {
    // Route EVERYTHING through the SYSTEM helper when either the secure desktop
    // is up (UAC / sign-in / lock / switch-user — only SYSTEM can reach
    // Winlogon) OR the foreground window is elevated/High-IL (the Medium in-app
    // injector is UIPI-blocked from admin windows, so mouse+keys silently do
    // nothing there). Sending the whole gesture on one ordered pipe also avoids
    // the cross-injector reordering that caused click-becomes-drag.
    if ((_hostSecureActive || _hostElevatedActive) && _uac.isConnected) {
      _uac.sendInput(event.data);
      return;
    }
    if (event.kind == 'mv') {
      final inGesture = _heldButtons.isNotEmpty ||
          _inputClock.elapsedMilliseconds < _helperMoveGraceUntilMs;
      if (_routeToHelper && inGesture && _uac.isConnected) {
        _uac.sendInput(event.data);
      } else {
        _injector.inject(event);
      }
      return;
    }
    if (_heldButtons.isEmpty) {
      _routeToHelper = _kRouteNormalInputViaHelper && _uac.isConnected;
    }
    if (_routeToHelper) {
      _uac.sendInput(event.data);
      if (event.kind == 'btn') {
        // Keep moves on the helper channel briefly so they can't be injected
        // ahead of this click by the faster in-app path.
        _helperMoveGraceUntilMs = _inputClock.elapsedMilliseconds + 250;
      }
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
      uacKind = (m['kind'] as int?) ?? 0;
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
  /// Whether the SYSTEM helper (and thus machine-wide multi-user access) is
  /// available on this host.
  bool get machineHelperSupported => _uac.isSupported;

  /// Fetch the machine-wide id + password from the SYSTEM helper, or null when
  /// the helper isn't reachable. Lets the UI show the shared credentials.
  Future<({String id, String password})?> fetchMachineCreds() =>
      _uac.fetchMachineCreds();

  /// Store [password] as the machine-wide password (shared by every account on
  /// this PC). No-op when the helper isn't present.
  void setMachinePassword(String password) =>
      _uac.setMachinePassword(password);

  void _setupUacBridge() {
    if (!_uac.isSupported) return;
    _uac.onActive = (w, h, kind) {
      // Host is now on the secure desktop → route input through the helper
      // (the only injector that reaches Winlogon).
      _hostSecureActive = true;
      _broadcastToPeers(
          jsonEncode({'k': 'uac', 't': 'active', 'w': w, 'h': h, 'kind': kind}));
    };
    _uac.onFrame = _broadcastUacFrame;
    _uac.onGone = () {
      _hostSecureActive = false;
      _broadcastToPeers(jsonEncode({'k': 'uac', 't': 'gone'}));
    };
    // Foreground elevated ↔ route input through the SYSTEM helper (reaches admin
    // windows the Medium in-app injector can't). Local host state only — not
    // broadcast to viewers.
    _uac.onElevated = (elevated) => _hostElevatedActive = elevated;
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
      // mouse doesn't stay stuck (fixes the minimize/maximize freeze). Send the
      // release through BOTH injectors: whichever one is holding the button
      // releases it, and a stray up for an unpressed button is a no-op — while
      // a release lost to a half-dead helper socket would leave the host
      // dragging forever.
      for (final b in _heldButtons.toList()) {
        final up = InputEvent.button(b, false);
        _routeInput(up);
        if (_routeToHelper) _injector.inject(up);
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
      // Master off switch: don't read the local clipboard at all when disabled.
      if (!clipboardSyncEnabled) return;
      await _pollClipText();
      _clipTick++;
      if (_clipTick.isEven) await _pollClipImage(); // images ~every 1.2s
      if (_clipTick % 3 == 0) await _pollClipFiles(); // files ~every 1.8s
    });
  }

  Future<void> _pollClipText() async {
    String? text;
    try {
      final data = await Clipboard.getData('text/plain');
      text = data?.text;
    } catch (_) {
      return;
    }
    if (text == null || text.isEmpty || text == _lastClip) return;
    _lastClip = text;
    _broadcastClip(text);
  }

  Future<void> _pollClipImage() async {
    Uint8List? img;
    try {
      img = await Pasteboard.image;
    } catch (_) {
      return;
    }
    if (img == null || img.isEmpty) return;
    final h = _imgHash(img);
    if (h == _lastClipImgHash) return;
    _lastClipImgHash = h;
    _broadcastClipImage(img);
  }

  // Cheap change-detector for clipboard images (not cryptographic).
  int _imgHash(Uint8List b) {
    if (b.isEmpty) return 0;
    return b.length ^ (b.first << 8) ^ (b[b.length >> 1] << 16) ^ (b.last << 24);
  }

  void _broadcastClipImage(Uint8List bytes) {
    final b64 = base64Encode(bytes);
    const chunk = 48 * 1024;
    final total = (b64.length / chunk).ceil().clamp(1, 1 << 20);
    for (var i = 0; i < total; i++) {
      final start = i * chunk;
      final end = start + chunk < b64.length ? start + chunk : b64.length;
      final msg = jsonEncode({
        'k': 'clip',
        'img': 1,
        'i': i,
        'n': total,
        'd': b64.substring(start, end),
      });
      for (final peer in _hostPeers.values) {
        peer.sendData(msg);
      }
      _viewerPeer?.sendData(msg);
    }
  }

  void _recvClipImage(Map<String, dynamic> m) {
    final i = m['i'] as int?;
    final n = m['n'] as int?;
    final d = m['d'] as String?;
    if (i == null || n == null || d == null) return;
    if (i == 0) {
      _clipImgBuf.clear();
      _clipImgNext = 0;
      _clipImgTotal = n;
    }
    if (i == _clipImgNext && n == _clipImgTotal) {
      _clipImgBuf.write(d);
      _clipImgNext++;
      if (_clipImgNext == _clipImgTotal) {
        try {
          final bytes = base64Decode(_clipImgBuf.toString());
          _lastClipImgHash = _imgHash(bytes); // don't echo it straight back
          Pasteboard.writeImage(bytes);
        } catch (_) {}
        _clipImgBuf.clear();
        _clipImgNext = 0;
      }
    } else {
      _clipImgBuf.clear();
      _clipImgNext = 0;
    }
  }

  // Detect files freshly copied to the local clipboard and mirror them to the
  // peer's clipboard. Small files only (chunked base64 over the data channel).
  static const int _clipFileMaxBytes = 64 * 1024 * 1024; // 64 MB cap
  Future<void> _pollClipFiles() async {
    if (_clipFileSuppress > 0) {
      _clipFileSuppress--;
      return;
    }
    List<String> paths;
    try {
      // User-context agent first (reads the file clipboard even on a SYSTEM
      // host); fall back to the in-process clipboard when there's no agent.
      paths = await _clipAgent.readFiles() ?? await Pasteboard.files();
    } catch (_) {
      return;
    }
    if (paths.isEmpty) {
      _lastClipFiles = const [];
      return;
    }
    // Only react to a *change* (a fresh Ctrl+C), never re-send the same set.
    if (paths.length == _lastClipFiles.length) {
      var same = true;
      for (var i = 0; i < paths.length; i++) {
        if (paths[i] != _lastClipFiles[i]) {
          same = false;
          break;
        }
      }
      if (same) return;
    }
    _lastClipFiles = List.of(paths);
    // ANNOUNCE-ON-COPY (AnyDesk model): send only names + sizes now, keep the
    // paths locally, and stream the bytes only when the other side actually
    // pastes (a 'clipfreq' comes back). No bytes cross the wire on copy.
    final entries = <Map<String, Object>>[];
    final kept = <String>[];
    for (final p in paths) {
      try {
        final len = await XFile(p).length();
        if (len > _clipFileMaxBytes) continue; // too big to mirror on paste
        final name = p.split(RegExp(r'[\\/]')).last;
        if (name.isEmpty) continue;
        entries.add({'name': name, 'size': len});
        kept.add(p);
      } catch (_) {
        // Directory / unreadable — skip (folder copy isn't supported).
      }
    }
    if (entries.isEmpty) return;
    _clipOutToken++;
    // Tag with this instance's identity so a host token can never collide with a
    // viewer token (both sides may announce over the same pair of channels).
    final token = 'c${identityHashCode(this)}_$_clipOutToken';
    _clipOutFiles[token] = kept; // serve these when a paste requests them
    // Bound memory: only keep the few most recent announced sets around.
    if (_clipOutFiles.length > 8) {
      _clipOutFiles.remove(_clipOutFiles.keys.first);
    }
    _sendClipCtl(jsonEncode({'k': 'clipfann', 'token': token, 'files': entries}));
  }

  // Send a clipboard-file control message on the reliable file channel to
  // whichever peer(s) we're connected to (host has many viewers; viewer has one).
  void _sendClipCtl(String msg) {
    for (final peer in _hostPeers.values) {
      peer.sendFileData(msg);
    }
    _viewerPeer?.sendFileData(msg);
  }

  // Destination: an announcement arrived. On attended Windows, place a
  // delayed-render virtual-file set on the clipboard so paste pulls bytes on
  // demand. Elsewhere (macOS / Linux / SYSTEM host) delayed rendering isn't
  // available, so eagerly fetch the bytes now and stage real files for paste.
  Future<void> _onClipFilesAnnounced(Map<String, dynamic> m) async {
    final token = m['token'] as String?;
    final raw = m['files'];
    if (token == null || raw is! List) return;
    final files = <Map<String, Object>>[];
    final names = <String>[];
    for (final f in raw) {
      if (f is Map) {
        final name = f['name'] as String?;
        final size = (f['size'] as num?)?.toInt() ?? 0;
        if (name != null && name.isNotEmpty) {
          files.add({'name': name, 'size': size});
          names.add(name);
        }
      }
    }
    if (files.isEmpty) return;
    _clipRecvNames[token] = names;
    if (_clipRecvNames.length > 8) {
      _clipRecvNames.remove(_clipRecvNames.keys.first);
    }

    if (ClipboardWriter.isSupported) {
      final ok = await ClipboardWriter.announceRemoteFiles(token, files);
      if (ok) {
        _clipNativeTokens.add(token);
        _startClipFetchPoller(); // paste will raise native fetch requests
        return;
      }
      // announce failed → fall through to eager fetch
    }
    // Fallback: pull every file now and stage it for a normal Ctrl+V.
    for (var i = 0; i < files.length; i++) {
      _requestClipFile(token, i);
    }
  }

  // Destination (Windows): poll the native delayed-render object for the file
  // indices the shell asked for on paste, and request those bytes from the peer.
  void _startClipFetchPoller() {
    if (_clipFetchPoller != null) return;
    _clipFetchPoller =
        Timer.periodic(const Duration(milliseconds: 150), (_) async {
      if (_hostPeers.isEmpty && _viewerPeer == null) {
        _stopClipFetchPoller();
        return;
      }
      final reqs = await ClipboardWriter.pollFileRequests();
      for (final r in reqs) {
        final token = r['token'] as String?;
        final index = r['index'] as int?;
        if (token != null && index != null) _requestClipFile(token, index);
      }
    });
  }

  void _stopClipFetchPoller() {
    _clipFetchPoller?.cancel();
    _clipFetchPoller = null;
  }

  void _requestClipFile(String token, int index) {
    _sendClipCtl(jsonEncode({'k': 'clipfreq', 'token': token, 'index': index}));
  }

  // Source: a paste on the other side wants the bytes for one announced file.
  // Read it now and stream it back in chunks (this is the ONLY time bytes move).
  Future<void> _onClipFileRequested(Map<String, dynamic> m) async {
    final token = m['token'] as String?;
    final index = m['index'] as int?;
    if (token == null || index == null) return;
    final paths = _clipOutFiles[token];
    Uint8List? bytes;
    if (paths != null && index >= 0 && index < paths.length) {
      try {
        bytes = await XFile(paths[index]).readAsBytes();
      } catch (_) {}
    }
    if (bytes == null) {
      _sendClipCtl(jsonEncode({
        'k': 'clipfdat',
        'token': token,
        'index': index,
        'ok': false,
        'seq': 0,
        'total': 1,
      }));
      return;
    }
    final b64 = base64Encode(bytes);
    const chunk = 48 * 1024;
    final total = (b64.length / chunk).ceil().clamp(1, 1 << 24);
    for (var i = 0; i < total; i++) {
      final start = i * chunk;
      final end = start + chunk < b64.length ? start + chunk : b64.length;
      _sendClipCtl(jsonEncode({
        'k': 'clipfdat',
        'token': token,
        'index': index,
        'ok': true,
        'seq': i,
        'total': total,
        'd': b64.substring(start, end),
      }));
    }
  }

  // Destination: reassemble streamed file bytes, then hand them to the paste
  // (native delayed-render) or stage them on disk (eager fallback).
  void _onClipFileData(Map<String, dynamic> m) {
    final token = m['token'] as String?;
    final index = m['index'] as int?;
    if (token == null || index == null) return;
    final key = '$token#$index';
    if (m['ok'] == false) {
      _clipRecv.remove(key);
      _completeClipRecv(token, index, false, null);
      return;
    }
    final seq = m['seq'] as int?;
    final total = m['total'] as int?;
    final d = m['d'] as String?;
    if (seq == null || total == null || d == null) return;
    var rec = _clipRecv[key];
    if (seq == 0) {
      rec = _ClipRecv(total);
      _clipRecv[key] = rec;
    }
    if (rec == null || total != rec.total || seq != rec.next) {
      _clipRecv.remove(key);
      return;
    }
    rec.buf.write(d);
    rec.next++;
    if (rec.next == rec.total) {
      _clipRecv.remove(key);
      Uint8List? bytes;
      try {
        bytes = base64Decode(rec.buf.toString());
      } catch (_) {}
      _completeClipRecv(token, index, bytes != null, bytes);
    }
  }

  Future<void> _completeClipRecv(
      String token, int index, bool ok, Uint8List? bytes) async {
    // Windows delayed-render: unblock the pending paste with the bytes.
    if (_clipNativeTokens.contains(token)) {
      await ClipboardWriter.deliverRemoteFileBytes(
          token, index, ok, bytes ?? Uint8List(0));
      return;
    }
    // Eager fallback (non-Windows): stage the file and put it on the clipboard.
    if (!ok || bytes == null) return;
    final names = _clipRecvNames[token];
    final name = (names != null && index < names.length)
        ? names[index]
        : 'file_$index';
    try {
      final path = await FileStore().saveToTemp(name, bytes);
      if (path != null) await _onClipboardFileReceived(path);
    } catch (_) {}
  }

  void _stopClipboardSync() {
    _clipTimer?.cancel();
    _clipTimer = null;
    _stopClipFetchPoller();
    // Fail any paste still blocked in the native layer so Explorer doesn't hang.
    for (final key in _clipRecv.keys.toList()) {
      final sep = key.lastIndexOf('#');
      if (sep <= 0) continue;
      final token = key.substring(0, sep);
      final index = int.tryParse(key.substring(sep + 1));
      if (index != null && _clipNativeTokens.contains(token)) {
        ClipboardWriter.deliverRemoteFileBytes(token, index, false, Uint8List(0));
      }
    }
    _clipRecv.clear();
    _clipOutFiles.clear();
    _clipNativeTokens.clear();
    _clipRecvNames.clear();
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

  /// The real machine hostname (via dart:io on native targets, so Discovery
  /// on other machines shows "DESKTOP-AB12CD" instead of a generic label);
  /// falls back to "`<os>`-host" on web or if the lookup fails.
  String _hostname() {
    final name = host_name.localHostname();
    return name.isEmpty ? '${_osName()}-host' : name;
  }

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
