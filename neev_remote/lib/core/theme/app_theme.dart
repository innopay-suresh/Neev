import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// Warm bento design system (v3) — cream canvas, white cards, one coral accent.
/// See DESIGN.md. Token NAMES are unchanged from v2 on purpose so every existing
/// widget keeps compiling; only the values moved.
///
/// The v2 canvas (#F8F8F9) sat ~3% off white, so cards never read as cards, and
/// the border (#ECECEC) was effectively invisible — that's what made the app look
/// flat. The warm canvas also separates us from AnyDesk/TeamViewer/Splashtop,
/// which all ship cool grey.
/// One full token set. Two instances below: Nova dark (user spec 2026-07-24)
/// and the v3 warm-light. AppColors getters read the ACTIVE one, so every
/// widget re-colors on toggle with zero wiring changes.
class _Palette {
  const _Palette({
    required this.primary,
    required this.primaryHover,
    required this.primaryDark,
    required this.primarySoft,
    required this.secondary,
    required this.secondarySoft,
    required this.deviceNavy,
    required this.deviceForest,
    required this.devicePlum,
    required this.deviceWalnut,
    required this.background,
    required this.surface,
    required this.surfaceLight,
    required this.surfaceAlt,
    required this.textPrimary,
    required this.textSecondary,
    required this.textTertiary,
    required this.success,
    required this.successSoft,
    required this.warning,
    required this.error,
    required this.infoSlate,
    required this.border,
    required this.borderStrong,
    required this.inkBand,
    required this.inkBandAlt,
  });
  final Color primary, primaryHover, primaryDark, primarySoft;
  final Color secondary, secondarySoft;
  final Color deviceNavy, deviceForest, devicePlum, deviceWalnut;
  final Color background, surface, surfaceLight, surfaceAlt;
  final Color textPrimary, textSecondary, textTertiary;
  final Color success, successSoft, warning, error, infoSlate;
  final Color border, borderStrong, inkBand, inkBandAlt;
}

/// Nova dark (user spec: bg #0F1115, cards #171A21, borders #2A2F3A, #FF6B00).
const _Palette _novaDark = _Palette(
  primary: Color(0xFFFF6B00),
  primaryHover: Color(0xFFFF8F3D),
  primaryDark: Color(0xFFFF8F3D),
  primarySoft: Color(0x24FF6B00),
  secondary: Color(0xFF1F2530),
  secondarySoft: Color(0xFF171A21),
  deviceNavy: Color(0xFF1E2A3D),
  deviceForest: Color(0xFF1D3328),
  devicePlum: Color(0xFF33202E),
  deviceWalnut: Color(0xFF2E2519),
  background: Color(0xFF0F1115),
  surface: Color(0xFF171A21),
  surfaceLight: Color(0xFF1D212B),
  surfaceAlt: Color(0xFF1A1E27),
  textPrimary: Color(0xFFF3F5F8),
  textSecondary: Color(0xFF98A2B3),
  textTertiary: Color(0xFF636B7A),
  success: Color(0xFF22C55E),
  successSoft: Color(0x2622C55E),
  warning: Color(0xFFF5A623),
  error: Color(0xFFEF4444),
  infoSlate: Color(0xFF7C8AA0),
  border: Color(0xFF2A2F3A),
  borderStrong: Color(0xFF39404E),
  inkBand: Color(0xFF1D212B),
  inkBandAlt: Color(0xFF242A36),
);

/// Nova light — the mockup's warm-light look with the same #FF6B00 accent.
const _Palette _novaLight = _Palette(
  primary: Color(0xFFFF6B00),
  primaryHover: Color(0xFFFF8F3D),
  primaryDark: Color(0xFFC94418),
  primarySoft: Color(0xFFFFE8D6),
  secondary: Color(0xFF243B53),
  secondarySoft: Color(0xFFE4E9EF),
  deviceNavy: Color(0xFF243B53),
  deviceForest: Color(0xFF294B3A),
  devicePlum: Color(0xFF543246),
  deviceWalnut: Color(0xFF554332),
  background: Color(0xFFF7F3EC),
  surface: Color(0xFFFFFEFB),
  surfaceLight: Color(0xFFF6F1E7),
  surfaceAlt: Color(0xFFF6F1E7),
  textPrimary: Color(0xFF1A1A1E),
  textSecondary: Color(0xFF6E675B),
  textTertiary: Color(0xFF9A9385),
  success: Color(0xFF198764),
  successSoft: Color(0xFFDDEFE7),
  warning: Color(0xFFD78A18),
  error: Color(0xFFD8493F),
  infoSlate: Color(0xFF53616D),
  border: Color(0xFFE2DACB),
  borderStrong: Color(0xFFD0C6AC),
  inkBand: Color(0xFF1A1A1E),
  inkBandAlt: Color(0xFF2A2720),
);

