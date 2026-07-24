import 'package:flutter/material.dart';

/// Obsidian design system (v4) — warm-charcoal canvas, elevated dark cards, one
/// ember accent that glows only where it matters; thumbnails read as lit windows.
/// See DESIGN.md (2026-07-24, direction "Obsidian" approved by user). Token NAMES
/// are unchanged from v3 on purpose so every existing widget keeps compiling; only
/// the values moved (same retune method v2→v3 used). This is a dark-first theme:
/// the app forces it regardless of OS brightness (see obsidianTheme()).
class AppColors {
  // Brand — ember. Lighter on the dark ground so it reads without shouting.
  static const Color primary = Color(0xFFFF6A32);
  static const Color primaryHover = Color(0xFFFF8352);
  static const Color primaryDark = Color(0xFFFF7A45); // text-on-tint + pressed (legible on dark)
  static const Color primarySoft = Color(0x26FF6A32); // ember tint over the charcoal
  static const Color accent = Color(0xFFFF6A32);
  static const Color accentDark = Color(0xFFFF8352); // readable ember on dark
  static const Color accentSoft = Color(0x26FF6A32);

  static const Color secondary = Color(0xFF2A2620); // elevated warm panel (ID card)
  static const Color secondarySoft = Color(0xFF232019);

  // Device-card grounds — deep jewel tones, read as lit cards on the charcoal.
  static const Color deviceNavy = Color(0xFF22344A);
  static const Color deviceForest = Color(0xFF23412F);
  static const Color devicePlum = Color(0xFF48293C);
  static const Color deviceWalnut = Color(0xFF473726);

  // Surfaces — warm charcoal (never pure black).
  static const Color background = Color(0xFF131210); // canvas
  static const Color surface = Color(0xFF1C1A16); // cards, sidebar, bars
  static const Color surfaceLight = Color(0xFF232019); // elevated
  static const Color surfaceAlt = Color(0xFF201D18); // input fills

  // Text — warm off-white (never pure white).
  static const Color textPrimary = Color(0xFFF4EFE4);
  static const Color textSecondary = Color(0xFFAAA093);
  static const Color textTertiary = Color(0xFF6F695B);

  // Status — brighter hues so they carry on the dark ground.
  static const Color success = Color(0xFF3ECF8E); // green — online
  static const Color successSoft = Color(0x263ECF8E);
  static const Color warning = Color(0xFFFFB655); // amber — favourite
  static const Color error = Color(0xFFFF5C50);
  static const Color infoSlate = Color(0xFF7C8AA0);

  // Lines
  static const Color border = Color(0xFF302C24);
  static const Color borderStrong = Color(0xFF3C3627);

  // Elevated band (promo / unattended callout) — warm, stands off the canvas.
  static const Color inkBand = Color(0xFF232019);
  static const Color inkBandAlt = Color(0xFF2A2620);
}

/// Deep shadows for the Obsidian charcoal — on a dark ground depth comes from
/// darker-than-canvas shadow plus a hairline top edge, not soft warm spread.
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

  /// Floating connection dock — deep lift off the charcoal.
  static const List<BoxShadow> dock = [
    BoxShadow(color: Color(0x59000000), blurRadius: 14, offset: Offset(0, 5)),
    BoxShadow(color: Color(0x73000000), blurRadius: 54, offset: Offset(0, 22)),
  ];

  /// Hovered device card — lifts + deepens.
  static const List<BoxShadow> cardHover = [
    BoxShadow(color: Color(0x66000000), blurRadius: 24, offset: Offset(0, 10)),
    BoxShadow(color: Color(0x80000000), blurRadius: 64, offset: Offset(0, 28)),
  ];

  /// Ember glow — the one signature Obsidian flourish. Use ONLY on the brand
  /// mark and the primary Connect action, never as ambient decoration.
  static const List<BoxShadow> emberGlow = [
    BoxShadow(color: Color(0x59FF6A32), blurRadius: 24, offset: Offset(0, 6)),
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

/// Obsidian theme — dark-first (forced regardless of OS brightness).
ThemeData obsidianTheme() {
  final base = ThemeData(useMaterial3: true, brightness: Brightness.dark);
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
    colorScheme: const ColorScheme.dark(
      primary: AppColors.primary,
      secondary: AppColors.accent,
      surface: AppColors.surface,
      error: AppColors.error,
      onPrimary: Colors.white,
      onSecondary: Colors.white,
      onSurface: AppColors.textPrimary,
      onError: Colors.black,
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
      backgroundColor: AppColors.surfaceLight,
      contentTextStyle: AppTypography.body.copyWith(color: AppColors.textPrimary),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(AppRadius.md),
        side: const BorderSide(color: AppColors.border),
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
        color: AppColors.inkBandAlt,
        borderRadius: BorderRadius.circular(AppRadius.sm),
        border: Border.all(color: AppColors.border),
      ),
      textStyle: AppTypography.caption.copyWith(color: AppColors.textPrimary),
    ),
  );
}
