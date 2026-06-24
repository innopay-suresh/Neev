#!/bin/bash
set -e

export PKG_CONFIG_PATH=/opt/homebrew/opt/libvpx/lib/pkgconfig

echo "Building RemoteAgent installers..."

# Ensure downloads directory exists
mkdir -p dist/packages

# 1. Build Windows NSIS Installer
echo "Building Windows Installer..."
cd client
    CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ ~/go/bin/wails build -platform windows/amd64 -nsis -clean
    cp build/bin/neev-remote-amd64-installer.exe ../dist/packages/NeevRemote-Windows-amd64.exe

# 2. Build Mac OS App
echo "Building Mac OS App..."
# Clear extended attributes (fixes 'resource fork, Finder information' codesign error)
xattr -rc . || true
~/go/bin/wails build -platform darwin/arm64

echo "Bundling dynamic libraries into the App..."
mkdir -p "build/bin/neev-remote.app/Contents/Frameworks"
dylibbundler -od -b -x "build/bin/neev-remote.app/Contents/MacOS/neev-remote" \
  -d "build/bin/neev-remote.app/Contents/Frameworks/" \
  -p "@executable_path/../Frameworks/" || true

# 3. Create Mac OS DMG
echo "Packaging Mac OS DMG..."
mkdir -p build_dmg
cp -R build/bin/neev-remote.app build_dmg/
ln -s /Applications build_dmg/Applications
hdiutil create -volname "NeevRemote" -srcfolder build_dmg -ov -format UDZO ../dist/packages/NeevRemote-macOS-arm64.dmg
rm -rf build_dmg

echo "Done! Installers are in the 'dist/packages' directory."