/// Rebuilds the MaterialApp when the theme flips (value = isDark).
final ValueNotifier<bool> themeIsDark = ValueNotifier<bool>(true);

/// Restores the saved theme (call before the first frame — no flash).
Future<void> restoreAppTheme() async {
  try {
    final prefs = await SharedPreferences.getInstance();
    themeIsDark.value = prefs.getBool('darkTheme') ?? true;
  } catch (_) {}
}

/// Flips dark ⇄ light and persists the choice.
Future<void> toggleAppTheme() async {
  themeIsDark.value = !themeIsDark.value;
  try {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setBool('darkTheme', themeIsDark.value);
  } catch (_) {}
}

/// Token surface — NAMES unchanged from every earlier version, so all widgets
/// keep compiling; values now come from the active palette (dark ⇄ light).
class AppColors {
  static _Palette get _c => themeIsDark.value ? _novaDark : _novaLight;
  static bool get isDark => themeIsDark.value;

  static Color get primary => _c.primary;
  static Color get primaryHover => _c.primaryHover;
  static Color get primaryDark => _c.primaryDark;
  static Color get primarySoft => _c.primarySoft;
  static Color get accent => _c.primary;
  static Color get accentDark => _c.primaryDark;
  static Color get accentSoft => _c.primarySoft;

  static Color get secondary => _c.secondary;
  static Color get secondarySoft => _c.secondarySoft;

  static Color get deviceNavy => _c.deviceNavy;
  static Color get deviceForest => _c.deviceForest;
  static Color get devicePlum => _c.devicePlum;
  static Color get deviceWalnut => _c.deviceWalnut;

  static Color get background => _c.background;
  static Color get surface => _c.surface;
  static Color get surfaceLight => _c.surfaceLight;
  static Color get surfaceAlt => _c.surfaceAlt;

  static Color get textPrimary => _c.textPrimary;
  static Color get textSecondary => _c.textSecondary;
  static Color get textTertiary => _c.textTertiary;

  static Color get success => _c.success;
  static Color get successSoft => _c.successSoft;
  static Color get warning => _c.warning;
  static Color get error => _c.error;
  static Color get infoSlate => _c.infoSlate;

  static Color get border => _c.border;
  static Color get borderStrong => _c.borderStrong;

  static Color get inkBand => _c.inkBand;
  static Color get inkBandAlt => _c.inkBandAlt;
}

/// Deep shadows for the dark canvas.
class AppShadows {
  static const List<BoxShadow> card = [
    BoxShadow(color: Color(0x40000000), blurRadius: 2, offset: Offset(0, 1)),
    BoxShadow(color: Color(0x59000000), blurRadius: 16, offset: Offset(0, 8)),
  ];
  static const List<BoxShadow> soft = [
    BoxShadow(color: Color(0x40000000), blurRadius: 8, offset: Offset(0, 3)),
  ];
  static const List<BoxShadow> float = [
    BoxShadow(color: Color(0x4D000000), blurRadius: 6, offset: Offset(0, 2)),
    BoxShadow(color: Color(0x66000000), blurRadius: 34, offset: Offset(0, 18)),
  ];
  static const List<BoxShadow> dock = [
    BoxShadow(color: Color(0x59000000), blurRadius: 14, offset: Offset(0, 5)),
    BoxShadow(color: Color(0x73000000), blurRadius: 54, offset: Offset(0, 22)),
  ];
  static const List<BoxShadow> cardHover = [
    BoxShadow(color: Color(0x66000000), blurRadius: 24, offset: Offset(0, 10)),
    BoxShadow(color: Color(0x80000000), blurRadius: 64, offset: Offset(0, 28)),
  ];
}

