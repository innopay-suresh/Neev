; Inno Setup script for Neev Remote (Windows installer).
; Build with: iscc.exe packaging\windows\installer.iss
; (build_windows.ps1 runs this automatically when iscc.exe is on PATH)

#define AppName "Neev Remote"
#define AppVersion "1.0.0"
#define AppPublisher "Neev"
#define AppExe "neev_remote.exe"

[Setup]
AppId={{8F1B6C3A-7E2D-4B5A-9C11-NEEVREMOTE001}}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
OutputDir=..\..\dist
OutputBaseFilename=NeevRemote-Setup-x64
Compression=lzma2
SolidCompression=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
WizardStyle=modern
; Admin is needed to (optionally) install the UAC helper as a LOCAL SYSTEM
; service and to install into Program Files. The installer elevates once, like
; AnyDesk/TeamViewer.
PrivilegesRequired=admin

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Additional icons:"
; Opt-in (off by default): installs the SYSTEM helper service so a remote
; controller can see and approve Windows UAC / admin prompts.
Name: "uacservice"; Description: "Enable remote control of Windows admin (UAC) prompts — installs a background helper service"; GroupDescription: "Advanced:"; Flags: unchecked

[Files]
; Packages the entire release folder produced by `flutter build windows`.
Source: "..\..\build\windows\x64\runner\Release\*"; DestDir: "{app}"; Flags: recursesubdirs createallsubdirs

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\{#AppExe}"
Name: "{group}\Uninstall {#AppName}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#AppName}"; Filename: "{app}\{#AppExe}"; Tasks: desktopicon

[Run]
; Install + start the helper service only if the user opted in.
Filename: "{app}\neev_helper.exe"; Parameters: "install"; Tasks: uacservice; Flags: runhidden waituntilterminated; StatusMsg: "Installing UAC helper service..."
Filename: "{app}\{#AppExe}"; Description: "Launch {#AppName}"; Flags: nowait postinstall skipifsilent

[UninstallRun]
; Always remove the service on uninstall (no-op if it was never installed).
Filename: "{app}\neev_helper.exe"; Parameters: "uninstall"; Flags: runhidden; RunOnceId: "uninstallneevhelper"
