import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'core/theme/app_theme.dart';
import 'core/diag_log.dart';
import 'window_manager.dart' show initWindowManager;
import 'presentation/pages/connect_page.dart';
import 'data/services/consent_flag_io.dart' show AppOpenBeacon;

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  DiagLog.init();
  await initWindowManager();
  // Restore the saved theme BEFORE the first frame so there's no flash.
  await restoreAppTheme();
  // Tell the headless transport this app is running, so Interactive Access =
  // "only while the app is open" can actually mean that.
  await AppOpenBeacon.start();
  runApp(const ProviderScope(child: NeevRemoteApp()));
}

class NeevRemoteApp extends StatelessWidget {
  const NeevRemoteApp({super.key});

  @override
  Widget build(BuildContext context) {
    // Rebuild the whole app when the theme flips — AppColors getters read the
    // active palette, so every widget re-colors on the next frame.
    return ValueListenableBuilder<bool>(
      valueListenable: themeIsDark,
      builder: (_, __, ___) => MaterialApp(
        title: 'Neev Remote',
        debugShowCheckedModeBanner: false,
        theme: lightTheme(),
        home: const ConnectPage(),
      ),
    );
  }
}
