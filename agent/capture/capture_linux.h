#pragma once
#include <X11/Xlib.h>

#define STATUS_OK           0
#define STATUS_NO_NEW_FRAME 1
#define STATUS_ERROR        2

typedef struct {
    unsigned char* data;
    int width;
    int height;
    int bytes_per_line;
    int status;
} LinuxFrame;

Display* open_display_x11(void);
LinuxFrame capture_frame_x11(Display* dpy);
void free_linux_frame(unsigned char* data);
void close_display_x11(Display* dpy);
void get_bounds_x11(Display* dpy, int* width, int* height);

typedef struct {
    int x;
    int y;
    int visible;
    int width;
    int height;
    int hotX;
    int hotY;
    void* mask; // not used on Linux
} XCursorInfo;

void get_xcursor_info(Display* dpy, XCursorInfo* info);
