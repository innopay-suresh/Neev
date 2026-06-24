/*
 * capture_mac.m — macOS screen capture via CGDisplayStream
 *
 * Uses CoreGraphics CGDisplayStream to receive BGRA frames efficiently.
 * On macOS 12.3+, ScreenCaptureKit is preferred but requires additional
 * entitlements; CGDisplayStream works on all supported versions (10.8+).
 *
 * PERMISSION REQUIRED: The app binary must be granted Screen Recording
 * permission in System Preferences → Privacy & Security → Screen Recording.
 *
 * Build flags: -framework CoreGraphics -framework CoreFoundation -framework CoreMedia
 */

#import <CoreGraphics/CoreGraphics.h>
#import <CoreMedia/CoreMedia.h>
#import <Foundation/Foundation.h>
#import <IOSurface/IOSurface.h>
#include "capture_darwin.h"
#include <stdlib.h>
#include <string.h>
#include <pthread.h>

typedef struct MacCaptureState {
    pthread_mutex_t     mutex;
    CGDisplayStreamRef  stream;
    IOSurfaceRef        surface;
    dispatch_queue_t    queue;
    volatile int        has_frame;
    int                 width;
    int                 height;
} MacCaptureState;

MacCaptureState* init_stream_mac(uint32_t display_id) {
    if (display_id == 0) {
        display_id = CGMainDisplayID();
    }
    MacCaptureState* state = malloc(sizeof(MacCaptureState));
    if (!state) return NULL;

    pthread_mutex_init(&state->mutex, NULL);
    state->stream = NULL;
    state->surface = NULL;
    state->has_frame = 0;

    state->queue = dispatch_queue_create("com.remote-agent.capture", NULL);
    
    state->width  = (int)CGDisplayPixelsWide(display_id);
    state->height = (int)CGDisplayPixelsHigh(display_id);

    NSDictionary* props = @{
        (__bridge NSString*)kCGDisplayStreamShowCursor: @YES,
        (__bridge NSString*)kCGDisplayStreamMinimumFrameTime: @(1.0/60.0),
    };

    state->stream = CGDisplayStreamCreateWithDispatchQueue(
        display_id,
        state->width, state->height,
        'BGRA',
        (__bridge CFDictionaryRef)props,
        state->queue,
        ^ (CGDisplayStreamFrameStatus status, uint64_t display_time, IOSurfaceRef frame_surface, CGDisplayStreamUpdateRef update_ref) {
            if (status != kCGDisplayStreamFrameStatusFrameComplete) return;
            
            pthread_mutex_lock(&state->mutex);
            if (state->surface) IOSurfaceDecrementUseCount(state->surface);
            if (frame_surface) IOSurfaceIncrementUseCount(frame_surface);
            state->surface   = frame_surface;
            state->has_frame = 1;
            pthread_mutex_unlock(&state->mutex);
        }
    );

    if (!state->stream) {
        NSLog(@"RemoteAgent: CGDisplayStreamCreateWithDispatchQueue failed (display=%u, width=%d, height=%d)", display_id, state->width, state->height);
        free(state);
        return NULL;
    }
    
    CGError err = CGDisplayStreamStart(state->stream);
    if (err != kCGErrorSuccess) {
        NSLog(@"RemoteAgent: CGDisplayStreamStart failed with error %d (display=%u, width=%d, height=%d)", err, display_id, state->width, state->height);
        stop_stream_mac(state);
        return NULL;
    }
    NSLog(@"RemoteAgent: CGDisplayStream started successfully (display=%u, width=%d, height=%d)", display_id, state->width, state->height);

    return state;
}

int request_screen_capture_access_mac(void) {
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 101500
    if (@available(macOS 10.15, *)) {
        return CGRequestScreenCaptureAccess() ? 1 : 0;
    }
#endif
    return 0;
}

