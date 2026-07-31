// Renders the consent dialog so its layout can be checked without hardware.
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:neev_remote/core/theme/app_theme.dart';
import 'package:neev_remote/presentation/widgets/consent_dialog.dart';

void main() {
  testWidgets('consent dialog renders with the real device id', (t) async {
    await t.pumpWidget(MaterialApp(
      theme: lightTheme(),
      home: const Scaffold(
        body: Center(child: ConsentDialog(deviceId: 'ctrl-926941775')),
      ),
    ));
    await t.pumpAndSettle();

    expect(find.text('Connection Request'), findsOneWidget);
    expect(find.text('926 941 775'), findsOneWidget,
        reason: 'the id must be grouped 3-3-3, and ctrl- stripped');
    expect(find.text('Remember this decision'), findsOneWidget);
    expect(find.text('Accept'), findsOneWidget);
    expect(find.text('Decline'), findsOneWidget);
  });

  testWidgets('Decline returns a refusal, and remember is carried', (t) async {
    ConsentChoice? got;
    await t.pumpWidget(MaterialApp(
      theme: lightTheme(),
      home: Scaffold(
        body: Builder(builder: (ctx) {
          return ElevatedButton(
            onPressed: () async {
              got = await showDialog<ConsentChoice>(
                context: ctx,
                builder: (_) => const ConsentDialog(deviceId: '926941775'),
              );
            },
            child: const Text('open'),
          );
        }),
      ),
    ));
    await t.tap(find.text('open'));
    await t.pumpAndSettle();
    await t.tap(find.text('Remember this decision'));
    await t.pumpAndSettle();
    await t.tap(find.text('Decline'));
    await t.pumpAndSettle();

    expect(got, isNotNull);
    expect(got!.accepted, isFalse);
    expect(got!.remember, isTrue,
        reason: 'a remembered DECLINE must persist too, not just accept');
  });
}
