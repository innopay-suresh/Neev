/*
 * capture_windows.c — Windows DXGI Desktop Duplication screen capture
 *
 * Uses DXGI Desktop Duplication API for efficient GPU-accelerated capture.
 * The system cursor is captured separately via GetCursorInfo/GetIconInfo
 * and returned as CursorInfo so the viewer can render it as an overlay.
 *
 * This code runs in the USER's desktop session (not as a service) so it
 * has full access to the interactive display via DXGI.
 */

#include "capture_windows.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <stdarg.h>

/*
 * Bridge to Go logging — set by Go on init so C can log via zerolog.
 * The Go side calls set_log_callback() with a pointer to a printf-like function.
 * If not set, we fall back to OutputDebugString + stderr.
 */
typedef void (*LogFn)(const char* msg);
static LogFn g_log_fn = NULL;

void set_log_callback(LogFn fn) {
    g_log_fn = fn;
}

/*
 * Log to: (1) Go callback if registered, (2) OutputDebugString (DebugView),
 * (3) stderr (visible in Go agent output).
 */
static void debug_log(const char* fmt, ...) {
    char buf[512];
    va_list args;
    va_start(args, fmt);
    vsnprintf(buf, sizeof(buf), fmt, args);
    va_end(args);

    if (g_log_fn != NULL) {
        g_log_fn(buf);
    }
    OutputDebugStringA(buf);
    /* Also write to stderr so Go's log package captures it */
    fprintf(stderr, "[capture] %s\n", buf);
}

/*
 * Like debug_log but always includes the HRESULT in a structured way
 * so the Go side can parse it easily.
 */
static void debug_log_hr(const char* step, HRESULT hr) {
    char buf[512];
    snprintf(buf, sizeof(buf),
             "[capture] DXGI STEP '%s' FAILED hr=0x%08X (%lu)\n",
             step, (unsigned int)hr, (unsigned long)hr);

    if (g_log_fn != NULL) {
        g_log_fn(buf);
    }
    OutputDebugStringA(buf);
    fprintf(stderr, "%s", buf);
}



/* --- Display enumeration via EnumDisplayMonitors --- */
static WinDisplayInfo*  g_win_displays   = NULL;
static int              g_win_display_count = 0;

static BOOL CALLBACK enum_monitor_cb(HMONITOR hMonitor, HDC hdcMonitor,
                                     LPRECT lprcMonitor, LPARAM dwData) {
    (void)hdcMonitor; (void)lprcMonitor;
    MONITORINFOEX mi = {0};
    mi.cbSize = sizeof(mi);
    if (!GetMonitorInfoA(hMonitor, (MONITORINFO*)&mi)) return TRUE;
    WinDisplayInfo info = {0};
    info.width = mi.rcMonitor.right - mi.rcMonitor.left;
    info.height = mi.rcMonitor.bottom - mi.rcMonitor.top;
    info.isPrimary = (mi.dwFlags & MONITORINFOF_PRIMARY) ? 1 : 0;
    info.hMonitor = (void*)hMonitor;
    WinDisplayInfo* tmp = realloc(g_win_displays, (g_win_display_count + 1) * sizeof(WinDisplayInfo));
    if (tmp) {
        g_win_displays = tmp;
        g_win_displays[g_win_display_count++] = info;
    }
    (void)dwData;
    return TRUE;
}

WinDisplayList get_active_displays_win(void) {
    if (g_win_displays) { free(g_win_displays); g_win_displays = NULL; }
    g_win_display_count = 0;
    EnumDisplayMonitors(NULL, NULL, enum_monitor_cb, 0);
    WinDisplayList list = {0};
    list.displays = g_win_displays;
    list.count = g_win_display_count;
    return list;
}

void free_display_list_win(WinDisplayList list) {
    (void)list;
    g_win_displays = NULL;
    g_win_display_count = 0;
}

/* Detect cursor type from HCURSOR.
 * Returns: 0=arrow, 1=ibeam, 2=cross, 3=wait/hourglass, 4=resize, 5=hand, 6=unknown
 */
