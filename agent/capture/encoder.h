#pragma once
#include <stdint.h>
#include <stddef.h>

/* H.264 encoder result structure */
typedef struct {
    unsigned char* data;
    int            size;
    int            is_keyframe;
    char           error_msg[256];
} H264EncodeResult;

/* Opaque H.264 encoder context */
typedef struct H264Encoder H264Encoder;

/* H264Encoder flags */
#define HW_DISABLED  0
#define HW_ENABLED   1

/* H264Encoder creation/destruction */
H264Encoder* h264_encoder_create(int width, int height, int fps, int bitrate_kbps, int hw_enabled);
void         h264_encoder_destroy(H264Encoder* enc);

/* Encode a BGRA frame. Returns 0 on success, -1 on error, 1 if buffering.
 * Output is Annex B format NAL units (00 00 00 01 ...). */
int h264_encode_frame(H264Encoder* enc,
                      const unsigned char* bgra,
                      int force_keyframe,
                      H264EncodeResult* out);

/* Free an encoded packet (call after consuming the output). */
void h264_free_packet(unsigned char* data);

/* Update bitrate on the fly (for adaptive bitrate control). */
int h264_encoder_set_bitrate(H264Encoder* enc, int bitrate_kbps);

/* Check if hardware acceleration is active */
int h264_encoder_is_hw_active(H264Encoder* enc);

/* JPEG encoder result structure */
typedef struct {
    unsigned char* data;
    int            size;
    char           error_msg[256];
} JPEGEncodeResult;

/* Encode a BGRA region as JPEG. Returns 0 on success, -1 on error.
 * x, y, width, height specify the region within the full frame.
 * quality is 1-100 (typical use: 70-85). */
int jpeg_encode_region(const unsigned char* bgra, int full_width, int full_height,
                       int x, int y, int width, int height, int quality,
                       JPEGEncodeResult* out);

/* Free an encoded JPEG packet (call after consuming the output). */
void jpeg_free_packet(unsigned char* data);