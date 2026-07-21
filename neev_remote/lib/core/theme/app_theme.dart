import 'package:flutter/material.dart';

/// Warm bento design system (v3) — cream canvas, white cards, one coral accent.
/// See DESIGN.md. Token NAMES are unchanged from v2 on purpose so every existing
/// widget keeps compiling; only the values moved.
///
/// The v2 canvas (#F8F8F9) sat ~3% off white, so cards never read as cards, and
/// the border (#ECECEC) was effectively invisible — that's what made the app look
/// flat. The warm canvas also separates us from AnyDesk/TeamViewer/Splashtop,
/// which all ship cool grey.
class AppColors {
  // Brand — orange (Command Center redesign; DESIGN.md 2026-07-21)
  static const Color primary = Color(0xFFF05A28);
  static const Color primaryHover = Color(0xFFF26E40);
  static const Color primaryDark = Color(0xFFC94418); // pressed / readable on white
  static const Color primarySoft = Color(0xFFFCE5D9); // orange tint
  static const Color accent = Color(0xFFF05A28);
  static const Color accentDark = Color(0xFFC94418);
  static const Color accentSoft = Color(0xFFFCE5D9);

  static const Color secondary = Color(0xFF243B53); // navy
  static const Color secondarySoft = Color(0xFFE4E9EF);

  // Muted device-card grounds (never bright blue). White/warm-grey models over.
  static const Color deviceNavy = Color(0xFF243B53);
  static const Color deviceForest = Color(0xFF294B3A);
  static const Color devicePlum = Color(0xFF543246);
  static const Color deviceWalnut = Color(0xFF554332);

  // Surfaces
  static const Color background = Color(0xFFF5F1E8); // warm cream canvas
  static const Color surface = Color(0xFFFFFEFB);
  static const Color surfaceLight = Color(0xFFF8F5EE);
  static const Color surfaceAlt = Color(0xFFF8F5EE);

  // Text
  static const Color textPrimary = Color(0xFF171714);
  static const Color textSecondary = Color(0xFF777266);
  static const Color textTertiary = Color(0xFF9A9385);

  // Status
  static const Color success = Color(0xFF198764); // green — online
  static const Color successSoft = Color(0xFFDDEFE7);
  static const Color warning = Color(0xFFD78A18); // amber — favourite
  static const Color error = Color(0xFFD8493F);
  static const Color infoSlate = Color(0xFF53616D);

  // Lines
  static const Color border = Color(0xFFDED6C8);
  static const Color borderStrong = Color(0xFFD0C6AC);

  // Dark band (promo / unattended callout)
  static const Color inkBand = Color(0xFF171714);
  static const Color inkBandAlt = Color(0xFF282720);
}

/// Warm, low-spread shadows tuned for the cream canvas.
class AppShadows {
  static const List<BoxShadow> card = [
    BoxShadow(color: Color(0x0A181108), blurRadius: 2, offset: Offset(0, 1)),
    BoxShadow(color: Color(0x0F181108), blurRadius: 10, offset: Offset(0, 4)),
  ];
  static const List<BoxShadow> soft = [
    BoxShadow(color: Color(0x0A181108), blurRadius: 6, offset: Offset(0, 2)),
  ];
  static const List<BoxShadow> float = [
    BoxShadow(color: Color(0x0D181108), blurRadius: 4, offset: Offset(0, 2)),
    BoxShadow(color: Color(0x24181108), blurRadius: 32, offset: Offset(0, 16)),
  ];

  // Command Center redesign (DESIGN.md 2026-07-21).
  /// Floating connection dock — layered warm shadow.
  static const List<BoxShadow> dock = [
    BoxShadow(color: Color(0x0F1C1812), blurRadius: 12, offset: Offset(0, 4)),
    BoxShadow(color: Color(0x1A1C1812), blurRadius: 50, offset: Offset(0, 20)),
  ];

