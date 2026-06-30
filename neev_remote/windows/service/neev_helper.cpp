// neev_helper.cpp — Neev Remote privileged helper (UAC support, Phase 1).
//
// WHY THIS EXISTS
//   A normal user-session app (the Flutter host) cannot see or click the
//   Windows UAC consent prompt or other elevated windows: UAC runs on the
//   isolated "secure desktop" and Windows UIPI forbids a lower-integrity
//   process from injecting input into a higher-integrity window. The only
//   robust fix (what AnyDesk/TeamViewer ship) is a LOCAL SYSTEM service plus a
//   SYSTEM-level agent running inside the interactive session. This binary is
//   that helper.
//
// PHASE 1 SCOPE (this file) — prove the privileged foundation on real machines:
//   * install / uninstall / run as a LOCAL SYSTEM auto-start service
//   * the service launches an AGENT *as SYSTEM* in the active interactive
//     session (so the agent can later open the secure desktop + inject into
//     elevated windows)
//   * the agent logs the current input-desktop name; triggering a UAC prompt
//     makes the log show the secure desktop ("Winlogon") being detected.
//   Later phases add: secure-desktop capture, input injection into
//   consent.exe / elevated windows, and a named-pipe IPC to the Flutter app.
//
// BUILD (standalone, no Flutter deps):
//   cl /EHsc /O2 /DUNICODE /D_UNICODE neev_helper.cpp ^
//      /link advapi32.lib user32.lib wtsapi32.lib userenv.lib
//
// USAGE:
//   neev_helper.exe install     (elevated) — create + start the service
//   neev_helper.exe uninstall   (elevated) — stop + delete the service
//   neev_helper.exe agent       — session-agent loop (the service launches this)
//   neev_helper.exe             — service entry point (invoked by the SCM)

#include <windows.h>
#include <wtsapi32.h>
#include <userenv.h>
#include <string>
#include <vector>
#include <cstdarg>
#include <cstdio>

static const wchar_t* kServiceName = L"NeevRemoteHelper";
static const wchar_t* kDisplayName = L"Neev Remote Helper";
static const wchar_t* kLogDir = L"C:\\ProgramData\\NeevRemote";
static const wchar_t* kLogPath = L"C:\\ProgramData\\NeevRemote\\helper.log";

// --------------------------------------------------------------------------
// Logging (single shared file; both the service and the agent append to it).
// --------------------------------------------------------------------------
static void Log(const wchar_t* tag, const wchar_t* fmt, ...) {
  CreateDirectoryW(kLogDir, nullptr);
  FILE* f = nullptr;
  if (_wfopen_s(&f, kLogPath, L"a+, ccs=UTF-8") != 0 || !f) return;
  SYSTEMTIME st;
  GetLocalTime(&st);
  fwprintf(f, L"[%04d-%02d-%02d %02d:%02d:%02d] %ls: ", st.wYear, st.wMonth,
           st.wDay, st.wHour, st.wMinute, st.wSecond, tag);
  va_list args;
  va_start(args, fmt);
  vfwprintf(f, fmt, args);
  va_end(args);
  fwprintf(f, L"\n");
  fclose(f);
}

static std::wstring SelfPath() {
  wchar_t buf[MAX_PATH] = {0};
  GetModuleFileNameW(nullptr, buf, MAX_PATH);
  return std::wstring(buf);
}

// --------------------------------------------------------------------------
// Service install / uninstall.
// --------------------------------------------------------------------------
static int InstallService() {
  SC_HANDLE scm = OpenSCManagerW(nullptr, nullptr, SC_MANAGER_ALL_ACCESS);
  if (!scm) {
    wprintf(L"OpenSCManager failed: %lu (run elevated)\n", GetLastError());
    return 1;
  }
  std::wstring path = L"\"" + SelfPath() + L"\"";
  SC_HANDLE svc = CreateServiceW(
      scm, kServiceName, kDisplayName, SERVICE_ALL_ACCESS,
      SERVICE_WIN32_OWN_PROCESS, SERVICE_AUTO_START, SERVICE_ERROR_NORMAL,
      path.c_str(), nullptr, nullptr, nullptr,
      nullptr /* LocalSystem */, nullptr);
  if (!svc) {
    DWORD e = GetLastError();
    if (e == ERROR_SERVICE_EXISTS) {
      svc = OpenServiceW(scm, kServiceName, SERVICE_ALL_ACCESS);
    } else {
      wprintf(L"CreateService failed: %lu\n", e);
      CloseServiceHandle(scm);
      return 1;
    }
  }
  if (svc) {
    StartServiceW(svc, 0, nullptr);
    CloseServiceHandle(svc);
    wprintf(L"Installed + started '%ls'. Log: %ls\n", kServiceName, kLogPath);
  }
  CloseServiceHandle(scm);
  return 0;
}

