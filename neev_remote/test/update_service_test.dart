// Update checks must not cry wolf, and must not falsely reassure.
import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/data/services/update_service.dart';

void main() {
  test('release number is parsed from the build stamp', () {
    expect(UpdateService.releaseNumber('build 2026-07-31 · r123-honest-progress'),
        123);
    expect(UpdateService.releaseNumber('build 2026-07-30 · r99-something'), 99);
  });

  test('an unparsable stamp yields null, never a number', () {
    // Null is what stops a bad stamp from being treated as "you are behind".
    expect(UpdateService.releaseNumber('nightly build'), isNull);
    expect(UpdateService.releaseNumber(''), isNull);
  });

  test('release numbers compare numerically, not as text', () {
    final a = UpdateService.releaseNumber('r99-x')!;
    final b = UpdateService.releaseNumber('r123-y')!;
    expect(b > a, isTrue,
        reason: 'string comparison would rank r99 above r123 and tell users on '
            'the newest build to downgrade');
  });
}