  /// Hovered device card — lifts off the cream canvas.
  static const List<BoxShadow> cardHover = [
    BoxShadow(color: Color(0x141C1812), blurRadius: 20, offset: Offset(0, 8)),
    BoxShadow(color: Color(0x211C1812), blurRadius: 60, offset: Offset(0, 24)),
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
  static TextStyle get display => const TextStyle(
      fontSize: 26,
      fontWeight: FontWeight.w600,
      letterSpacing: -0.3,
      color: AppColors.textPrimary);

  static TextStyle get heading1 => const TextStyle(
      fontSize: 24,
      fontWeight: FontWeight.w600,
      letterSpacing: -0.2,
      color: AppColors.textPrimary);

  static TextStyle get heading2 => const TextStyle(
      fontSize: 18, fontWeight: FontWeight.w600, color: AppColors.textPrimary);

  static TextStyle get title => const TextStyle(
      fontSize: 15, fontWeight: FontWeight.w600, color: AppColors.textPrimary);

  static TextStyle get body => const TextStyle(
      fontSize: 14, fontWeight: FontWeight.w500, color: AppColors.textPrimary);

  static TextStyle get bodyStrong => const TextStyle(
      fontSize: 14, fontWeight: FontWeight.w600, color: AppColors.textPrimary);

  static TextStyle get caption => const TextStyle(
      fontSize: 12, fontWeight: FontWeight.w500, color: AppColors.textSecondary);

  static TextStyle get label => const TextStyle(
      fontSize: 12,
      fontWeight: FontWeight.w600,
      letterSpacing: 0.1,
      color: AppColors.textSecondary);

  // ---- Bento additions (DESIGN.md) ----

  /// Page + section titles, stat values. Space Grotesk.
  static TextStyle get pageTitle => const TextStyle(
      fontFamily: kFontDisplay,
      fontSize: 19,
      fontWeight: FontWeight.w600,
      color: AppColors.textPrimary);

  static TextStyle get sectionTitle => const TextStyle(
      fontFamily: kFontDisplay,
      fontSize: 15,
      fontWeight: FontWeight.w600,
      color: AppColors.textPrimary);

  static TextStyle get cardTitle => const TextStyle(
      fontFamily: kFontDisplay,
      fontSize: 13,
      fontWeight: FontWeight.w600,
      color: AppColors.textPrimary);

  /// Device IDs / passwords / build stamps. Tabular so digits don't jitter.
  static TextStyle get idLarge => const TextStyle(
      fontFamily: kFontMono,
      fontSize: 14,
      fontWeight: FontWeight.w600,
      letterSpacing: 0.3,
      fontFeatures: [FontFeature.tabularFigures()],
      color: AppColors.textPrimary);

  static TextStyle get mono => const TextStyle(
      fontFamily: kFontMono,
      fontSize: 12.5,
      fontWeight: FontWeight.w500,
      fontFeatures: [FontFeature.tabularFigures()],
      color: AppColors.textPrimary);

  /// Tiny uppercase field labels ("YOUR ID", "PASSWORD").
  static TextStyle get microLabel => const TextStyle(
      fontSize: 9,
      fontWeight: FontWeight.w500,
      letterSpacing: 0.6,
      color: AppColors.textTertiary);

  /// Row meta under a tile ("Reception-PC · 2 days ago").
  static TextStyle get meta => const TextStyle(
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

/// Light theme.
ThemeData lightTheme() {
  final base = ThemeData(useMaterial3: true, brightness: Brightness.light);
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
    colorScheme: const ColorScheme.light(
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
        side: const BorderSide(color: AppColors.border, width: 1),
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
        side: const BorderSide(color: AppColors.borderStrong),
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
        borderSide: const BorderSide(color: AppColors.border),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(AppRadius.md),
        borderSide: const BorderSide(color: AppColors.border),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(AppRadius.md),
        borderSide: const BorderSide(color: AppColors.primary, width: 1.5),
      ),
      errorBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(AppRadius.md),
        borderSide: const BorderSide(color: AppColors.error),
      ),
      focusedErrorBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(AppRadius.md),
        borderSide: const BorderSide(color: AppColors.error, width: 1.5),
      ),
      contentPadding: const EdgeInsets.symmetric(
          horizontal: AppSpacing.md, vertical: 13),
    ),
    dividerTheme: const DividerThemeData(
      color: AppColors.border,
      thickness: 1,
      space: 1,
    ),
    iconTheme: const IconThemeData(color: AppColors.textSecondary, size: 22),
    snackBarTheme: SnackBarThemeData(
      behavior: SnackBarBehavior.floating,
      backgroundColor: AppColors.textPrimary,
      contentTextStyle: AppTypography.body.copyWith(color: Colors.white),
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
        side: const BorderSide(color: AppColors.border),
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
        color: AppColors.textPrimary,
        borderRadius: BorderRadius.circular(AppRadius.sm),
      ),
      textStyle: AppTypography.caption.copyWith(color: Colors.white),
    ),
  );
}