static int detect_cursor_type(HCURSOR hCursor) {
    if (!hCursor) return 0;
    /* Compare handle against standard system-cursor handles loaded by ordinal.
     * IDC_ constants are in winuser.h but may need explicit headers; use raw IDs:
     *   32512 = IDC_ARROW, 32513 = IDC_IBEAM, 32514 = IDC_CROSS,
     *   32515 = IDC_WAIT, 32646 = IDC_SIZEALL, 32649 = IDC_HAND
     */
    static const struct { int id; int type; } cursor_map[] = {
        { 32512, 0 },  /* IDC_ARROW  */
        { 32513, 1 },  /* IDC_IBEAM  */
        { 32514, 2 },  /* IDC_CROSS  */
        { 32515, 3 },  /* IDC_WAIT   */
        { 32646, 4 },  /* IDC_SIZEALL */
        { 32649, 5 },  /* IDC_HAND   */
        { 32640, 4 },  /* IDC_SIZENWSE */
        { 32642, 4 },  /* IDC_SIZENESW */
        { 32643, 4 },  /* IDC_SIZEWE   */
        { 32645, 4 },  /* IDC_SIZENS   */
    };
    HMODULE user32 = GetModuleHandleW(L"user32.dll");
    if (!user32) return 6;
    for (int i = 0; i < (int)(sizeof(cursor_map)/sizeof(cursor_map[0])); i++) {
        HANDLE hStd = LoadImageA(user32, MAKEINTRESOURCE(cursor_map[i].id),
                                  IMAGE_CURSOR, 0, 0, LR_SHARED);
        if (hStd && hStd == (HANDLE)hCursor) {
            return cursor_map[i].type;
        }
    }
    return 6; /* custom/unknown cursor */
}

/* Module-level D3D11/DXGI state — created once, reused across frames. */
static ID3D11Device*            g_device       = NULL;
static ID3D11DeviceContext*     g_context      = NULL;
static IDXGIOutputDuplication*  g_duplication  = NULL;
static ID3D11Texture2D*         g_staging      = NULL;
static int                      g_width        = 0;
static int                      g_height       = 0;
static int                      g_initialized  = 0;

/* Cursor cache — avoid recreating the cursor bitmap every frame.
 * Updated only when the cursor shape changes (detected via cursor hash). */
static HCURSOR                  g_last_cursor  = NULL;
static HBITMAP                  g_cursor_bmp   = NULL;
static void*                    g_cursor_bits  = NULL;
static int                      g_cursor_w     = 0;
static int                      g_cursor_h     = 0;
static int                      g_cursor_hotX  = 0;
static int                      g_cursor_hotY  = 0;

static int is_remote_session(void) {
    return GetSystemMetrics(SM_REMOTESESSION);
}