/// Radii — see DESIGN.md.
class AppRadii {
  static const double sm = 6;
  static const double md = 9;
  static const double lg = 12;
  static const double xl = 15;
  static const double card = 18; // device cards (Command Center)
  static const double panel = 24; // connection dock, large panels, modals
}

/// Bundled fonts (pubspec assets). NOT system fonts: the v2 stack asked for
/// 'Segoe UI Variable Text', which does not exist on macOS, so the Mac build
/// silently fell back to a generic sans and had no typographic identity.
const String _fontFamily = 'Inter'; // body / UI
const String kFontDisplay = 'SpaceGrotesk'; // titles, stat values
const String kFontMono = 'JetBrainsMono'; // device IDs, passwords, data
const List<String> _fontFallback = <String>[
  'SpaceGrotesk',
  'sans-serif',
];

/// Typography — Segoe UI Variable, medium weight, no heavy bold.
/// (Getters, so they compose with the theme's default font.)
class AppTypography {
  static TextStyle get display => TextStyle(
      fontSize: 26,
      fontWeight: FontWeight.w600,
      letterSpacing: -0.3,
      color: AppColors.textPrimary);

  static TextStyle get heading1 => TextStyle(
      fontSize: 24,
      fontWeight: FontWeight.w600,
      letterSpacing: -0.2,
      color: AppColors.textPrimary);

  static TextStyle get heading2 => TextStyle(
      fontSize: 18, fontWeight: FontWeight.w600, color: AppColors.textPrimary);

  static TextStyle get title => TextStyle(
      fontSize: 15, fontWeight: FontWeight.w600, color: AppColors.textPrimary);

  static TextStyle get body => TextStyle(
      fontSize: 14, fontWeight: FontWeight.w500, color: AppColors.textPrimary);

  static TextStyle get bodyStrong => TextStyle(
      fontSize: 14, fontWeight: FontWeight.w600, color: AppColors.textPrimary);

  static TextStyle get caption => TextStyle(
      fontSize: 12, fontWeight: FontWeight.w500, color: AppColors.textSecondary);

  static TextStyle get label => TextStyle(
      fontSize: 12,
      fontWeight: FontWeight.w600,
      letterSpacing: 0.1,
      color: AppColors.textSecondary);

  // ---- Bento additions (DESIGN.md) ----

  /// Page + section titles, stat values. Space Grotesk.
  static TextStyle get pageTitle => TextStyle(
      fontFamily: kFontDisplay,
      fontSize: 19,
      fontWeight: FontWeight.w600,
      color: AppColors.textPrimary);

  static TextStyle get sectionTitle => TextStyle(
      fontFamily: kFontDisplay,
      fontSize: 15,
      fontWeight: FontWeight.w600,
      color: AppColors.textPrimary);

  static TextStyle get cardTitle => TextStyle(
      fontFamily: kFontDisplay,
      fontSize: 13,
      fontWeight: FontWeight.w600,
      color: AppColors.textPrimary);

  /// Device IDs / passwords / build stamps. Tabular so digits don't jitter.
  static TextStyle get idLarge => TextStyle(
      fontFamily: kFontMono,
      fontSize: 14,
      fontWeight: FontWeight.w600,
      letterSpacing: 0.3,
      fontFeatures: [FontFeature.tabularFigures()],
      color: AppColors.textPrimary);

  static TextStyle get mono => TextStyle(
      fontFamily: kFontMono,
      fontSize: 12.5,
      fontWeight: FontWeight.w500,
      fontFeatures: [FontFeature.tabularFigures()],
      color: AppColors.textPrimary);

  /// Tiny uppercase field labels ("YOUR ID", "PASSWORD").
  static TextStyle get microLabel => TextStyle(
      fontSize: 9,
      fontWeight: FontWeight.w500,
      letterSpacing: 0.6,
      color: AppColors.textTertiary);

  /// Row meta under a tile ("Reception-PC · 2 days ago").
  static TextStyle get meta => TextStyle(
      fontSize: 10.5,
      fontWeight: FontWeight.w400,
      color: AppColors.textTertiary);
}

