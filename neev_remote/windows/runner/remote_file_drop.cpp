#include "remote_file_drop.h"

#include <windows.h>
#include <objidl.h>
#include <ole2.h>
#include <shlobj.h>
#include <shlwapi.h>
#include <shobjidl.h>

#include <atomic>
#include <cstring>
#include <new>

// A minimal IDataObject that advertises a group of virtual files
// (CFSTR_FILEDESCRIPTORW) whose contents (CFSTR_FILECONTENTS, TYMED_ISTREAM) are
// rendered on demand — i.e. the bytes are fetched only when the shell asks for
// them at paste time. This is the standard shell mechanism AnyDesk-style tools
// use for "transfer on paste, not on copy".
namespace {

UINT CfFileDescriptor() {
  static UINT cf = RegisterClipboardFormatW(CFSTR_FILEDESCRIPTORW);
  return cf;
}
UINT CfFileContents() {
  static UINT cf = RegisterClipboardFormatW(CFSTR_FILECONTENTS);
  return cf;
}
UINT CfPreferredDropEffect() {
  static UINT cf = RegisterClipboardFormatW(CFSTR_PREFERREDDROPEFFECT);
  return cf;
}

class RemoteFileDrop : public IDataObject {
 public:
  RemoteFileDrop(std::wstring token, std::vector<RemoteFileEntry> files)
      : ref_(1), token_(std::move(token)), files_(std::move(files)) {}

  // ---- IUnknown ----
  HRESULT STDMETHODCALLTYPE QueryInterface(REFIID riid, void** ppv) override {
    if (!ppv) return E_POINTER;
    // __uuidof avoids needing the IID_* symbols from uuid.lib at link time.
    if (riid == __uuidof(IUnknown) || riid == __uuidof(IDataObject)) {
      *ppv = static_cast<IDataObject*>(this);
      AddRef();
      return S_OK;
    }
    *ppv = nullptr;
    return E_NOINTERFACE;
  }
  ULONG STDMETHODCALLTYPE AddRef() override { return ++ref_; }
  ULONG STDMETHODCALLTYPE Release() override {
    ULONG n = --ref_;
    if (n == 0) delete this;
    return n;
  }

  // ---- IDataObject ----
  HRESULT STDMETHODCALLTYPE GetData(FORMATETC* fe, STGMEDIUM* stg) override {
    if (!fe || !stg) return E_INVALIDARG;
    ZeroMemory(stg, sizeof(*stg));

    // File group descriptor (names + sizes).
    if (fe->cfFormat == CfFileDescriptor() && (fe->tymed & TYMED_HGLOBAL)) {
      return MakeDescriptor(stg);
    }
    // Contents of one file, by lindex — fetched on demand.
    if (fe->cfFormat == CfFileContents() && (fe->tymed & TYMED_ISTREAM)) {
      return MakeContents(fe->lindex, stg);
    }
    // Preferred drop effect = COPY (so paste copies, never moves).
    if (fe->cfFormat == CfPreferredDropEffect() && (fe->tymed & TYMED_HGLOBAL)) {
      HGLOBAL h = GlobalAlloc(GHND, sizeof(DWORD));
      if (!h) return E_OUTOFMEMORY;
      *static_cast<DWORD*>(GlobalLock(h)) = DROPEFFECT_COPY;
      GlobalUnlock(h);
      stg->tymed = TYMED_HGLOBAL;
      stg->hGlobal = h;
      stg->pUnkForRelease = nullptr;
      return S_OK;
    }
    return DV_E_FORMATETC;
  }

  HRESULT STDMETHODCALLTYPE GetDataHere(FORMATETC*, STGMEDIUM*) override {
    return E_NOTIMPL;
  }

  HRESULT STDMETHODCALLTYPE QueryGetData(FORMATETC* fe) override {
    if (!fe) return E_INVALIDARG;
    if (fe->cfFormat == CfFileDescriptor() && (fe->tymed & TYMED_HGLOBAL))
      return S_OK;
    if (fe->cfFormat == CfFileContents() && (fe->tymed & TYMED_ISTREAM))
      return S_OK;
    if (fe->cfFormat == CfPreferredDropEffect() && (fe->tymed & TYMED_HGLOBAL))
      return S_OK;
    return DV_E_FORMATETC;
  }

  HRESULT STDMETHODCALLTYPE GetCanonicalFormatEtc(FORMATETC*,
                                                  FORMATETC* out) override {
    if (out) out->ptd = nullptr;
    return E_NOTIMPL;
  }
  HRESULT STDMETHODCALLTYPE SetData(FORMATETC*, STGMEDIUM*, BOOL) override {
    return E_NOTIMPL;
  }

  HRESULT STDMETHODCALLTYPE EnumFormatEtc(DWORD dir,
                                          IEnumFORMATETC** out) override {
    if (dir != DATADIR_GET || !out) return E_NOTIMPL;
    FORMATETC fmts[3] = {};
    fmts[0] = {static_cast<CLIPFORMAT>(CfFileDescriptor()), nullptr,
               DVASPECT_CONTENT, -1, TYMED_HGLOBAL};
    fmts[1] = {static_cast<CLIPFORMAT>(CfFileContents()), nullptr,
               DVASPECT_CONTENT, -1, TYMED_ISTREAM};
    fmts[2] = {static_cast<CLIPFORMAT>(CfPreferredDropEffect()), nullptr,
               DVASPECT_CONTENT, -1, TYMED_HGLOBAL};
    return SHCreateStdEnumFmtEtc(3, fmts, out);
  }

