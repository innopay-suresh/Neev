#!/usr/bin/env python3
"""Assert a built package actually contains what the release claims.

Written after r130 was assembled, uploaded and verified GREEN while its macOS
package contained no NeevVoice.app at all — the helper had failed to compile on
CI, the packaging script only warned, and the build-stamp check I was running
looked at the Flutter app and never at the helper. A release check that only
inspects the part you remember to inspect is not a release check.

Usage:
    verify_release.py <release-tag> <macos-zip> <windows-portable-zip>

Exits non-zero and prints every failure, so one run tells you everything that
is wrong rather than one thing at a time.
"""
import sys
import zipfile

failures = []
checks = 0


def check(label, ok):
    global checks
    checks += 1
    print(("  PASS  " if ok else "  FAIL  ") + label)
    if not ok:
        failures.append(label)


def blob_contains(zf, name_filter, needle, size_cap=250 * 1024 * 1024):
    """True if any member matching name_filter contains needle."""
    for info in zf.infolist():
        if info.is_dir() or info.file_size > size_cap:
            continue
        if not name_filter(info.filename):
            continue
        try:
            if needle in zf.read(info):
                return True
        except Exception:
            pass
    return False


def verify_macos(path, tag):
    print(f"macOS package: {path}")
    with zipfile.ZipFile(path) as outer:
        inner_name = next(
            (n for n in outer.namelist() if n.endswith("NeevRemote-macos.zip")), None)
        check("macOS app zip present in artifact", inner_name is not None)
        if not inner_name:
            return
        import io
        with zipfile.ZipFile(io.BytesIO(outer.read(inner_name))) as z:
            names = z.namelist()
            check(f"build stamp {tag} in App.framework",
                  blob_contains(z, lambda n: n.endswith("Frameworks/App.framework/Versions/A/App"),
                                tag.encode()))
            # The check that r130 was missing.
            helper = [n for n in names if n.endswith("NeevVoice.app/Contents/MacOS/NeevVoice")]
            check("NeevVoice.app binary present (host session controls)", bool(helper))
            check("NeevVoice Info.plist present",
                  any(n.endswith("NeevVoice.app/Contents/Info.plist") for n in names))
            check("microphone usage description in helper plist",
                  blob_contains(z, lambda n: n.endswith("NeevVoice.app/Contents/Info.plist"),
                                b"NSMicrophoneUsageDescription"))
            if helper:
                # Only strings that RELIABLY appear as literals. Swift stores
                # short strings inline rather than in the binary's literal
                # section, so "End session" (11 bytes) is absent even from a
                # build that definitely has that menu item — checking for it
                # failed on a known-good binary, which is worse than not
                # checking at all: it teaches you to wave failures through.
                for label, needle in [
                    ("helper: system-sound menu", b"Share this Mac"),
                    ("helper: ScreenCaptureKit linked", b"ScreenCaptureKit"),
                    ("helper: recording menu", b"Record this session"),
                    ("helper: session-active title", b"Remote session active"),
                    ("helper: permission guidance", b"Screen & System Audio Recording"),
                ]:
                    check(label, blob_contains(z, lambda n: n == helper[0], needle))
                # Universal binary, so Intel Macs are covered too. A silently
                # arm64-only helper would simply not launch on an Intel host.
                info = next(i for i in z.infolist() if i.filename == helper[0])
                check("helper: binary is a plausible universal build (>200 KB)",
                      info.file_size > 200 * 1024)
            check("neev-agent (daemon) present",
                  any(n.endswith("Resources/daemon/neev-agent") for n in names))


def verify_windows(path, tag):
    print(f"Windows package: {path}")
    with zipfile.ZipFile(path) as outer:
        inner_name = next(
            (n for n in outer.namelist() if n.endswith("windows-x64-portable.zip")), None)
        check("Windows portable zip present in artifact", inner_name is not None)
        if not inner_name:
            return
        import io
        with zipfile.ZipFile(io.BytesIO(outer.read(inner_name))) as z:
            check(f"build stamp {tag} in app.so",
                  blob_contains(z, lambda n: n.endswith("data/app.so"), tag.encode()))
            host = lambda n: n.endswith("neev-host.exe")
            for label, needle in [
                ("host: Record button", b"Record"),
                ("host: Sound button", b"Sound off"),
                ("host: Mic button", b"Mic off"),
                ("host: Disconnect button", b"Disconnect"),
                ("host: WebM muxer", b"V_VP8"),
                ("host: recordings folder", b"Neev Recordings"),
            ]:
                check(label, blob_contains(z, host, needle))


def main():
    if len(sys.argv) != 4:
        print(__doc__)
        return 2
    tag, mac, win = sys.argv[1], sys.argv[2], sys.argv[3]
    verify_macos(mac, tag)
    verify_windows(win, tag)
    print(f"\n{checks - len(failures)}/{checks} checks passed")
    if failures:
        print("REFUSING TO PUBLISH — failed:")
        for f in failures:
            print("  - " + f)
        return 1
    print("Package contents match the release. Safe to publish.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