MacCaptureResult capture_frame_mac(MacCaptureState* state) {
    MacCaptureResult result = {0};
    if (!state) {
        result.status = STATUS_ERROR;
        return result;
    }

    pthread_mutex_lock(&state->mutex);

    if (!state->has_frame || !state->surface) {
        pthread_mutex_unlock(&state->mutex);
        result.status = STATUS_NO_NEW_FRAME;
        return result;
    }
    state->has_frame = 0;

    size_t bpr  = IOSurfaceGetBytesPerRow(state->surface);
    size_t h    = IOSurfaceGetHeight(state->surface);
    size_t size = bpr * h;

    IOSurfaceLock(state->surface, kIOSurfaceLockReadOnly, NULL);
    void* base = IOSurfaceGetBaseAddress(state->surface);

    unsigned char* buf = (unsigned char*)malloc(size);
    if (!buf) {
        IOSurfaceUnlock(state->surface, kIOSurfaceLockReadOnly, NULL);
        pthread_mutex_unlock(&state->mutex);
        result.status = STATUS_ERROR;
        return result;
    }
    memcpy(buf, base, size);
    IOSurfaceUnlock(state->surface, kIOSurfaceLockReadOnly, NULL);
    
    result.data         = buf;
    result.width        = (int)IOSurfaceGetWidth(state->surface);
    result.height       = (int)h;
    result.bytes_per_row = (int)bpr;
    result.status       = STATUS_OK;

    pthread_mutex_unlock(&state->mutex);
    return result;
}

void free_frame_mac(unsigned char* data) {
    free(data);
}

void stop_stream_mac(MacCaptureState* state) {
    if (!state) return;
    
    if (state->stream) {
        CGDisplayStreamStop(state->stream);
    }
    
    // Barrier: wait for any currently executing callbacks to finish
    if (state->queue) {
        dispatch_sync(state->queue, ^{
            // Do nothing
        });
    }
    
    if (state->surface) {
        IOSurfaceDecrementUseCount(state->surface);
        state->surface = NULL;
    }
    if (state->stream) {
        CFRelease(state->stream);
        state->stream = NULL;
    }
    if (state->queue) {
        dispatch_release(state->queue);
        state->queue = NULL;
    }
    
    pthread_mutex_destroy(&state->mutex);
    free(state);
}

MacDisplayList get_active_displays_mac() {
    MacDisplayList list = {0};
    uint32_t count = 0;
    CGGetActiveDisplayList(0, NULL, &count);
    if (count == 0) return list;

    CGDirectDisplayID* dspys = malloc(count * sizeof(CGDirectDisplayID));
    CGGetActiveDisplayList(count, dspys, &count);

    list.displays = malloc(count * sizeof(MacDisplayInfo));
    list.count = count;

    for (int i = 0; i < count; i++) {
        list.displays[i].id = dspys[i];
        list.displays[i].width = (int)CGDisplayPixelsWide(dspys[i]);
        list.displays[i].height = (int)CGDisplayPixelsHigh(dspys[i]);
        list.displays[i].isPrimary = CGDisplayIsMain(dspys[i]) ? 1 : 0;
    }
    free(dspys);
    return list;
}

void free_display_list_mac(MacDisplayList list) {
    if (list.displays) {
        free(list.displays);
    }
}

void get_bounds_mac(MacCaptureState* state, int* width, int* height) {
    if (state) {
        *width = state->width;
        *height = state->height;
    } else {
        *width = 0; *height = 0;
    }
}

void get_cursor_info_mac(MacCursorInfo* info) {
    if (!info) return;
    memset(info, 0, sizeof(MacCursorInfo));

    CGEventRef event = CGEventCreate(NULL);
    if (!event) {
        info->visible = 0;
        return;
    }
    CGPoint pt = CGEventGetLocation(event);
    CFRelease(event);

    CGRect displayBounds = CGDisplayBounds(CGMainDisplayID());

    /* Check if cursor is within the main display bounds */
    if (CGRectContainsPoint(displayBounds, pt)) {
        info->visible = 1;
        info->x = (int)pt.x;
        info->y = (int)(displayBounds.size.height - pt.y); /* flip Y for screen coords */
    } else {
        info->visible = 0;
    }

    /* On macOS, the cursor IS baked into the video frames via kCGDisplayStreamShowCursor.
       The viewer can infer cursor position from our stream, but we also expose it here
       for cases where the viewer wants to render a custom cursor overlay. */
    info->width = 0;
    info->height = 0;
    info->hotX = 0;
    info->hotY = 0;
    info->mask = NULL;
}
