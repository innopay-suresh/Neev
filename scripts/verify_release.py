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

# Strings that prove a viewer-facing feature actually shipped. Long enough to
# survive Swift/Dart small-string inlining and specific enough that a rename
# fails the check loudly rather than passing by accident.
VIEWER_STRINGS = [
    ("Record button present", "The host captures it and sends"),
    ("recording sent-back promise", "The file is sent here when"),
    ("host-refusal reason (interactive)", "not accepting interactive connections"),
    ("host-refusal reason (declined)", "declined the connection"),
]

failures = []
checks = 0


def check(label, ok):
    global checks
    checks += 1
    print(("  PASS  " if ok else "  FAIL  ") + label)
    if not ok:
        failures.append(label)


def _pkg_has_scripts(blob):
    """True if a .pkg carries an installer Scripts payload.

    A .pkg is a xar archive. The scripts live in a COMPRESSED cpio entry, so the
    filename "postinstall" never appears in the raw bytes or in the table of
    contents — checking for it reported a correctly built package as broken.
    What pkgbuild --scripts does add, and what is therefore checkable, is a file
    entry named "Scripts" in the TOC.
    """
    import struct
    import zlib
    try:
        if blob[:4] != b"xar!":
            return False
        header_size = struct.unpack(">H", blob[4:6])[0]
        toc_len = struct.unpack(">Q", blob[8:16])[0]
        toc = zlib.decompress(blob[header_size:header_size + toc_len])
        return b"<name>Scripts</name>" in toc
    except Exception:
        return False


def _pkg_package_info(blob):
    """The .pkg's PackageInfo XML, decompressed, or None.

    Every earlier version of this check regex-searched the raw .pkg bytes for
    PackageInfo elements. That can never match: PackageInfo is a zlib-compressed
    heap entry inside the xar, so the check silently passed on every package it
    was ever run against — including one whose PackageInfo held the exact
    element it was meant to reject. Locating the entry through the TOC is the
    only way to read it.
    """
    import re
    import struct
    import xml.etree.ElementTree as ET
    import zlib
    try:
        if blob[:4] != b"xar!":
            return None
        header_size = struct.unpack(">H", blob[4:6])[0]
        toc_len = struct.unpack(">Q", blob[8:16])[0]
        heap = header_size + toc_len
        toc = ET.fromstring(
            zlib.decompress(blob[header_size:header_size + toc_len]))
        for f in toc.iter("file"):
            name = f.findtext("name")
            data = f.find("data")
            if name != "PackageInfo" or data is None:
                continue
            off = int(data.findtext("offset"))
            length = int(data.findtext("length"))
            raw = blob[heap + off:heap + off + length]
            style = data.findtext("encoding/{*}style") or \
                (data.find("encoding").get("style") if data.find("encoding") is not None else "")
            if "gzip" in (style or ""):
                raw = zlib.decompress(raw)
            return raw
        return None
    except Exception:
        return None


def _pkg_installs_at_fixed_location(blob):
    """True if the .pkg cannot be relocated away from --install-location.

    pkgbuild marks app bundles relocatable by default, emitting
    <relocate><bundle id="…"/></relocate>. The installer then asks
    LaunchServices where that bundle id already lives and writes the payload
    THERE, ignoring --install-location entirely. On a Mac that had ever held an
    older copy — one in the Trash was enough — /Applications stayed empty and
    the postinstall failed on a package whose payload was perfectly intact,
    while a never-seen-it Mac installed fine. Only BundleIsRelocatable=false in
    a component plist removes the element.
    """
    info = _pkg_package_info(blob)
    if info is None:
        return False
    import re
    # <relocate/> (self-closing, i.e. empty) is the shape we want.
    return re.search(rb"<relocate>\s*<bundle", info) is None


def _encodings(needle):
    """Every byte form a string may take in a shipped binary.

    Dart AOT stores a string as Latin-1 when every character fits in a byte and
    as UTF-16LE the moment one does not — so an em dash silently moves a whole
    tooltip into a different encoding. Probing UTF-8 only reported a shipped
    feature as MISSING, which is the worst kind of check: it fails on good
    builds and teaches you to ignore it.
    """
    if isinstance(needle, bytes):
        return [needle]
    forms = [needle.encode("utf-8"), needle.encode("utf-16-le")]
    try:
        forms.append(needle.encode("latin-1"))
    except UnicodeEncodeError:
        pass
    # De-duplicate while keeping order (ASCII utf-8 and latin-1 are identical).
    seen, out = set(), []
    for f in forms:
        if f not in seen:
            seen.add(f)
            out.append(f)
    return out