static int UninstallService() {
  SC_HANDLE scm = OpenSCManagerW(nullptr, nullptr, SC_MANAGER_ALL_ACCESS);
  if (!scm) {
    wprintf(L"OpenSCManager failed: %lu (run elevated)\n", GetLastError());
    return 1;
  }
  SC_HANDLE svc = OpenServiceW(scm, kServiceName, SERVICE_ALL_ACCESS);
  if (svc) {
    SERVICE_STATUS st = {0};
    ControlService(svc, SERVICE_CONTROL_STOP, &st);
    Sleep(500);
    DeleteService(svc);
    CloseServiceHandle(svc);
    wprintf(L"Uninstalled '%ls'\n", kServiceName);
  } else {
    wprintf(L"Service '%ls' not found\n", kServiceName);
  }
  CloseServiceHandle(scm);
  return 0;
}

// --------------------------------------------------------------------------
// Launch the agent AS SYSTEM in the active interactive session.
//
// A service lives in session 0 (isolated), so SendInput/capture there never
// touch the user's screen. We duplicate our own SYSTEM token, retarget it to
// the active console session, and CreateProcessAsUser — yielding a SYSTEM
// process inside the user's session that CAN reach the secure desktop and
// inject into elevated windows.
// --------------------------------------------------------------------------
// Pick the session that is actively receiving user input. With RDP the physical
// console (WTSGetActiveConsoleSessionId) is LOCKED and the user is in a separate
// session, so we must target the WTSActive session — that's where the user (and
// their UAC prompt) actually is. Fall back to the console.
static DWORD GetTargetSessionId() {
  DWORD result = 0xFFFFFFFF;
  PWTS_SESSION_INFOW sessions = nullptr;
  DWORD count = 0;
  if (WTSEnumerateSessionsW(WTS_CURRENT_SERVER_HANDLE, 0, 1, &sessions,
                            &count)) {
    for (DWORD i = 0; i < count; i++) {
      if (sessions[i].State == WTSActive) {
        result = sessions[i].SessionId;
        break;
      }
    }
    WTSFreeMemory(sessions);
  }
  if (result == 0xFFFFFFFF) result = WTSGetActiveConsoleSessionId();
  return result;
}

static HANDLE LaunchAgentInSession(DWORD sid) {
  if (sid == 0xFFFFFFFF) {
    Log(L"svc", L"no target session yet");
    return nullptr;
  }

  HANDLE selfTok = nullptr;
  if (!OpenProcessToken(GetCurrentProcess(),
                        TOKEN_DUPLICATE | TOKEN_ADJUST_SESSIONID |
                            TOKEN_QUERY | TOKEN_ASSIGN_PRIMARY,
                        &selfTok)) {
    Log(L"svc", L"OpenProcessToken failed: %lu", GetLastError());
    return nullptr;
  }
  HANDLE primary = nullptr;
  if (!DuplicateTokenEx(selfTok, MAXIMUM_ALLOWED, nullptr, SecurityImpersonation,
                        TokenPrimary, &primary)) {
    Log(L"svc", L"DuplicateTokenEx failed: %lu", GetLastError());
    CloseHandle(selfTok);
    return nullptr;
  }
  CloseHandle(selfTok);

  // Retarget the SYSTEM token to the user's session.
  if (!SetTokenInformation(primary, TokenSessionId, &sid, sizeof(sid))) {
    Log(L"svc", L"SetTokenInformation(session=%lu) failed: %lu", sid,
        GetLastError());
    // continue anyway — some configs still launch into the right session
  }

  LPVOID env = nullptr;
  CreateEnvironmentBlock(&env, primary, FALSE);

  STARTUPINFOW si = {0};
  si.cb = sizeof(si);
  si.lpDesktop = const_cast<LPWSTR>(L"winsta0\\default");
  PROCESS_INFORMATION pi = {0};
  std::wstring cmd = L"\"" + SelfPath() + L"\" agent";
  std::wstring mutableCmd = cmd;  // CreateProcess may modify the buffer

  BOOL ok = CreateProcessAsUserW(
      primary, nullptr, &mutableCmd[0], nullptr, nullptr, FALSE,
      CREATE_UNICODE_ENVIRONMENT | CREATE_NO_WINDOW, env, nullptr, &si, &pi);

  if (env) DestroyEnvironmentBlock(env);
  CloseHandle(primary);

  if (!ok) {
    Log(L"svc", L"CreateProcessAsUser(agent) failed: %lu", GetLastError());
    return nullptr;
  }
  Log(L"svc", L"launched agent in session %lu (pid %lu)", sid, pi.dwProcessId);
  CloseHandle(pi.hThread);
  return pi.hProcess;  // caller owns; used to detect agent exit
}