static HRESULT init_dxgi(void) {
    IDXGIFactory1* factory = NULL;
    /* FORCE GDI FALLBACK: DXGI Desktop Duplication often returns completely black frames
       on laptops with Hybrid GPUs (Intel+NVIDIA) because the app runs on the dedicated GPU
       while the desktop is composed on the integrated GPU. GDI BitBlt is 100% reliable. */
    return E_FAIL;
    HRESULT hr = CreateDXGIFactory1(&IID_IDXGIFactory1, (void**)&factory);
    if (FAILED(hr)) {
        debug_log("[RemoteAgent] init_dxgi: CreateDXGIFactory1 failed hr=0x%08X\n", hr);
        return hr;
    }

    IDXGIAdapter1* adapter = NULL;
    IDXGIOutput* output = NULL;
    UINT adapterIdx = 0;

    while (factory->lpVtbl->EnumAdapters1(factory, adapterIdx, &adapter) != DXGI_ERROR_NOT_FOUND) {
        hr = adapter->lpVtbl->EnumOutputs(adapter, 0, &output);
        if (hr != DXGI_ERROR_NOT_FOUND && output != NULL) {
            debug_log("[RemoteAgent] init_dxgi: Found display output on adapter %u\n", adapterIdx);
            break;
        }
        adapter->lpVtbl->Release(adapter);
        adapter = NULL;
        adapterIdx++;
    }
    factory->lpVtbl->Release(factory);

    if (!adapter || !output) {
        debug_log("[RemoteAgent] init_dxgi: No adapter with an output found!\n");
        return E_FAIL;
    }

    D3D_FEATURE_LEVEL featureLevel;
    debug_log("[RemoteAgent] init_dxgi: starting D3D11CreateDevice...\n");
    hr = D3D11CreateDevice(
        (IDXGIAdapter*)adapter,
        D3D_DRIVER_TYPE_UNKNOWN,
        NULL,
        0,
        NULL, 0,
        D3D11_SDK_VERSION,
        &g_device,
        &featureLevel,
        &g_context
    );
    adapter->lpVtbl->Release(adapter);

    if (FAILED(hr)) {
        debug_log("[RemoteAgent] init_dxgi: D3D11CreateDevice failed hr=0x%08X\n", hr);
        output->lpVtbl->Release(output);
        return hr;
    }

    IDXGIOutput1* output1 = NULL;
    hr = output->lpVtbl->QueryInterface(output, &IID_IDXGIOutput1, (void**)&output1);
    output->lpVtbl->Release(output);
    if (FAILED(hr)) {
        debug_log("[RemoteAgent] init_dxgi: QueryInterface IDXGIOutput1 failed hr=0x%08X\n", hr);
        return hr;
    }

    debug_log("[RemoteAgent] init_dxgi: calling DuplicateOutput...\n");
    hr = output1->lpVtbl->DuplicateOutput(output1, (IUnknown*)g_device, &g_duplication);
    output1->lpVtbl->Release(output1);
    if (FAILED(hr)) {
        debug_log("[RemoteAgent] init_dxgi: DuplicateOutput failed hr=0x%08X\n", hr);
        return hr;
    }

    debug_log("[RemoteAgent] init_dxgi: SUCCESS\n");
    return hr;
}

/* Build a 32-bit BGRA bitmap from an HCURSOR at 32x32 (standard size).
 * Returns 0 on failure, 1 on success. Bitmap must be freed with free_cursor_mask. */