def blob_contains(zf, name_filter, needle, size_cap=250 * 1024 * 1024):
    """True if any member matching name_filter contains needle, in any of the
    encodings a compiled binary might have stored it in."""
    forms = _encodings(needle)
    for info in zf.infolist():
        if info.is_dir() or info.file_size > size_cap:
            continue
        if not name_filter(info.filename):
            continue
        try:
            blob = zf.read(info)
        except Exception:
            continue
        if any(f in blob for f in forms):
            return True
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
            appfw = lambda n: n.endswith("Frameworks/App.framework/Versions/A/App")
            check(f"build stamp {tag} in App.framework", blob_contains(z, appfw, tag.encode()))
            # Viewer-facing features live in the Dart snapshot. Checking the
            # host binary alone is how a missing macOS helper passed once.
            for label, needle in VIEWER_STRINGS:
                check("viewer: " + label, blob_contains(z, appfw, needle))
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
            # Reported, not enforced: an ad-hoc build is legitimate (forks
            # cannot read the signing secret) but means every macOS update
            # invalidates the machine's Screen Recording grant. Worth seeing at
            # publish time rather than discovering when a host stops capturing.
            agent = [n for n in names if n.endswith("Resources/daemon/neev-agent")]
            if agent:
                blob = z.read(agent[0])
                # Look for the signing certificate's common name, which IS
                # embedded verbatim. The designated requirement is stored
                # binary-encoded, so searching for the text "certificate leaf"
                # reported a correctly signed build as ad-hoc — the third time a
                # check in this file has failed by assuming an encoding.
                stable = b"Neev Remote Code Signing" in blob
                print("  " + ("INFO  " if stable else "WARN  ") +
                      ("agent has a STABLE signing identity — TCC grants survive updates"
                       if stable else
                       "agent is AD-HOC signed — every update needs Screen Recording re-granted"))
                # A dylib path under /opt/homebrew or /usr/local exists only on
                # a machine with Homebrew. Shipped that way, dyld cannot resolve
                # it on a user's Mac and kills the daemon before main() —
                # launchd then crash-loops it forever with
                # `last exit reason = OS_REASON_DYLD`. An installed, correctly
                # signed daemon that can never start, and it looked
                # machine-specific for days because a dev Mac runs it fine.
                # The build now bundles dependencies beside the binary and
                # repoints them at @loader_path.
                portable = (b"/opt/homebrew" not in blob
                            and b"/usr/local/opt" not in blob)
                check("agent has no build-machine dylib paths", portable)
            check("install-daemon.sh present (postinstall calls it)",
                  any(n.endswith("Resources/daemon/install-daemon.sh") for n in names))

    # The .pkg must carry a postinstall, or installing it drops the app and
    # nothing else — no host daemon, no session controls, and an app that looks
    # installed while silently lacking half its features. This is the same class
    # of silent gap that shipped a macOS package with no NeevVoice.app.
    with zipfile.ZipFile(path) as outer:
        pkg = next((n for n in outer.namelist() if n.endswith("NeevRemote-macos.pkg")), None)
        check("macOS .pkg present in artifact", pkg is not None)
        if pkg:
            pkg_blob = outer.read(pkg)
            check("pkg carries a postinstall script", _pkg_has_scripts(pkg_blob))
            # A relocatable bundle lets the installer follow LaunchServices to
            # wherever an old copy was last seen and write there instead of
            # /Applications, so the app never lands where the postinstall looks.
            check("pkg is non-relocatable (installs at /Applications)",
                  _pkg_installs_at_fixed_location(pkg_blob))

    # The .dmg is the fallback install route, and the one that has never failed
    # on a user's machine: drag-to-Applications runs no installer logic, so
    # there is nothing to relocate or silently no-op. It has to actually be in
    # the artifact for that to be true.
    #
    # Size is the only property checkable here — a UDZO image is compressed, so
    # the app bundle's contents are not greppable. The daemon payload inside the
    # app is covered by the .zip checks above, and the .dmg is built from the
    # same bundle in the same run.
    with zipfile.ZipFile(path) as outer:
        dmg = next((n for n in outer.namelist()
                    if n.endswith("NeevRemote-macos.dmg")), None)
        check("macOS .dmg present in artifact", dmg is not None)
        if dmg:
            check("dmg is a full image (>20 MB)",
                  outer.getinfo(dmg).file_size > 20 * 1024 * 1024)


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
            appso = lambda n: n.endswith("data/app.so")
            check(f"build stamp {tag} in app.so", blob_contains(z, appso, tag.encode()))
            for label, needle in VIEWER_STRINGS:
                check("viewer: " + label, blob_contains(z, appso, needle))
            host = lambda n: n.endswith("neev-host.exe")
            for label, needle in [
                ("host: Record button", b"Record"),
                ("host: Sound button", b"Sound off"),
                ("host: Voice button", b"Voice off"),
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
