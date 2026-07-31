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
    expect(find.text('Allow'), findsOneWidget);
    expect(find.text('Decline'), findsOneWidget);
    // The access level is chosen HERE, by the host, at the moment of granting.
    expect(find.text('Full control'), findsOneWidget);
    expect(find.text('View only'), findsOneWidget);
    expect(find.text('Full Control Access'), findsOneWidget);
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

  testWidgets('the host can grant view-only from the prompt', (t) async {
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

    // Defaults to full control, then the host downgrades the grant.
    expect(find.text('Full Control Access'), findsOneWidget);
    await t.tap(find.text('View only'));
    await t.pumpAndSettle();
    expect(find.text('View Only Access'), findsOneWidget,
        reason: 'the panel must state what is actually being granted');

    await t.tap(find.text('Allow'));
    await t.pumpAndSettle();
    expect(got!.accepted, isTrue);
    expect(got!.control, isFalse,
        reason: 'Allow after choosing View only must NOT grant control');
  });
}
