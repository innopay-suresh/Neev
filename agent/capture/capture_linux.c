/*
 * capture_x11.c — Linux screen capture via X11 MIT-SHM extension
 *
 * XShm (MIT Shared Memory Extension) lets the X server write frame data
 * directly into a shared memory segment we map, avoiding an extra copy
 * through the network socket (even on local display).
 *
 * Falls back to XGetImage if XShm is unavailable (remote X11).
 *
 * Build flags: -lX11 -lXext
 */

#include "capture_linux.h"
#include <X11/Xlib.h>
#include <X11/extensions/XShm.h>
#include <sys/ipc.h>
#include <sys/shm.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

/* Module-level X11 state */
static Display*        g_dpy      = NULL;
static XShmSegmentInfo g_shm      = {0};
static XImage*         g_image    = NULL;
static int             g_width    = 0;
static int             g_height   = 0;
static int             g_has_shm  = 0;
static int             g_initialized = 0;

/* Open display — called from Go via open_display_x11(). */
Display* open_display_x11(void) {
    if (!g_dpy) {
        g_dpy = XOpenDisplay(NULL);
    }
    return g_dpy;
}

static int init_shm(void) {
    if (!g_dpy) return 0;

    Screen* screen = DefaultScreenOfDisplay(g_dpy);
    g_width  = WidthOfScreen(screen);
    g_height = HeightOfScreen(screen);

    /* Check if XShm extension is available. */
    int event_base, error_base;
    if (!XShmQueryExtension(g_dpy)) {
        g_has_shm = 0;
        return 1; /* Will use fallback XGetImage */
    }
    g_has_shm = 1;

    /* Create shared XImage. */
    g_image = XShmCreateImage(
        g_dpy,
        DefaultVisual(g_dpy, DefaultScreen(g_dpy)),
        DefaultDepth(g_dpy, DefaultScreen(g_dpy)),
        ZPixmap,
        NULL,
        &g_shm,
        g_width, g_height
    );
    if (!g_image) return 0;

    /* Allocate shared memory. */
    g_shm.shmid = shmget(IPC_PRIVATE,
                          g_image->bytes_per_line * g_image->height,
                          IPC_CREAT | 0600);
    if (g_shm.shmid == -1) { XDestroyImage(g_image); return 0; }

    g_shm.shmaddr = shmat(g_shm.shmid, NULL, 0);
    g_shm.readOnly = False;
    g_image->data = g_shm.shmaddr;

    XShmAttach(g_dpy, &g_shm);
    /* Mark for deletion — will be removed when last process detaches. */
    shmctl(g_shm.shmid, IPC_RMID, NULL);
    return 1;
}

LinuxFrame capture_frame_x11(Display* dpy) {
    LinuxFrame result = {0};

    if (!g_initialized) {
        if (!init_shm()) {
            result.status = STATUS_ERROR;
            return result;
        }
        g_initialized = 1;
    }

    Window root = DefaultRootWindow(dpy);

    if (g_has_shm) {
        if (!XShmGetImage(dpy, root, g_image, 0, 0, AllPlanes)) {
            result.status = STATUS_ERROR;
            return result;
        }
        /* Copy from shared memory to heap (Go will free). */
        int size = g_image->bytes_per_line * g_image->height;
        unsigned char* buf = (unsigned char*)malloc(size);
        if (!buf) { result.status = STATUS_ERROR; return result; }
        memcpy(buf, g_image->data, size);

        result.data          = buf;
        result.width         = g_image->width;
        result.height        = g_image->height;
        result.bytes_per_line = g_image->bytes_per_line;
        result.status        = STATUS_OK;
    } else {
        /* Fallback: XGetImage (slow but universal). */
        XImage* img = XGetImage(dpy, root, 0, 0, g_width, g_height,
                                 AllPlanes, ZPixmap);
        if (!img) { result.status = STATUS_ERROR; return result; }
        int size = img->bytes_per_line * img->height;
        unsigned char* buf = (unsigned char*)malloc(size);
        if (!buf) { XDestroyImage(img); result.status = STATUS_ERROR; return result; }
        memcpy(buf, img->data, size);
        result.data          = buf;
        result.width         = img->width;
        result.height        = img->height;
        result.bytes_per_line = img->bytes_per_line;
        result.status        = STATUS_OK;
        XDestroyImage(img);
    }
    return result;
}

void free_linux_frame(unsigned char* data) {
    free(data);
}

void close_display_x11(Display* dpy) {
    if (g_has_shm && g_image) {
        XShmDetach(dpy, &g_shm);
        shmdt(g_shm.shmaddr);
        XDestroyImage(g_image);
        g_image = NULL;
    }
    XCloseDisplay(dpy);
    g_dpy = NULL;
}

void get_bounds_x11(Display* dpy, int* width, int* height) {
    if (!dpy) {
        *width = 0; *height = 0;
        return;
    }
    Screen* screen = DefaultScreenOfDisplay(dpy);
    *width = WidthOfScreen(screen);
    *height = HeightOfScreen(screen);
}

void get_xcursor_info(Display* dpy, XCursorInfo* info) {
    if (!dpy || !info) return;
    memset(info, 0, sizeof(*info));

    Window root = DefaultRootWindow(dpy);
    Window child;
    int rootX, rootY, winX, winY;
    unsigned int mask;
    Status st = XQueryPointer(dpy, root, &child, &child, &rootX, &rootY, &winX, &winY, &mask);
    if (!st) {
        info->visible = 0;
        return;
    }
    info->visible = 1;
    info->x = rootX;
    info->y = rootY;
    info->width = 0;
    info->height = 0;
    info->hotX = 0;
    info->hotY = 0;
    info->mask = NULL;
}
