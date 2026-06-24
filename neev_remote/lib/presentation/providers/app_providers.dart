import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../../data/services/remote_service.dart';

/// The signaling server URL to use by default.
///
/// On the web the app must dial the SAME origin it was served from — otherwise
/// "localhost" resolves to the *viewer's* machine, not the server. On desktop
/// there is no serving origin, so we fall back to localhost for dev.
String defaultRelayUrl() {
  if (kIsWeb) {
    final base = Uri.base; // the page URL the app was loaded from
    final scheme = base.scheme == 'https' ? 'wss' : 'ws';
    return '$scheme://${base.authority}/ws';
  }
  return 'ws://localhost:8080/ws';
}

// --- Core session service ---

/// The single orchestrator that owns signaling, WebRTC and screen capture for
/// both host and viewer roles. Lives for the lifetime of the app.
final remoteServiceProvider = ChangeNotifierProvider<RemoteService>((ref) {
  final service = RemoteService();
  ref.onDispose(service.dispose);
  return service;
});

// --- Settings ---

class AppSettings {
  final String relayUrl;
  final int videoBitrate;
  final int videoFps;
  final bool autoAnswer;
  final bool startOnBoot;
  final bool viewOnly;

  const AppSettings({
    this.relayUrl = '',
    this.videoBitrate = 1500,
    this.videoFps = 30,
    this.autoAnswer = false,
    this.startOnBoot = false,
    this.viewOnly = false,
  });

  AppSettings copyWith({
    String? relayUrl,
    int? videoBitrate,
    int? videoFps,
    bool? autoAnswer,
    bool? startOnBoot,
    bool? viewOnly,
  }) {
    return AppSettings(
      relayUrl: relayUrl ?? this.relayUrl,
      videoBitrate: videoBitrate ?? this.videoBitrate,
      videoFps: videoFps ?? this.videoFps,
      autoAnswer: autoAnswer ?? this.autoAnswer,
      startOnBoot: startOnBoot ?? this.startOnBoot,
      viewOnly: viewOnly ?? this.viewOnly,
    );
  }
}

final settingsProvider =
    StateNotifierProvider<SettingsNotifier, AppSettings>((ref) {
  return SettingsNotifier();
});

class SettingsNotifier extends StateNotifier<AppSettings> {
  SettingsNotifier() : super(AppSettings(relayUrl: defaultRelayUrl())) {
    _load();
  }

  static const _kRelay = 'relayUrl';
  static const _kBitrate = 'videoBitrate';
  static const _kFps = 'videoFps';
  static const _kViewOnly = 'viewOnly';

  Future<void> _load() async {
    final prefs = await SharedPreferences.getInstance();
    state = state.copyWith(
      relayUrl: prefs.getString(_kRelay),
      videoBitrate: prefs.getInt(_kBitrate),
      videoFps: prefs.getInt(_kFps),
      viewOnly: prefs.getBool(_kViewOnly),
    );
  }

  Future<void> _save() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_kRelay, state.relayUrl);
    await prefs.setInt(_kBitrate, state.videoBitrate);
    await prefs.setInt(_kFps, state.videoFps);
    await prefs.setBool(_kViewOnly, state.viewOnly);
  }

  void updateRelayUrl(String url) {
    state = state.copyWith(relayUrl: url);
    _save();
  }

  void updateVideoBitrate(int bitrate) {
    state = state.copyWith(videoBitrate: bitrate);
    _save();
  }

  void updateVideoFps(int fps) {
    state = state.copyWith(videoFps: fps);
    _save();
  }

  void toggleAutoAnswer() {
    state = state.copyWith(autoAnswer: !state.autoAnswer);
  }

  void toggleStartOnBoot() {
    state = state.copyWith(startOnBoot: !state.startOnBoot);
  }

  void toggleViewOnly() {
    state = state.copyWith(viewOnly: !state.viewOnly);
    _save();
  }
}

// --- Recent connections ---

class RecentConnection {
  final String id;
  final String name;
  final String? ipAddress;
  final DateTime lastConnected;

  RecentConnection({
    required this.id,
    required this.name,
    this.ipAddress,
    required this.lastConnected,
  });
}

final recentConnectionsProvider =
    StateNotifierProvider<RecentConnectionsNotifier, List<RecentConnection>>(
        (ref) {
  return RecentConnectionsNotifier();
});

class RecentConnectionsNotifier extends StateNotifier<List<RecentConnection>> {
  RecentConnectionsNotifier() : super([]);

  void addConnection(RecentConnection connection) {
    state = [
      connection,
      ...state.where((c) => c.id != connection.id),
    ].take(10).toList();
  }

  void removeConnection(String id) {
    state = state.where((c) => c.id != id).toList();
  }

  void clear() {
    state = [];
  }
}