/// Spacing — 8px grid (with 4px half-steps).
class AppSpacing {
  static const double xs = 4;
  static const double sm = 8;
  static const double md = 12;
  static const double lg = 16;
  static const double xl = 24;
  static const double xxl = 32;
}

/// Border radius. v2: inputs 10, buttons 12, cards 18.
class AppRadius {
  static const double xs = 6;
  static const double sm = 8;
  static const double input = 10;
  static const double md = 12; // buttons
  static const double lg = 16;
  static const double card = 18;
  static const double xl = 20;
  static const double pill = 999;
}

/// Nova dark theme — dark-first (forced regardless of OS brightness).
ThemeData lightTheme() {
  final dark = AppColors.isDark;
  final base = ThemeData(
      useMaterial3: true,
      brightness: dark ? Brightness.dark : Brightness.light);
  final textTheme = base.textTheme
      .apply(
        fontFamily: _fontFamily,
        fontFamilyFallback: _fontFallback,
        bodyColor: AppColors.textPrimary,
        displayColor: AppColors.textPrimary,
      )
      .copyWith(
        headlineLarge: AppTypography.heading1,
        headlineMedium: AppTypography.heading2,
        titleMedium: AppTypography.title,
        bodyLarge: AppTypography.body,
        bodyMedium: AppTypography.body,
        bodySmall: AppTypography.caption,
        labelLarge: AppTypography.bodyStrong,
      );

  return base.copyWith(
    scaffoldBackgroundColor: AppColors.background,
    textTheme: textTheme,
    primaryTextTheme: textTheme,
    colorScheme: (dark ? const ColorScheme.dark() : const ColorScheme.light()).copyWith(
      primary: AppColors.primary,
      secondary: AppColors.accent,
      surface: AppColors.surface,
      error: AppColors.error,
      onPrimary: Colors.white,
      onSecondary: Colors.white,
      onSurface: AppColors.textPrimary,
      onError: Colors.white,
    ),
    appBarTheme: AppBarTheme(
      backgroundColor: AppColors.surface,
      surfaceTintColor: Colors.transparent,
      foregroundColor: AppColors.textPrimary,
      elevation: 0,
      scrolledUnderElevation: 0,
      centerTitle: false,
      titleTextStyle: AppTypography.heading2,
    ),
    cardTheme: CardThemeData(
      color: AppColors.surface,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(AppRadius.card),
        side: BorderSide(color: AppColors.border, width: 1),
      ),
    ),
    // Primary button — 48px, solid violet, radius 12, no gradient.
    elevatedButtonTheme: ElevatedButtonThemeData(
      style: ElevatedButton.styleFrom(
        backgroundColor: AppColors.primary,
        foregroundColor: Colors.white,
        disabledBackgroundColor: AppColors.primary.withValues(alpha: 0.4),
        disabledForegroundColor: Colors.white,
        elevation: 0,
        minimumSize: const Size(0, 48),
        textStyle: AppTypography.bodyStrong,
        padding: const EdgeInsets.symmetric(horizontal: AppSpacing.xl),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppRadius.md),
        ),
      ).copyWith(
        overlayColor: WidgetStateProperty.resolveWith((s) {
          if (s.contains(WidgetState.pressed)) {
            return Colors.white.withValues(alpha: 0.18);
          }
          if (s.contains(WidgetState.hovered)) {
            return Colors.white.withValues(alpha: 0.10);
          }
          return null;
        }),
      ),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: AppColors.primary,
        foregroundColor: Colors.white,
        minimumSize: const Size(0, 48),
        textStyle: AppTypography.bodyStrong,
        padding: const EdgeInsets.symmetric(horizontal: AppSpacing.xl),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppRadius.md),
        ),
      ),
    ),
    // Secondary — outline.
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: AppColors.textPrimary,
        backgroundColor: AppColors.surface,
        minimumSize: const Size(0, 44),
        textStyle: AppTypography.bodyStrong,
        side: BorderSide(color: AppColors.borderStrong),
        padding: const EdgeInsets.symmetric(horizontal: AppSpacing.lg),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppRadius.md),
        ),
      ),
    ),
    // Ghost — transparent.
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        foregroundColor: AppColors.accentDark,
        textStyle: AppTypography.bodyStrong,
        minimumSize: const Size(0, 40),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(AppRadius.sm),
        ),
      ),
    ),
    // Inputs — 44px, rounded, violet focus ring, leading icon.
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: AppColors.surfaceAlt,
      hintStyle: AppTypography.body.copyWith(color: AppColors.textTertiary),
      labelStyle: AppTypography.caption,
      floatingLabelStyle: AppTypography.caption.copyWith(color: AppColors.primary),
      prefixIconColor: AppColors.textTertiary,
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(AppRadius.md),
        borderSide: BorderSide(color: AppColors.border),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(AppRadius.md),
        borderSide: BorderSide(color: AppColors.border),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(AppRadius.md),
        borderSide: BorderSide(color: AppColors.primary, width: 1.5),
      ),
      errorBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(AppRadius.md),
        borderSide: BorderSide(color: AppColors.error),
      ),
      focusedErrorBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(AppRadius.md),
        borderSide: BorderSide(color: AppColors.error, width: 1.5),
      ),
      contentPadding: const EdgeInsets.symmetric(
          horizontal: AppSpacing.md, vertical: 13),
    ),
    dividerTheme: DividerThemeData(
      color: AppColors.border,
      thickness: 1,
      space: 1,
    ),
    iconTheme: IconThemeData(color: AppColors.textSecondary, size: 22),
    snackBarTheme: SnackBarThemeData(
      behavior: SnackBarBehavior.floating,
      backgroundColor: AppColors.surfaceLight,
      contentTextStyle: AppTypography.body.copyWith(color: AppColors.textPrimary),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(AppRadius.md),
      ),
    ),
    popupMenuTheme: PopupMenuThemeData(
      color: AppColors.surface,
      surfaceTintColor: Colors.transparent,
      elevation: 10,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(AppRadius.md),
        side: BorderSide(color: AppColors.border),
      ),
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: AppColors.surface,
      surfaceTintColor: Colors.transparent,
      elevation: 24,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(AppRadius.lg),
      ),
      titleTextStyle: AppTypography.heading2,
    ),
    tooltipTheme: TooltipThemeData(
      decoration: BoxDecoration(
        color: AppColors.inkBandAlt,
        borderRadius: BorderRadius.circular(AppRadius.sm),
        border: Border.all(color: AppColors.border),
      ),
      textStyle: AppTypography.caption.copyWith(color: AppColors.textPrimary),
    ),
  );
}

