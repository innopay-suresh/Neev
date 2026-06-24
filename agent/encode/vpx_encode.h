#pragma once
#include <stddef.h>

typedef struct VpxEncoder VpxEncoder;

typedef struct {
    unsigned char* data;
    int            size;
    int            is_keyframe;
    char           error_msg[256];
} EncodeResult;

VpxEncoder* vpx_encoder_create(int width, int height, int fps, int bitrate_kbps);
int         vpx_encode_frame(VpxEncoder* enc,
                              const unsigned char* rgba,
                              int force_keyframe,
                              EncodeResult* out);
void        vpx_free_packet(unsigned char* data);
void        vpx_encoder_destroy(VpxEncoder* enc);
int         vpx_encoder_set_bitrate(VpxEncoder* enc, int bitrate_kbps);
