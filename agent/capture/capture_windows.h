#pragma once
#include <windows.h>
#include <d3d11.h>
#include <dxgi1_2.h>

#define STATUS_OK           0
#define STATUS_NO_NEW_FRAME 1
#define STATUS_ERROR        2
#define STATUS_ACCESS_DENIED 3

typedef struct {
    unsigned char* data;
    int width;
    int height;
    int stride;
    int status;
    int hr; // HRESULT error code
} CaptureResult;

// WinDisplayInfo — describes one physical display/monitor on Windows.
typedef struct {
    int width;
    int height;
    int isPrimary;
    void* hMonitor; // HMONITOR handle — not exposed to Go
} WinDisplayInfo;

typedef struct {
    WinDisplayInfo* displays;
    int count;
} WinDisplayList;

// CursorInfo — returned alongside each frame so the viewer can render
// the system cursor as an overlay at the correct position.
typedef struct {
    int x;            // cursor X position (screen pixels)
    int y;            // cursor Y position (screen pixels)
    int visible;      // 1 = cursor is visible, 0 = hidden
    int width;        // cursor bitmap width (0 if no cursor)
    int height;       // cursor bitmap height
    int hotX;         // hot-spot X offset within bitmap
    int hotY;         // hot-spot Y offset within bitmap
    int cursorType;   // 0=arrow, 1=ibeam, 2=cross, 3=wait, 4=resize, 5=hand
    unsigned char* mask; // 32-bit BGRA cursor bitmap (width*height*4), free with FreeCursorMask
} CursorInfo;

CaptureResult capture_frame_win(void);
void          free_frame_win(unsigned char* data);
void          free_cursor_mask(unsigned char* mask);
void          get_cursor_info(CursorInfo* info);
WinDisplayList get_active_displays_win(void);
void           free_display_list_win(WinDisplayList list);
void           set_log_callback(void (*fn)(const char*)); // bridge to Go zerolog