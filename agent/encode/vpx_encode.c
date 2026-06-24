/*
 * vpx_encode.c — libvpx VP8 encoding helper
 *
 * Wraps the vpx_codec_encode API into a simple frame-in/packet-out model.
 * The encoder context is heap-allocated so Go can hold an opaque pointer.
 *
 * Build: link with -lvpx  (brew install libvpx / apt install libvpx-dev)
 */

#include "vpx_encode.h"
#include <vpx/vpx_encoder.h>
#include <vpx/vp8cx.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

struct VpxEncoder {
    vpx_codec_ctx_t    codec;
    vpx_image_t        img;
    vpx_codec_enc_cfg_t cfg;
    int                width;
    int                height;
    int                frame_count;
    /* Output packet buffer — caller frees via vpx_free_packets(). */
    unsigned char*     pkt_data;
    size_t             pkt_size;
};

VpxEncoder* vpx_encoder_create(int width, int height, int fps, int bitrate_kbps) {
    VpxEncoder* enc = (VpxEncoder*)calloc(1, sizeof(VpxEncoder));
    if (!enc) return NULL;

    enc->width  = width;
    enc->height = height;

    vpx_codec_enc_config_default(vpx_codec_vp8_cx(), &enc->cfg, 0);

    enc->cfg.g_w             = width;
    enc->cfg.g_h             = height;
    enc->cfg.g_timebase.num  = 1;
    enc->cfg.g_timebase.den  = fps;
    enc->cfg.rc_target_bitrate = bitrate_kbps;
    enc->cfg.g_threads       = 4;
    enc->cfg.g_error_resilient = VPX_ERROR_RESILIENT_DEFAULT;
    /* Real-time mode: deadline = 1 (fastest). */
    enc->cfg.g_pass          = VPX_RC_ONE_PASS;
    enc->cfg.rc_end_usage    = VPX_CBR;
    enc->cfg.kf_mode         = VPX_KF_AUTO;
    enc->cfg.kf_max_dist     = fps * 2; /* keyframe at most every 2 seconds */
    enc->cfg.rc_min_quantizer = 10;
    enc->cfg.rc_max_quantizer = 50;

    if (vpx_codec_enc_init(&enc->codec, vpx_codec_vp8_cx(), &enc->cfg, 0) != VPX_CODEC_OK) {
        free(enc);
        return NULL;
    }

    /* Speed: token partitions, cpu-used (0=quality … 16=fastest).
       Lower value = better quality, slower encoding.
       For remote desktop: 2 gives noticeably better quality with minimal latency impact. */
    vpx_codec_control(&enc->codec, VP8E_SET_CPUUSED, 2);
    vpx_codec_control(&enc->codec, VP8E_SET_STATIC_THRESHOLD, 1);

    if (!vpx_img_alloc(&enc->img, VPX_IMG_FMT_I420, width, height, 32)) {
        vpx_codec_destroy(&enc->codec);
        free(enc);
        return NULL;
    }

    return enc;
}

/*
 * vpx_encode_frame — encodes one RGBA frame.
 *
 * Input:  rgba_data (width*height*4 bytes, row-major)
 * Output: fills enc->pkt_data / enc->pkt_size with compressed VP8 data.
 *         Returns 0 on success, -1 on error, 1 if no output (encoder buffering).
 */
int vpx_encode_frame(VpxEncoder* enc,
                     const unsigned char* rgba,
                     int force_keyframe,
                     EncodeResult* out) {
    int w = enc->width, h = enc->height;

    /* Convert RGBA → I420 (YUV 4:2:0) */
    unsigned char* Y  = enc->img.planes[0];
    unsigned char* Cb = enc->img.planes[1];
    unsigned char* Cr = enc->img.planes[2];
    int strideY  = enc->img.stride[0];
    int strideCb = enc->img.stride[1];
    int strideCr = enc->img.stride[2];

    for (int row = 0; row < h; row++) {
        for (int col = 0; col < w; col++) {
            int idx = (row * w + col) * 4;
            unsigned char r = rgba[idx+0];
            unsigned char g = rgba[idx+1];
            unsigned char b = rgba[idx+2];

            /* BT.601 full-range conversion */
            int yv = (( 66*r + 129*g +  25*b + 128) >> 8) + 16;
            Y[row * strideY + col] = (unsigned char)(yv < 0 ? 0 : yv > 255 ? 255 : yv);

            if ((row & 1) == 0 && (col & 1) == 0) {
                int cb = ((-38*r -  74*g + 112*b + 128) >> 8) + 128;
                int cr = ((112*r -  94*g -  18*b + 128) >> 8) + 128;
                Cb[(row/2) * strideCb + col/2] = (unsigned char)(cb < 0 ? 0 : cb > 255 ? 255 : cb);
                Cr[(row/2) * strideCr + col/2] = (unsigned char)(cr < 0 ? 0 : cr > 255 ? 255 : cr);
            }
        }
    }

    vpx_enc_frame_flags_t flags = force_keyframe ? VPX_EFLAG_FORCE_KF : 0;
    vpx_codec_pts_t pts = enc->frame_count++;

    /* Deadline: VPX_DL_GOOD_QUALITY (2) balances speed and quality for real-time.
       VPX_DL_REALTIME (1) = fastest but lowest quality. */
    if (vpx_codec_encode(&enc->codec, &enc->img, pts, 1, flags, VPX_DL_GOOD_QUALITY)
        != VPX_CODEC_OK) {
        snprintf(out->error_msg, sizeof(out->error_msg), "%s - %s", vpx_codec_error(&enc->codec), vpx_codec_error_detail(&enc->codec) ? vpx_codec_error_detail(&enc->codec) : "");
        return -1;
    }

    /* Collect output packets. */
    const vpx_codec_cx_pkt_t* pkt;
    vpx_codec_iter_t iter = NULL;
    out->data = NULL;
    out->size = 0;
    out->is_keyframe = 0;

    while ((pkt = vpx_codec_get_cx_data(&enc->codec, &iter)) != NULL) {
        if (pkt->kind == VPX_CODEC_CX_FRAME_PKT) {
            size_t sz = pkt->data.frame.sz;
            unsigned char* buf = (unsigned char*)malloc(sz);
            if (!buf) return -1;
            memcpy(buf, pkt->data.frame.buf, sz);
            out->data = buf;
            out->size = (int)sz;
            out->is_keyframe = (pkt->data.frame.flags & VPX_FRAME_IS_KEY) ? 1 : 0;
            return 0;
        }
    }
    return 1; /* No packet yet (encoder is buffering). */
}

void vpx_free_packet(unsigned char* data) {
    free(data);
}

void vpx_encoder_destroy(VpxEncoder* enc) {
    if (!enc) return;
    vpx_img_free(&enc->img);
    vpx_codec_destroy(&enc->codec);
    free(enc);
}

/* Update bitrate on the fly (for adaptive bitrate control). */
int vpx_encoder_set_bitrate(VpxEncoder* enc, int bitrate_kbps) {
    enc->cfg.rc_target_bitrate = bitrate_kbps;
    return vpx_codec_enc_config_set(&enc->codec, &enc->cfg) == VPX_CODEC_OK ? 0 : -1;
}