static int build_cursor_bitmap(HCURSOR cursor, HBITMAP* outBmp, void** outBits, int* outW, int* outH, int* outHotX, int* outHotY) {
    if (!cursor) return 0;

    ICONINFO ii = {0};
    if (!GetIconInfo(cursor, &ii)) return 0;

    *outHotX = ii.xHotspot;
    *outHotY = ii.yHotspot;

    /* Get the icon's color bitmap dimensions */
    BITMAP bmp = {0};
    if (ii.hbmColor) {
        GetObject(ii.hbmColor, sizeof(BITMAP), &bmp);
    } else if (ii.hbmMask) {
        GetObject(ii.hbmMask, sizeof(BITMAP), &bmp);
        if (bmp.bmHeight == 0) bmp.bmHeight = bmp.bmWidth; /* mask bitmaps are doubled height */
    }

    int w = bmp.bmWidth > 0 ? bmp.bmWidth : 32;
    int h = bmp.bmHeight > 0 ? bmp.bmHeight : 32;

    /* If the icon has a mask only (no color), the height is 2x (mask+color combined) */
    if (!ii.hbmColor) h = h / 2;

    /* Create a memory DC and draw the icon into it */
    HDC hdcScreen = GetDC(NULL);
    HDC hdcMem = CreateCompatibleDC(hdcScreen);
    BITMAPINFO bmi = {0};
    bmi.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
    bmi.bmiHeader.biWidth = w;
    bmi.bmiHeader.biHeight = -h; /* top-down */
    bmi.bmiHeader.biPlanes = 1;
    bmi.bmiHeader.biBitCount = 32;
    bmi.bmiHeader.biCompression = BI_RGB;

    void* bits = NULL;
    HBITMAP dibBmp = CreateDIBSection(hdcMem, &bmi, DIB_RGB_COLORS, &bits, NULL, 0);
    if (!dibBmp || !bits) {
        DeleteDC(hdcMem);
        ReleaseDC(NULL, hdcScreen);
        if (dibBmp) DeleteObject(dibBmp);
        return 0;
    }

    HBITMAP oldBmp = SelectObject(hdcMem, dibBmp);
    memset(bits, 0, w * h * 4); /* transparent BGRA */

    /* Draw the icon with transparency */
    HBRUSH oldBrush = SelectObject(hdcMem, GetStockObject(DC_BRUSH));
    SetDCBrushColor(hdcMem, RGB(0, 0, 0));
    PatBlt(hdcMem, 0, 0, w, h, PATCOPY);

    if (ii.hbmColor) {
        HDC hdcColor = CreateCompatibleDC(hdcScreen);
        HBITMAP oldColor = SelectObject(hdcColor, ii.hbmColor);
        BitBlt(hdcMem, 0, 0, w, h, hdcColor, 0, 0, SRCCOPY);
        SelectObject(hdcColor, oldColor);
        DeleteDC(hdcColor);
    }

    if (ii.hbmMask) {
        /* Mask is AND mask — white=transparent, black=opaque for arrow cursor */
        HDC hdcMask = CreateCompatibleDC(hdcScreen);
        HBITMAP oldMask = SelectObject(hdcMask, ii.hbmMask);
        /* Draw mask (handle doubled-height mask) */
        int maskH = bmp.bmHeight;
        if (!ii.hbmColor) maskH = bmp.bmHeight / 2;
        /* The AND mask is below the XOR mask in the combined bitmap */
        int andMaskY = ii.hbmColor ? 0 : maskH;
        BitBlt(hdcMem, 0, 0, w, maskH, hdcMask, 0, andMaskY, SRCAND);
        SelectObject(hdcMask, oldMask);
        DeleteDC(hdcMask);
    }

    SelectObject(hdcMem, oldBmp);
    SelectObject(hdcMem, oldBrush);
    DeleteDC(hdcMem);
    ReleaseDC(NULL, hdcScreen);

    if (ii.hbmColor) DeleteObject(ii.hbmColor);
    if (ii.hbmMask) DeleteObject(ii.hbmMask);

    *outBmp = dibBmp;
    *outBits = bits;
    *outW = w;
    *outH = h;
    return 1;
}

static void ensure_cursor_cache(void) {
    CURSORINFO ci = {0};
    ci.cbSize = sizeof(CURSORINFO);
    if (!GetCursorInfo(&ci)) return;

    if (ci.hCursor != g_last_cursor) {
        /* Cursor shape changed — rebuild the bitmap cache */
        if (g_cursor_bmp) {
            DeleteObject(g_cursor_bmp);
            g_cursor_bmp = NULL;
            g_cursor_bits = NULL;
        }
        g_last_cursor = ci.hCursor;
        if (ci.hCursor) {
            build_cursor_bitmap(ci.hCursor, &g_cursor_bmp, &g_cursor_bits, &g_cursor_w, &g_cursor_h, &g_cursor_hotX, &g_cursor_hotY);
            debug_log("[RemoteAgent] cursor bitmap: %dx%d hot=(%d,%d)\n", g_cursor_w, g_cursor_h, g_cursor_hotX, g_cursor_hotY);
        }
    }
}

