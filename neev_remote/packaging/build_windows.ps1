# Builds Neev Remote for Windows (x64). Both outputs are SELF-CONTAINED — they
# bundle the Flutter engine, plugin DLLs and the Visual C++ runtime, so end
# users just install/run with nothing pre-installed:
#   dist\NeevRemote-windows-x64-portable.zip  (portable, unzip & run neev_remote.exe)
#   dist\NeevRemote-Setup-x64.exe             (installer, if Inno Setup is installed)
#
# These BUILD-TIME tools are needed only on the machine that builds (not by end
# users): Flutter, Visual Studio "Desktop development with C++", and (for the
# installer) Inno Setup.
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

$Out = "dist"
New-Item -ItemType Directory -Force -Path $Out | Out-Null

Write-Host "==> flutter build windows --release"
flutter build windows --release

$ReleaseDir = "build\windows\x64\runner\Release"
if (-not (Test-Path $ReleaseDir)) { throw "Release dir not found: $ReleaseDir" }

# Bundle the Visual C++ runtime DLLs next to the exe so the app runs on a clean
# PC with nothing pre-installed (end users don't need the VC++ redistributable).
Write-Host "==> bundling Visual C++ runtime"
$vswhere = "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe"
if (Test-Path $vswhere) {
  $vsPath = & $vswhere -latest -property installationPath
  $crt = Get-ChildItem (Join-Path $vsPath "VC\Redist\MSVC") -Recurse -Directory `
           -Filter "Microsoft.VC*.CRT" -ErrorAction SilentlyContinue |
         Where-Object { $_.FullName -match "\\x64\\" } |
         Sort-Object FullName -Descending | Select-Object -First 1
  if ($crt) {
    foreach ($dll in "msvcp140.dll","vcruntime140.dll","vcruntime140_1.dll") {
      $src = Join-Path $crt.FullName $dll
      if (Test-Path $src) { Copy-Item $src $ReleaseDir -Force }
    }
    Write-Host "    bundled CRT from $($crt.FullName)"
  } else {
    Write-Warning "    VC++ CRT folder not found; app may need the VC++ redistributable"
  }
} else {
  Write-Warning "    vswhere not found; skipping VC++ runtime bundling"
}

Write-Host "==> portable zip"
$Zip = Join-Path $Out "NeevRemote-windows-x64-portable.zip"
if (Test-Path $Zip) { Remove-Item $Zip }
Compress-Archive -Path "$ReleaseDir\*" -DestinationPath $Zip

Write-Host "==> installer (Inno Setup)"
$iscc = Get-Command iscc.exe -ErrorAction SilentlyContinue
if ($iscc) {
  & $iscc.Source "packaging\windows\installer.iss"
  Write-Host "Installer written to $Out"
} else {
  Write-Warning "iscc.exe not found - skipping installer. Install Inno Setup from https://jrsoftware.org/isdl.php"
}

Write-Host "==> done"
Get-ChildItem $Out
