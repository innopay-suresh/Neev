#pragma once
#include <stdint.h>

#define STATUS_OK           0
#define STATUS_NO_NEW_FRAME 1
#define STATUS_ERROR        2

typedef struct {
    unsigned char* data;
    int width;
    int height;
    int bytes_per_row;
    int status;
} MacCaptureResult;

typedef struct MacCaptureState MacCaptureState;

typedef struct {
    uint32_t id;
    int width;
    int height;
    int isPrimary;
} MacDisplayInfo;

// CursorInfo for macOS
typedef struct {
    int x;
    int y;
    int visible;
    int width;
    int height;
    int hotX;
    int hotY;
    int cursorType;   // 0=arrow, 1=ibeam, 2=cross, 3=wait, 4=resize, 5=hand
    void* mask; // not used on macOS, cursor is baked into frames
} MacCursorInfo;

typedef struct {
    MacDisplayInfo* displays;
    int count;
} MacDisplayList;

MacCaptureState* init_stream_mac(uint32_t display_id);
int              request_screen_capture_access_mac(void);
MacCaptureResult capture_frame_mac(MacCaptureState* state);
void             free_frame_mac(unsigned char* data);
void             stop_stream_mac(MacCaptureState* state);
MacDisplayList   get_active_displays_mac();
void             free_display_list_mac(MacDisplayList list);
void             get_bounds_mac(MacCaptureState* state, int* width, int* height);
void             get_cursor_info_mac(MacCursorInfo* info);