void get_cursor_info(CursorInfo* info) {
    memset(info, 0, sizeof(*info));
    if (!info) return;

    CURSORINFO ci = {0};
    ci.cbSize = sizeof(CURSORINFO);
    if (!GetCursorInfo(&ci)) {
        info->visible = 0;
        return;
    }

    info->visible = (ci.flags & CURSOR_SHOWING) ? 1 : 0;
    info->x = (int)(ci.ptScreenPos.x);
    info->y = (int)(ci.ptScreenPos.y);
    info->cursorType = detect_cursor_type(ci.hCursor);

    /* Update cursor bitmap cache if needed */
    ensure_cursor_cache();

    if (g_cursor_bits && g_cursor_w > 0) {
        /* Copy the cached cursor bitmap — caller frees with free_cursor_mask */
        int size = g_cursor_w * g_cursor_h * 4;
        unsigned char* buf = (unsigned char*)malloc(size);
        if (buf) {
            memcpy(buf, g_cursor_bits, size);
            info->mask = buf;
            info->width = g_cursor_w;
            info->height = g_cursor_h;
            info->hotX = g_cursor_hotX;
            info->hotY = g_cursor_hotY;
        }
    }
}

void free_cursor_mask(unsigned char* mask) {
    free(mask);
}

CaptureResult capture_frame_win(void) {
    CaptureResult result = {0};

    if (!g_initialized) {
        debug_log("[RemoteAgent] capture_frame_win: not initialized, calling init_dxgi...\n");
        HRESULT hr = init_dxgi();
        if (FAILED(hr)) {
            debug_log("[RemoteAgent] capture_frame_win: init_dxgi FAILED hr=0x%08X, using GDI fallback\n", hr);
            /* DXGI init failed — try GDI capture as last resort.
               This can happen in some headless RDP scenarios. */
            g_initialized = 1;
            g_width = GetSystemMetrics(SM_CXSCREEN);
            g_height = GetSystemMetrics(SM_CYSCREEN);
            debug_log("[RemoteAgent] GDI fallback init: %dx%d\n", g_width, g_height);
        }
        g_initialized = 1;
    }

    DXGI_OUTDUPL_FRAME_INFO frameInfo = {0};
    IDXGIResource* desktopResource = NULL;
    HRESULT hr;

    if (g_duplication) {
        hr = g_duplication->lpVtbl->AcquireNextFrame(g_duplication, 0, &frameInfo, &desktopResource);
        if (hr == DXGI_ERROR_WAIT_TIMEOUT) {
            result.status = STATUS_NO_NEW_FRAME;
            return result;
        }
        if (FAILED(hr)) {
            /* Desktop inaccessible — return no-new-frame to keep connection alive.
               If we lost access (e.g. UAC prompt, mode change), reinitialize next frame. */
            debug_log("[RemoteAgent] AcquireNextFrame failed hr=0x%08X\n", hr);
            if (hr == DXGI_ERROR_ACCESS_LOST || hr == E_ACCESSDENIED || hr == DXGI_ERROR_SESSION_DISCONNECTED) {
                if (g_duplication) { g_duplication->lpVtbl->Release(g_duplication); g_duplication = NULL; }
                if (g_context) { g_context->lpVtbl->Release(g_context); g_context = NULL; }
                if (g_device) { g_device->lpVtbl->Release(g_device); g_device = NULL; }
                if (g_staging) { g_staging->lpVtbl->Release(g_staging); g_staging = NULL; }
                g_initialized = 0;
            }
            result.status = STATUS_NO_NEW_FRAME;
            return result;
        }

        ID3D11Texture2D* gpuTex = NULL;
        hr = desktopResource->lpVtbl->QueryInterface(desktopResource, &IID_ID3D11Texture2D, (void**)&gpuTex);
        desktopResource->lpVtbl->Release(desktopResource);
        if (FAILED(hr)) {
            g_duplication->lpVtbl->ReleaseFrame(g_duplication);
            result.status = STATUS_ERROR;
            return result;
        }

        D3D11_TEXTURE2D_DESC desc = {0};
        gpuTex->lpVtbl->GetDesc(gpuTex, &desc);

        if (!g_staging || g_width != (int)desc.Width || g_height != (int)desc.Height) {
            if (g_staging) { g_staging->lpVtbl->Release(g_staging); g_staging = NULL; }
            D3D11_TEXTURE2D_DESC stagingDesc = desc;
            stagingDesc.Usage = D3D11_USAGE_STAGING;
            stagingDesc.BindFlags = 0;
            stagingDesc.CPUAccessFlags = D3D11_CPU_ACCESS_READ;
            stagingDesc.MiscFlags = 0;
            g_device->lpVtbl->CreateTexture2D(g_device, &stagingDesc, NULL, &g_staging);
            g_width = (int)desc.Width;
            g_height = (int)desc.Height;
            debug_log("[RemoteAgent] staging texture resized: %dx%d\n", g_width, g_height);
        }

        g_context->lpVtbl->CopyResource(g_context, (ID3D11Resource*)g_staging, (ID3D11Resource*)gpuTex);
        gpuTex->lpVtbl->Release(gpuTex);
        g_duplication->lpVtbl->ReleaseFrame(g_duplication);

        D3D11_MAPPED_SUBRESOURCE mapped = {0};
        hr = g_context->lpVtbl->Map(g_context, (ID3D11Resource*)g_staging, 0, D3D11_MAP_READ, 0, &mapped);
        if (FAILED(hr)) {
            result.status = STATUS_ERROR;
            return result;
        }

        int stride = (int)mapped.RowPitch;
        int size = stride * g_height;
        unsigned char* buf = (unsigned char*)malloc(size);
        if (!buf) {
            g_context->lpVtbl->Unmap(g_context, (ID3D11Resource*)g_staging, 0);
            result.status = STATUS_ERROR;
            return result;
        }
        memcpy(buf, mapped.pData, size);
        g_context->lpVtbl->Unmap(g_context, (ID3D11Resource*)g_staging, 0);

        result.data = buf;
        result.width = g_width;
        result.height = g_height;
        result.stride = stride;
        result.status = STATUS_OK;
        return result;
    }

    /* GDI fallback — BitBlt from screen DC */
    HDC hdc = GetDC(NULL);
    if (!hdc) {
        result.status = STATUS_ERROR;
        return result;
    }
    int w = GetSystemMetrics(SM_CXSCREEN);
    int h = GetSystemMetrics(SM_CYSCREEN);
    int stride = w * 4;
    int size = stride * h;
    unsigned char* buf = (unsigned char*)malloc(size);
    if (!buf) {
        ReleaseDC(NULL, hdc);
        result.status = STATUS_ERROR;
        return result;
    }
    /* Use GetDiBits to get the actualbits with correct stride */
    BITMAPINFO bmi = {0};
    bmi.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
    bmi.bmiHeader.biWidth = w;
    bmi.bmiHeader.biHeight = -h; /* top-down */
    bmi.bmiHeader.biPlanes = 1;
    bmi.bmiHeader.biBitCount = 32;
    bmi.bmiHeader.biCompression = BI_RGB;

    /* BitBlt into a compatible DC first, then GetDiBits */
    HDC memDC = CreateCompatibleDC(hdc);
    HBITMAP dibBmp = CreateDIBSection(memDC, &bmi, DIB_RGB_COLORS, NULL, NULL, 0);
    if (!dibBmp) {
        free(buf);
        DeleteDC(memDC);
        ReleaseDC(NULL, hdc);
        result.status = STATUS_ERROR;
        return result;
    }
    HBITMAP oldBmp = SelectObject(memDC, dibBmp);
    BitBlt(memDC, 0, 0, w, h, hdc, 0, 0, SRCCOPY);
    GetDIBits(memDC, dibBmp, 0, h, buf, &bmi, DIB_RGB_COLORS);
    SelectObject(memDC, oldBmp);
    DeleteObject(dibBmp);
    DeleteDC(memDC);
    ReleaseDC(NULL, hdc);

    g_width = w;
    g_height = h;

    result.data = buf;
    result.width = w;
    result.height = h;
    result.stride = stride;
    result.status = STATUS_OK;
    return result;
}

void free_frame_win(unsigned char* data) {
    free(data);
}