/// Brand wordmark colours. "Neev" carries the logo's green, "Remote" the
/// product accent, so the name reads the same everywhere it appears.
class BrandColors {
  /// Sampled from the logo mark itself, so the wordmark and the icon agree.
  static const Color neev = Color(0xFF2E5411);
  static const Color remote = Color(0xFFF05A28);
}

/// The brand mark (the logo image itself).
///
/// The UI used to draw a coloured square containing the letter "N". That was a
/// placeholder, and because it was drawn rather than an image asset it survived
/// the rebrand — the app icon changed while every in-app mark still said "N".
class BrandMark extends StatelessWidget {
  const BrandMark({super.key, this.size = 28});

  final double size;

  @override
  Widget build(BuildContext context) {
    return Image.asset(
      'assets/brand/logo.png',
      width: size,
      height: size,
      filterQuality: FilterQuality.medium,
    );
  }
}

/// The "NeevRemote" wordmark, two-tone.
///
/// A widget rather than a copied TextSpan at each call site: the name appears in
/// the window title bar, the home header and the consent card, and those had
/// already drifted apart in size and weight once.
class BrandWordmark extends StatelessWidget {
  const BrandWordmark({super.key, this.style});

  /// Base style; only the colours are overridden per half.
  final TextStyle? style;

  @override
  Widget build(BuildContext context) {
    final base = style ?? AppTypography.title;
    return RichText(
      text: TextSpan(
        style: base,
        children: [
          TextSpan(text: 'Neev', style: TextStyle(color: BrandColors.neev)),
          TextSpan(text: 'Remote', style: TextStyle(color: BrandColors.remote)),
        ],
      ),
    );
  }
}