  HRESULT STDMETHODCALLTYPE DAdvise(FORMATETC*, DWORD, IAdviseSink*,
                                    DWORD*) override {
    return OLE_E_ADVISENOTSUPPORTED;
  }
  HRESULT STDMETHODCALLTYPE DUnadvise(DWORD) override {
    return OLE_E_ADVISENOTSUPPORTED;
  }
  HRESULT STDMETHODCALLTYPE EnumDAdvise(IEnumSTATDATA**) override {
    return OLE_E_ADVISENOTSUPPORTED;
  }

 private:
  HRESULT MakeDescriptor(STGMEDIUM* stg) {
    size_t count = files_.size();
    size_t bytes = sizeof(FILEGROUPDESCRIPTORW) +
                   (count > 0 ? (count - 1) : 0) * sizeof(FILEDESCRIPTORW);
    HGLOBAL h = GlobalAlloc(GHND, bytes);
    if (!h) return E_OUTOFMEMORY;
    auto* fgd = static_cast<FILEGROUPDESCRIPTORW*>(GlobalLock(h));
    fgd->cItems = static_cast<UINT>(count);
    for (size_t i = 0; i < count; i++) {
      FILEDESCRIPTORW& fd = fgd->fgd[i];
      fd.dwFlags = FD_FILESIZE | FD_PROGRESSUI;
      fd.nFileSizeLow = static_cast<DWORD>(files_[i].size & 0xFFFFFFFF);
      fd.nFileSizeHigh = static_cast<DWORD>(files_[i].size >> 32);
      wcsncpy_s(fd.cFileName, MAX_PATH, files_[i].name.c_str(), _TRUNCATE);
    }
    GlobalUnlock(h);
    stg->tymed = TYMED_HGLOBAL;
    stg->hGlobal = h;
    stg->pUnkForRelease = nullptr;
    return S_OK;
  }

  HRESULT MakeContents(LONG lindex, STGMEDIUM* stg) {
    uint32_t index = (lindex < 0) ? 0 : static_cast<uint32_t>(lindex);
    if (index >= files_.size()) return DV_E_LINDEX;
    // Pull the bytes on demand (blocks until Dart delivers or times out).
    std::vector<BYTE> bytes;
    if (!FetchRemoteFileBytes(token_, index, bytes)) return E_FAIL;
    // Wrap in a memory-backed IStream the shell reads to write the file.
    IStream* s = SHCreateMemStream(
        bytes.empty() ? reinterpret_cast<const BYTE*>("") : bytes.data(),
        static_cast<UINT>(bytes.size()));
    if (!s) return E_OUTOFMEMORY;
    stg->tymed = TYMED_ISTREAM;
    stg->pstm = s;
    stg->pUnkForRelease = nullptr;
    return S_OK;
  }

  std::atomic<ULONG> ref_;
  std::wstring token_;
  std::vector<RemoteFileEntry> files_;
};

}  // namespace

// ---------------------------------------------------------------------------
// Dedicated clipboard STA thread.
//
// When another app pastes, COM marshals IDataObject::GetData onto the STA thread
// that called OleSetClipboard. Our GetData blocks (waiting for bytes from Dart).
// If that were the Flutter UI thread, the Dart fetch-poller could never run and
// paste would deadlock. So we own a private STA thread with its own message pump
// purely for the clipboard object: GetData blocks HERE, while the UI thread
// stays free to service the fetch and deliver the bytes.
// ---------------------------------------------------------------------------
namespace {

const UINT WM_SET_CLIPBOARD = WM_APP + 71;

struct SetClipboardMsg {
  std::wstring token;
  std::vector<RemoteFileEntry> files;
};

HANDLE g_staThread = nullptr;
DWORD g_staThreadId = 0;
HANDLE g_staReady = nullptr;

DWORD WINAPI ClipboardStaThread(LPVOID) {
  OleInitialize(nullptr);
  // Force the message queue to exist before anyone posts to this thread.
  MSG msg;
  PeekMessageW(&msg, nullptr, WM_USER, WM_USER, PM_NOREMOVE);
  if (g_staReady) SetEvent(g_staReady);
  while (GetMessageW(&msg, nullptr, 0, 0) > 0) {
    if (msg.message == WM_SET_CLIPBOARD) {
      auto* payload = reinterpret_cast<SetClipboardMsg*>(msg.lParam);
      if (payload) {
        auto* obj = new (std::nothrow) RemoteFileDrop(payload->token,
                                                      payload->files);
        if (obj) {
          OleSetClipboard(obj);
          obj->Release();  // clipboard holds its own ref
        }
        delete payload;
      }
    }
    TranslateMessage(&msg);
    DispatchMessageW(&msg);
  }
  OleUninitialize();
  return 0;
}

bool EnsureStaThread() {
  if (g_staThread) return true;
  g_staReady = CreateEventW(nullptr, TRUE, FALSE, nullptr);
  g_staThread =
      CreateThread(nullptr, 0, ClipboardStaThread, nullptr, 0, &g_staThreadId);
  if (!g_staThread) return false;
  // Wait until the queue is ready so the first PostThreadMessage can't be lost.
  if (g_staReady) WaitForSingleObject(g_staReady, 5000);
  return true;
}

}  // namespace

bool SetRemoteFileClipboard(const std::wstring& token,
                            const std::vector<RemoteFileEntry>& files) {
  if (files.empty()) return false;
  if (!EnsureStaThread()) return false;
  auto* payload = new (std::nothrow) SetClipboardMsg{token, files};
  if (!payload) return false;
  if (!PostThreadMessageW(g_staThreadId, WM_SET_CLIPBOARD, 0,
                          reinterpret_cast<LPARAM>(payload))) {
    delete payload;
    return false;
  }
  return true;
}