// --------------------------------------------------------------------------
// Service control + main loop. Keeps exactly one agent alive, relaunching it
// when it exits (e.g. session switch / logoff).
// --------------------------------------------------------------------------
static SERVICE_STATUS g_status = {0};
static SERVICE_STATUS_HANDLE g_statusHandle = nullptr;
static HANDLE g_stopEvent = nullptr;

static void SetState(DWORD state) {
  g_status.dwServiceType = SERVICE_WIN32_OWN_PROCESS;
  g_status.dwCurrentState = state;
  g_status.dwControlsAccepted = (state == SERVICE_RUNNING) ? SERVICE_ACCEPT_STOP : 0;
  SetServiceStatus(g_statusHandle, &g_status);
}

static void WINAPI ServiceCtrl(DWORD ctrl) {
  if (ctrl == SERVICE_CONTROL_STOP || ctrl == SERVICE_CONTROL_SHUTDOWN) {
    SetState(SERVICE_STOP_PENDING);
    if (g_stopEvent) SetEvent(g_stopEvent);
  }
}

static void WINAPI ServiceMain(DWORD, LPWSTR*) {
  g_statusHandle = RegisterServiceCtrlHandlerW(kServiceName, ServiceCtrl);
  if (!g_statusHandle) return;
  g_stopEvent = CreateEventW(nullptr, TRUE, FALSE, nullptr);
  SetState(SERVICE_RUNNING);
  Log(L"svc", L"service started (LOCAL SYSTEM, session 0)");

  HANDLE agent = nullptr;
  DWORD agentSession = 0xFFFFFFFF;
  for (;;) {
    DWORD target = GetTargetSessionId();
    bool dead = (!agent || WaitForSingleObject(agent, 0) == WAIT_OBJECT_0);
    bool moved = (agent && target != 0xFFFFFFFF && target != agentSession);
    if (dead || moved) {
      if (agent) {
        if (moved) {
          Log(L"svc", L"active session %lu -> %lu; relaunching agent",
              agentSession, target);
          TerminateProcess(agent, 0);
        } else {
          Log(L"svc", L"agent exited; relaunching");
        }
        CloseHandle(agent);
        agent = nullptr;
      }
      agent = LaunchAgentInSession(target);
      agentSession = target;
    }
    if (WaitForSingleObject(g_stopEvent, 3000) == WAIT_OBJECT_0) break;
  }

  if (agent) {
    TerminateProcess(agent, 0);
    CloseHandle(agent);
  }
  Log(L"svc", L"service stopping");
  SetState(SERVICE_STOPPED);
}

// --------------------------------------------------------------------------
// Phase 2: capture a desktop by name via GDI. The secure (Winlogon) desktop is
// NOT DWM-composited, so a plain BitBlt captures it correctly (unlike the
// normal desktop, which needs DXGI). The agent runs as SYSTEM, so it may
// OpenInputDesktop(Winlogon), SetThreadDesktop onto it, and BitBlt.
// --------------------------------------------------------------------------
static bool SaveHBitmapToBmp(HBITMAP hbm, HDC hdc, int w, int h,
                             const wchar_t* path) {
  BITMAPINFOHEADER bi = {0};
  bi.biSize = sizeof(bi);
  bi.biWidth = w;
  bi.biHeight = h;  // bottom-up
  bi.biPlanes = 1;
  bi.biBitCount = 24;
  bi.biCompression = BI_RGB;
  const int rowSize = ((w * 3 + 3) & ~3);
  const int imgSize = rowSize * h;
  std::vector<BYTE> bits(imgSize);
  if (!GetDIBits(hdc, hbm, 0, h, bits.data(),
                 reinterpret_cast<BITMAPINFO*>(&bi), DIB_RGB_COLORS)) {
    return false;
  }
  BITMAPFILEHEADER bf = {0};
  bf.bfType = 0x4D42;  // 'BM'
  bf.bfOffBits = sizeof(bf) + sizeof(bi);
  bf.bfSize = bf.bfOffBits + imgSize;
  HANDLE hf = CreateFileW(path, GENERIC_WRITE, 0, nullptr, CREATE_ALWAYS,
                          FILE_ATTRIBUTE_NORMAL, nullptr);
  if (hf == INVALID_HANDLE_VALUE) return false;
  DWORD wr = 0;
  WriteFile(hf, &bf, sizeof(bf), &wr, nullptr);
  WriteFile(hf, &bi, sizeof(bi), &wr, nullptr);
  WriteFile(hf, bits.data(), imgSize, &wr, nullptr);
  CloseHandle(hf);
  return true;
}

