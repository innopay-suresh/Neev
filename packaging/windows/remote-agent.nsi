!include "MUI2.nsh"

!ifndef APP_NAME
  !define APP_NAME "Remote Agent"
!endif
!ifndef INSTALL_DIR
  !define INSTALL_DIR "$PROGRAMFILES\RemoteAgent"
!endif
!ifndef OUTPUT_FILE
  !define OUTPUT_FILE "RemoteAgentInstaller.exe"
!endif
!ifndef AGENT_BINARY
  !define AGENT_BINARY "..\..\agent\remote-agent.exe"
!endif
!ifndef CLIENT_BINARY
  !define CLIENT_BINARY "..\..\client\build\bin\remote-agent.exe"
!endif
!ifndef RELAY_URL
  !define RELAY_URL "ws://localhost:8080/ws"
!endif
!ifndef ENROLLMENT_CODE
  !define ENROLLMENT_CODE ""
!endif
!ifndef ORG_ID
  !define ORG_ID ""
!endif
!ifndef DEVICE_GROUP
  !define DEVICE_GROUP ""
!endif
!ifndef TURN_URL
  !define TURN_URL ""
!endif
!ifndef TURN_USER
  !define TURN_USER "agent"
!endif
!ifndef TURN_PASS
  !define TURN_PASS "changeme"
!endif
!ifndef AGENT_CERT_FILE
  !define AGENT_CERT_FILE ""
!endif
!ifndef AGENT_KEY_FILE
  !define AGENT_KEY_FILE ""
!endif
!ifndef AGENT_CA_FILE
  !define AGENT_CA_FILE ""
!endif
!ifndef STATUS_URL
  !define STATUS_URL "http://127.0.0.1:7891/"
!endif

Name "${APP_NAME}"
OutFile "${OUTPUT_FILE}"
InstallDir "${INSTALL_DIR}"
RequestExecutionLevel admin
SetCompressor /SOLID lzma

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "explorer.exe"
!define MUI_FINISHPAGE_RUN_PARAMETERS "${STATUS_URL}"
!define MUI_FINISHPAGE_RUN_TEXT "Open RemoteAgent Status Page"
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH
!insertmacro MUI_LANGUAGE "English"

; Write config to a location accessible by the user-level agent
Function WriteBootstrapConfig
  CreateDirectory "$PROGRAMDATA\RemoteAgent"
  FileOpen $0 "$PROGRAMDATA\RemoteAgent\agent.env" w
  FileWrite $0 "RELAY_URL=${RELAY_URL}$\r$\n"
  FileWrite $0 "ENROLLMENT_CODE=${ENROLLMENT_CODE}$\r$\n"
  FileWrite $0 "ORG_ID=${ORG_ID}$\r$\n"
  FileWrite $0 "DEVICE_GROUP=${DEVICE_GROUP}$\r$\n"
  FileWrite $0 "TURN_URL=${TURN_URL}$\r$\n"
  FileWrite $0 "TURN_USER=${TURN_USER}$\r$\n"
  FileWrite $0 "TURN_PASS=${TURN_PASS}$\r$\n"
  FileWrite $0 "AGENT_CERT_FILE=${AGENT_CERT_FILE}$\r$\n"
  FileWrite $0 "AGENT_KEY_FILE=${AGENT_KEY_FILE}$\r$\n"
  FileWrite $0 "AGENT_CA_FILE=${AGENT_CA_FILE}$\r$\n"
  FileWrite $0 "NO_BROWSER=1$\r$\n"
  FileClose $0
FunctionEnd

; Register the agent as a user-level autostart entry.
; Runs as the currently logged-in user (not as LocalSystem service),
; so it has full access to the interactive desktop for screen capture.
Function InstallUserAutostart
  WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "RemoteAgent" '"$INSTDIR\remote-agent.exe"'
  CreateDirectory "$APPDATA\Microsoft\Windows\Start Menu\Programs\Startup"
  CreateShortCut "$APPDATA\Microsoft\Windows\Start Menu\Programs\Startup\RemoteAgent.lnk" "$INSTDIR\remote-agent.exe" "" "$INSTDIR\remote-agent.exe" 0
FunctionEnd

Function un.RemoveUserAutostart
  Delete "$APPDATA\Microsoft\Windows\Start Menu\Programs\Startup\RemoteAgent.lnk"
  RMDir "$APPDATA\Microsoft\Windows\Start Menu\Programs\Startup"
  DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Run\RemoteAgent"
FunctionEnd

Function CreateShortcuts
  CreateDirectory "$SMPROGRAMS\RemoteAgent"
  CreateShortCut "$SMPROGRAMS\RemoteAgent\RemoteAgent.lnk" "$INSTDIR\NeevRemote.exe"
  CreateShortCut "$DESKTOP\RemoteAgent.lnk" "$INSTDIR\NeevRemote.exe"
  CreateShortCut "$SMPROGRAMS\RemoteAgent\Uninstall RemoteAgent.lnk" "$INSTDIR\uninstall.exe"
FunctionEnd

Function WriteUninstallRegistry
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\RemoteAgent" "DisplayName" "${APP_NAME}"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\RemoteAgent" "UninstallString" "$INSTDIR\uninstall.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\RemoteAgent" "DisplayVersion" "1.0.0"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\RemoteAgent" "Publisher" "RemoteAgent"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\RemoteAgent" "InstallLocation" "$INSTDIR"
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\RemoteAgent" "NoModify" 1
  WriteRegDWORD HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\RemoteAgent" "NoRepair" 1
FunctionEnd

Section "Install"
  SetShellVarContext all
  SetOutPath "$INSTDIR"
  File /oname=remote-agent.exe "${AGENT_BINARY}"
  File /oname=NeevRemote.exe "${CLIENT_BINARY}"
  Call WriteBootstrapConfig
  WriteUninstaller "$INSTDIR\uninstall.exe"
  Call InstallUserAutostart
  Call CreateShortcuts
  Call WriteUninstallRegistry
SectionEnd

Section "Uninstall"
  SetShellVarContext all
  nsExec::ExecToLog 'taskkill /F /IM remote-agent.exe'
  Call un.RemoveUserAutostart
  Delete "$INSTDIR\remote-agent.exe"
  Delete "$INSTDIR\NeevRemote.exe"
  Delete "$INSTDIR\uninstall.exe"
  Delete "$PROGRAMDATA\RemoteAgent\agent.env"
  Delete "$SMPROGRAMS\RemoteAgent\RemoteAgent.lnk"
  Delete "$SMPROGRAMS\RemoteAgent\Uninstall RemoteAgent.lnk"
  Delete "$DESKTOP\RemoteAgent.lnk"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\RemoteAgent"
  RMDir "$PROGRAMDATA\RemoteAgent"
  RMDir "$INSTDIR"
  RMDir "$SMPROGRAMS\RemoteAgent"
SectionEnd