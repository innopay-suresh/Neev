import 'package:flutter/material.dart';

import '../../core/theme/app_theme.dart';

/// Skeleton placeholders for content that is loading.
///
/// A spinner says "something is happening"; a skeleton says WHAT is coming and
/// how much of it, so the layout does not jump when the data lands. It also
/// stops the empty state and the loading state from looking identical, which is
/// the difference between "still fetching" and "you have no devices".
///
/// The shimmer is deliberately slow (1.4s) and low-contrast: a fast, bright
/// pulse reads as an error state and is hostile to anyone sensitive to motion.
class Skeleton extends StatefulWidget {
  const Skeleton({
    super.key,
    required this.width,
    required this.height,
    this.radius = AppRadii.sm,
  });

  /// A single line of text. [width] is usually fractional via [SkeletonLine].
  final double width;
  final double height;
  final double radius;

  @override
  State<Skeleton> createState() => _SkeletonState();
}

class _SkeletonState extends State<Skeleton>
    with SingleTickerProviderStateMixin {
  late final AnimationController _c = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1400),
  )..repeat(reverse: true);

  @override
  void dispose() {
    // A repeating controller left running holds a ticker for the life of the
    // route and keeps the app awake redrawing something nobody can see.
    _c.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _c,
      builder: (context, _) {
        return Container(
          width: widget.width,
          height: widget.height,
          decoration: BoxDecoration(
            color: Color.lerp(
              AppColors.border,
              AppColors.surfaceAlt,
              _c.value,
            ),
            borderRadius: BorderRadius.circular(widget.radius),
          ),
        );
      },
    );
  }
}

/// One placeholder line, sized as a fraction of the available width so a
/// skeleton row mirrors the ragged look of real text instead of a solid block.
class SkeletonLine extends StatelessWidget {
  const SkeletonLine({super.key, this.widthFactor = 1.0, this.height = 12});

  final double widthFactor;
  final double height;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, c) => Skeleton(
        width: c.maxWidth * widthFactor,
        height: height,
      ),
    );
  }
}

/// A list of placeholder rows, shaped like the rows that will replace them.
class SkeletonList extends StatelessWidget {
  const SkeletonList({super.key, this.rows = 5, this.rowHeight = 56});

  final int rows;
  final double rowHeight;

  @override
  Widget build(BuildContext context) {
    return ListView.separated(
      padding: const EdgeInsets.all(AppSpacing.md),
      itemCount: rows,
      separatorBuilder: (_, _) => const SizedBox(height: AppSpacing.sm),
      itemBuilder: (context, i) => SizedBox(
        height: rowHeight,
        child: Row(
          children: [
            const Skeleton(width: 36, height: 36, radius: AppRadii.md),
            const SizedBox(width: AppSpacing.md),
            Expanded(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  // Vary the widths so the block reads as text, not as bars.
                  SkeletonLine(widthFactor: i.isEven ? 0.42 : 0.34),
                  const SizedBox(height: AppSpacing.xs),
                  SkeletonLine(widthFactor: i.isEven ? 0.24 : 0.30, height: 10),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