static bool CaptureInputDesktopToBmp(const wchar_t* path) {
  HDESK hDesk = OpenInputDesktop(0, FALSE, GENERIC_ALL);
  if (!hDesk) {
    Log(L"agent", L"capture: OpenInputDesktop failed %lu", GetLastError());
    return false;
  }
  HDESK hPrev = GetThreadDesktop(GetCurrentThreadId());
  if (!SetThreadDesktop(hDesk)) {
    Log(L"agent", L"capture: SetThreadDesktop failed %lu", GetLastError());
    CloseDesktop(hDesk);
    return false;
  }

  bool ok = false;
  HDC hScreen = GetDC(nullptr);
  if (hScreen) {
    int x = GetSystemMetrics(SM_XVIRTUALSCREEN);
    int y = GetSystemMetrics(SM_YVIRTUALSCREEN);
    int w = GetSystemMetrics(SM_CXVIRTUALSCREEN);
    int h = GetSystemMetrics(SM_CYVIRTUALSCREEN);
    if (w <= 0 || h <= 0) {
      w = GetSystemMetrics(SM_CXSCREEN);
      h = GetSystemMetrics(SM_CYSCREEN);
      x = 0;
      y = 0;
    }
    HDC hMem = CreateCompatibleDC(hScreen);
    HBITMAP hbm = CreateCompatibleBitmap(hScreen, w, h);
    if (hMem && hbm) {
      HGDIOBJ oldObj = SelectObject(hMem, hbm);
      BitBlt(hMem, 0, 0, w, h, hScreen, x, y, SRCCOPY);
      SelectObject(hMem, oldObj);  // deselect before GetDIBits
      ok = SaveHBitmapToBmp(hbm, hScreen, w, h, path);
    }
    if (hbm) DeleteObject(hbm);
    if (hMem) DeleteDC(hMem);
    ReleaseDC(nullptr, hScreen);
  }

  SetThreadDesktop(hPrev);
  CloseDesktop(hDesk);
  if (ok) Log(L"agent", L"capture: wrote %ls", path);
  return ok;
}

// --------------------------------------------------------------------------
// Agent: runs as SYSTEM inside the interactive session. Detects the secure
// desktop and (Phase 2) captures it to a file so we can confirm we can SEE the
// UAC prompt.
// --------------------------------------------------------------------------
static int RunAgent() {
  Log(L"agent", L"agent started (session detection + capture running)");
  std::wstring last;
  for (;;) {
    std::wstring name = L"(unknown)";
    HDESK d = OpenInputDesktop(0, FALSE, DESKTOP_READOBJECTS);
    if (d) {
      wchar_t buf[256] = {0};
      DWORD needed = 0;
      if (GetUserObjectInformationW(d, UOI_NAME, buf, sizeof(buf), &needed)) {
        name = buf;
      }
      CloseDesktop(d);
    } else {
      name = L"(OpenInputDesktop failed: " +
             std::to_wstring(GetLastError()) + L")";
    }
    if (name != last) {
      bool secure = (_wcsicmp(name.c_str(), L"Winlogon") == 0);
      Log(L"agent", L"input desktop -> %ls%ls", name.c_str(),
          secure ? L"   <<< SECURE DESKTOP / UAC PROMPT ACTIVE" : L"");
      if (secure) {
        Sleep(600);  // let the UAC dialog finish drawing
        CaptureInputDesktopToBmp(
            L"C:\\ProgramData\\NeevRemote\\secure_capture.bmp");
      }
      last = name;
    }
    Sleep(400);
  }
}

int wmain(int argc, wchar_t** argv) {
  if (argc >= 2) {
    if (_wcsicmp(argv[1], L"install") == 0) return InstallService();
    if (_wcsicmp(argv[1], L"uninstall") == 0) return UninstallService();
    if (_wcsicmp(argv[1], L"agent") == 0) return RunAgent();
  }
  // No recognised argument: assume the SCM is starting us as a service.
  SERVICE_TABLE_ENTRYW table[] = {
      {const_cast<LPWSTR>(kServiceName), ServiceMain}, {nullptr, nullptr}};
  if (!StartServiceCtrlDispatcherW(table)) {
    wprintf(L"Neev Remote Helper (UAC support)\n"
            L"Usage: neev_helper.exe [install|uninstall|agent]\n");
    return 1;
  }
  return 0;
}
