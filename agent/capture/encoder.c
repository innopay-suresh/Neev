//go:build cgo && (darwin || linux)

/*
 * encoder.c — FFmpeg H.264 encoding helper with hardware acceleration
 *
 * Platform-specific hardware acceleration:
 *   - macOS:  VideoToolbox (VT_enc_h264) first, fall back to libx264
 *   - Windows: NVENC first, fall back to libx264
 *   - Linux:   libx264 only
 *
 * Software fallback always uses libx264 with:
 *   - preset=ultrafast
 *   - tune=zerolatency
 *
 * Output format: Annex B NAL units (00 00 00 01 start codes)
 *
 * Build (macOS homebrew):
 *   pkg-config --cflags --libs libavcodec libavutil libswscale libx264
 * Build (Linux):
 *   pkg-config --cflags --libs libavcodec libavutil libswscale libx264
 * Build (Windows vcpkg):
 *   -lavcodec -lavutil -lswscale -lx264
 */

#include "encoder.h"
#include <libavcodec/avcodec.h>
#include <libavutil/imgutils.h>
#include <libswscale/swscale.h>
#include <libavutil/opt.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>

/* Detect platform at compile time via preprocessor */
#if defined(__APPLE__)
    #include <TargetConditionals.h>
    #define PLATFORM_MAC    1
    #define PLATFORM_WINDOWS 0
    #define PLATFORM_LINUX  0
#elif defined(_WIN32)
    #define PLATFORM_MAC    0
    #define PLATFORM_WINDOWS 1
    #define PLATFORM_LINUX  0
#else
    #define PLATFORM_MAC    0
    #define PLATFORM_WINDOWS 0
    #define PLATFORM_LINUX  1
#endif

struct H264Encoder {
    AVCodecContext*  ctx;
    const AVCodec*   codec;
    AVFrame*         frame;
    struct SwsContext* sws_ctx;
    int              width;
    int              height;
    int              fps;
    int              bitrate_kbps;
    int              hw_enabled;
    int              frame_count;
    /* Reusable packet to avoid repeated allocation */
    AVPacket*        pkt;
};

static const char* get_hw_codec_name(int hw_enabled) {
    if (!hw_enabled) return "libx264";

#if PLATFORM_MAC
    if (hw_enabled) return "videotoolbox";
#endif

#if PLATFORM_WINDOWS
    if (hw_enabled) return "nvenc";
#endif

    return "libx264";
}

static const AVCodec* find_encoder(int hw_enabled) {
    if (!hw_enabled) {
        /* Software: libx264 */
        const AVCodec* c = avcodec_find_encoder_by_name("libx264");
        if (!c) c = avcodec_find_encoder(AV_CODEC_ID_H264);
        return c;
    }

#if PLATFORM_MAC
    /* macOS: try VideoToolbox first */
    const AVCodec* c = avcodec_find_encoder_by_name("videotoolbox");
    if (c) return c;
    fprintf(stderr, "[H264] VideoToolbox not available, falling back to libx264\n");
    return avcodec_find_encoder_by_name("libx264");
#endif

#if PLATFORM_WINDOWS
    /* Windows: try NVENC first */
    const AVCodec* c = avcodec_find_encoder_by_name("nvenc_h264");
    if (c) return c;
    c = avcodec_find_encoder_by_name("nvenc");
    if (c) return c;
    fprintf(stderr, "[H264] NVENC not available, falling back to libx264\n");
    return avcodec_find_encoder_by_name("libx264");
#endif

    /* Linux or unknown: libx264 */
    return avcodec_find_encoder_by_name("libx264");
}

static int setup_hw_accel(AVCodecContext* ctx, int hw_enabled) {
    if (!hw_enabled) return 0;

#if PLATFORM_MAC
    /* VideoToolbox: use hardware device type */
    #if LIBAVUTIL_VERSION_MAJOR >= 58
        int ret = av_hwdevice_ctx_create(&ctx->hw_device_ctx, AV_HWDEVICE_TYPE_VIDEOTOOLBOX, NULL, NULL, 0);
        if (ret < 0) {
            fprintf(stderr, "[H264] Failed to create VideoToolbox device: %d\n", ret);
            return -1;
        }
        ctx->hw_frames_ctx = av_hwdevice_ctx_alloc(AV_HWDEVICE_TYPE_VIDEOTOOLBOX);
        if (!ctx->hw_frames_ctx) {
            fprintf(stderr, "[H264] Failed to allocate VideoToolbox frames context\n");
            return -1;
        }
        /* Frame context will be configured later when we know dimensions */
    #endif
    return 0;
#endif

    return 0;
}

H264Encoder* h264_encoder_create(int width, int height, int fps, int bitrate_kbps, int hw_enabled) {
    /* Ensure even dimensions (H.264 requires them) */
    if (width % 2 != 0) width++;
    if (height % 2 != 0) height++;

    H264Encoder* enc = (H264Encoder*)calloc(1, sizeof(H264Encoder));
    if (!enc) return NULL;

    enc->width = width;
    enc->height = height;
    enc->fps = fps;
    enc->bitrate_kbps = bitrate_kbps;
    enc->hw_enabled = hw_enabled;
    enc->frame_count = 0;

    /* Find codec */
    enc->codec = find_encoder(hw_enabled);
    if (!enc->codec) {
        fprintf(stderr, "[H264] H.264 encoder not found\n");
        free(enc);
        return NULL;
    }

    /* Create context */
    enc->ctx = avcodec_alloc_context3(enc->codec);
    if (!enc->ctx) {
        fprintf(stderr, "[H264] Failed to allocate codec context\n");
        free(enc);
        return NULL;
    }

    /* Configure context */
    enc->ctx->width = enc->width;
    enc->ctx->height = enc->height;
    enc->ctx->time_base = (AVRational){1, fps};
    enc->ctx->framerate = (AVRational){fps, 1};
    enc->ctx->gop_size = fps * 2;          /* keyframe every 2 seconds */
    enc->ctx->max_b_frames = 0;            /* no B-frames for low latency */
    enc->ctx->pix_fmt = AV_PIX_FMT_YUV420P;
    enc->ctx->bit_rate = bitrate_kbps * 1000;
    enc->ctx->rc_buffer_size = bitrate_kbps * 1000;
    enc->ctx->rc_max_rate = bitrate_kbps * 1000;
    enc->ctx->rc_min_rate = bitrate_kbps * 1000;
    enc->ctx->rc_initial_buffer_occupancy = enc->ctx->rc_buffer_size * 3 / 4;

    /* Software encoding: configure libx264 options */
    if (!hw_enabled || strcmp(enc->codec->name, "libx264") == 0) {
        /* Use av_opt_set to set x264 params */
        char preset[64] = "ultrafast";
        char tune[64] = "zerolatency";

        /* Try to set x264 preset/tune */
        AVDictionary* opts = NULL;
        av_dict_set(&opts, "preset", preset, 0);
        av_dict_set(&opts, "tune", tune, 0);

        /* Try preset=ultrafast for real-time */
        av_opt_set(enc->ctx->priv_data, "preset", "ultrafast", 0);
        av_opt_set(enc->ctx->priv_data, "tune", "zerolatency", 0);
        /* Speed optimizations */
        av_opt_set(enc->ctx->priv_data, "profile", "baseline", 0);
        av_dict_free(&opts);
    }

    /* Hardware acceleration setup */
    if (hw_enabled) {
        #if LIBAVUTIL_VERSION_MAJOR >= 58
            /* Try to create hardware device context */
            if (strcmp(enc->codec->name, "videotoolbox") == 0 ||
                strcmp(enc->codec->name, "nvenc") == 0 ||
                strcmp(enc->codec->name, "nvenc_h264") == 0) {

                enum AVHWDeviceType hw_type = AV_HWDEVICE_TYPE_NONE;
                #if PLATFORM_MAC
                    hw_type = AV_HWDEVICE_TYPE_VIDEOTOOLBOX;
                #elif PLATFORM_WINDOWS
                    hw_type = AV_HWDEVICE_TYPE_CUDA;
                #endif

                if (hw_type != AV_HWDEVICE_TYPE_NONE) {
                    int ret = av_hwdevice_ctx_create(&enc->ctx->hw_device_ctx, hw_type, NULL, NULL, 0);
                    if (ret < 0) {
                        fprintf(stderr, "[H264] Failed to create HW device: %d\n", ret);
                    } else {
                        fprintf(stderr, "[H264] Hardware acceleration enabled with %s\n", enc->codec->name);
                    }
                }
            }
        #endif
    }

    /* Open codec */
    AVDictionary* opts = NULL;
    int ret = avcodec_open2(enc->ctx, enc->codec, &opts);
    av_dict_free(&opts);

    if (ret < 0) {
        fprintf(stderr, "[H264] Failed to open codec: %d\n", ret);
        avcodec_free_context(&enc->ctx);
        free(enc);
        return NULL;
    }

    /* Create input frame for RGBA -> YUV conversion */
    enc->frame = av_frame_alloc();
    if (!enc->frame) {
        fprintf(stderr, "[H264] Failed to allocate frame\n");
        avcodec_free_context(&enc->ctx);
        free(enc);
        return NULL;
    }
    enc->frame->format = AV_PIX_FMT_YUV420P;
    enc->frame->width = enc->width;
    enc->frame->height = enc->height;

    ret = av_image_alloc(enc->frame->data, enc->frame->linesize, enc->width, enc->height, AV_PIX_FMT_YUV420P, 1);
    if (ret < 0) {
        fprintf(stderr, "[H264] Failed to allocate frame buffer\n");
        av_frame_free(&enc->frame);
        avcodec_free_context(&enc->ctx);
        free(enc);
        return NULL;
    }

    /* Create SWScale context for BGRA -> YUV420P conversion */
    enc->sws_ctx = sws_getContext(
        enc->width, enc->height, AV_PIX_FMT_BGRA,
        enc->width, enc->height, AV_PIX_FMT_YUV420P,
        SWS_FAST_BILINEAR, NULL, NULL, NULL
    );
    if (!enc->sws_ctx) {
        fprintf(stderr, "[H264] Failed to create SWScale context\n");
        av_freep(&enc->frame->data[0]);
        av_frame_free(&enc->frame);
        avcodec_free_context(&enc->ctx);
        free(enc);
        return NULL;
    }

    /* Create reusable packet */
    enc->pkt = av_packet_alloc();
    if (!enc->pkt) {
        fprintf(stderr, "[H264] Failed to allocate packet\n");
        sws_freeContext(enc->sws_ctx);
        av_freep(&enc->frame->data[0]);
        av_frame_free(&enc->frame);
        avcodec_free_context(&enc->ctx);
        free(enc);
        return NULL;
    }

    fprintf(stderr, "[H264] Encoder created: %dx%d @ %d fps, bitrate=%d kbps, hw=%s\n",
            enc->width, enc->height, enc->fps, enc->bitrate_kbps,
            hw_enabled ? "YES" : "NO");

    return enc;
}

int h264_encode_frame(H264Encoder* enc,
                      const unsigned char* bgra,
                      int force_keyframe,
                      H264EncodeResult* out) {
    if (!enc || !bgra || !out) return -1;

    memset(out, 0, sizeof(*out));

    /* Step 1: Convert BGRA -> YUV420P using SWScale */
    const uint8_t* srcSlice[] = { bgra };
    int srcStride = enc->width * 4; /* BGRA = 4 bytes/pixel */

    /* Setup source plane for RGBA (stored as BGRA in memory) */
    int srcStep = 4; /* stride for BGRA */
    sws_scale(enc->sws_ctx,
              (const uint8_t* const*)&bgra, &srcStep,
              0, enc->height,
              enc->frame->data, enc->frame->linesize);

    enc->frame->pts = enc->frame_count++;

    /* Force IDR frame if requested */
    if (force_keyframe) {
        enc->frame->pict_type = AV_PICTURE_TYPE_I;
        enc->frame->flags |= AV_FRAME_FLAG_KEY;
    } else {
        enc->frame->pict_type = AV_PICTURE_TYPE_NONE;
        enc->frame->flags &= ~AV_FRAME_FLAG_KEY;
    }

    /* Step 2: Send frame to encoder */
    int ret = avcodec_send_frame(enc->ctx, enc->frame);
    if (ret < 0) {
        snprintf(out->error_msg, sizeof(out->error_msg),
                 "avcodec_send_frame failed: %d", ret);
        return -1;
    }

    /* Step 3: Receive encoded packet */
    ret = avcodec_receive_packet(enc->ctx, enc->pkt);
    if (ret == AVERROR(EAGAIN)) {
        return 1; /* Encoder buffering, need more frames */
    }
    if (ret < 0) {
        snprintf(out->error_msg, sizeof(out->error_msg),
                 "avcodec_receive_packet failed: %d", ret);
        return -1;
    }

    /* Step 4: Convert to Annex B format (add start codes) */
    /* FFmpeg outputs MP4 format by default; convert to Annex B */
    int nal_count = 0;
    int total_size = 0;

    /* Count NAL units and calculate total size */
    uint8_t* p = enc->pkt->data;
    int remaining = enc->pkt->size;

    /* Find NAL units (start code: 00 00 00 01 or 00 00 01) */
    while (remaining > 0) {
        int skip = 0;
        /* Find start code */
        if (remaining >= 4 && p[0] == 0 && p[1] == 0 && p[2] == 0 && p[3] == 1) {
            skip = 4;
        } else if (remaining >= 3 && p[0] == 0 && p[1] == 0 && p[2] == 1) {
            skip = 3;
        } else {
            /* No start code found, this is first NAL - assume it has none */
            /* The first NAL might be SPS/PPS without start code */
            skip = 0;
            /* If we have length prefix mode (MP4), convert to start codes */
            /* Check if first 4 bytes are length prefix (big endian) */
            int len = (p[0] << 24) | (p[1] << 16) | (p[2] << 8) | p[3];
            if (len > 0 && len < remaining - 4) {
                /* Convert to Annex B */
                uint8_t* new_buf = (uint8_t*)malloc(remaining + 16); /* extra space for start codes */
                if (!new_buf) {
                    av_packet_unref(enc->pkt);
                    return -1;
                }

                uint8_t* dst = new_buf;
                int offset = 4;
                int count = 0;

                while (offset < remaining && count < 100) { /* safety limit */
                    int nal_len = (p[offset-4] << 24) | (p[offset-3] << 16) |
                                  (p[offset-2] << 8) | p[offset-1];
                    if (nal_len <= 0 || nal_len > remaining - offset + 4) break;

                    /* Add start code */
                    dst[0] = 0; dst[1] = 0; dst[2] = 0; dst[3] = 1;
                    dst += 4;

                    /* Copy NAL unit */
                    memcpy(dst, p + offset, nal_len);
                    dst += nal_len;
                    offset += nal_len + 4;
                    count++;
                }

                out->data = new_buf;
                out->size = (int)(dst - new_buf);
                out->is_keyframe = (enc->pkt->flags & AV_PKT_FLAG_KEY) ? 1 : 0;

                av_packet_unref(enc->pkt);
                return 0;
            }
            break;
        }

        p += skip;
        remaining -= skip;
        nal_count++;
    }

    /* If we get here, assume it's already Annex B or single NAL */
    /* Copy the packet data */
    uint8_t* buf = (uint8_t*)malloc(enc->pkt->size);
    if (!buf) {
        av_packet_unref(enc->pkt);
        return -1;
    }
    memcpy(buf, enc->pkt->data, enc->pkt->size);

    out->data = buf;
    out->size = enc->pkt->size;
    out->is_keyframe = (enc->pkt->flags & AV_PKT_FLAG_KEY) ? 1 : 0;

    av_packet_unref(enc->pkt);
    return 0;
}

void h264_free_packet(unsigned char* data) {
    free(data);
}

void h264_encoder_destroy(H264Encoder* enc) {
    if (!enc) return;

    if (enc->pkt) {
        av_packet_free(&enc->pkt);
    }
    if (enc->sws_ctx) {
        sws_freeContext(enc->sws_ctx);
    }
    if (enc->frame) {
        if (enc->frame->data[0]) {
            av_freep(&enc->frame->data[0]);
        }
        av_frame_free(&enc->frame);
    }
    if (enc->ctx) {
        avcodec_free_context(&enc->ctx);
    }
    free(enc);
}

int h264_encoder_set_bitrate(H264Encoder* enc, int bitrate_kbps) {
    if (!enc || !enc->ctx) return -1;

    enc->bitrate_kbps = bitrate_kbps;
    enc->ctx->bit_rate = bitrate_kbps * 1000;
    enc->ctx->rc_max_rate = bitrate_kbps * 1000;
    enc->ctx->rc_min_rate = bitrate_kbps * 1000;

    return 0;
}

int h264_encoder_is_hw_active(H264Encoder* enc) {
    if (!enc) return 0;
    if (!enc->ctx) return 0;
    return (enc->ctx->hw_device_ctx != NULL) ? 1 : 0;
}

/* JPEG encoding using FFmpeg's MJPEG codec */
int jpeg_encode_region(const unsigned char* bgra, int full_width, int full_height,
                       int x, int y, int width, int height, int quality,
                       JPEGEncodeResult* out) {
    if (!bgra || !out || width <= 0 || height <= 0) return -1;
    
    memset(out, 0, sizeof(*out));
    
    /* Validate dimensions are even (MJPEG prefers this) */
    if (width % 2 != 0) width--;
    if (height % 2 != 0) height--;
    if (width <= 0 || height <= 0) return -1;
    
    /* Find MJPEG encoder */
    const AVCodec* codec = avcodec_find_encoder(AV_CODEC_ID_MJPEG);
    if (!codec) {
        snprintf(out->error_msg, sizeof(out->error_msg), "MJPEG encoder not found");
        return -1;
    }
    
    /* Create codec context */
    AVCodecContext* ctx = avcodec_alloc_context3(codec);
    if (!ctx) {
        snprintf(out->error_msg, sizeof(out->error_msg), "Failed to allocate codec context");
        return -1;
    }
    
    ctx->width = width;
    ctx->height = height;
    ctx->pix_fmt = AV_PIX_FMT_YUVJ420P;
    ctx->global_quality = quality;  /* MJPEG quality parameter */
    
    /* Open codec */
    int ret = avcodec_open2(ctx, codec, NULL);
    if (ret < 0) {
        snprintf(out->error_msg, sizeof(out->error_msg), "Failed to open codec: %d", ret);
        avcodec_free_context(&ctx);
        return -1;
    }
    
    /* Create frame */
    AVFrame* frame = av_frame_alloc();
    if (!frame) {
        avcodec_free_context(&ctx);
        return -1;
    }
    frame->format = AV_PIX_FMT_YUVJ420P;
    frame->width = width;
    frame->height = height;
    
    ret = av_image_alloc(frame->data, frame->linesize, width, height, AV_PIX_FMT_YUVJ420P, 1);
    if (ret < 0) {
        av_frame_free(&frame);
        avcodec_free_context(&ctx);
        return -1;
    }
    
    /* Create SWScale context for BGRA -> YUVJ420P conversion */
    struct SwsContext* sws_ctx = sws_getContext(
        width, height, AV_PIX_FMT_BGRA,
        width, height, AV_PIX_FMT_YUVJ420P,
        SWS_FAST_BILINEAR, NULL, NULL, NULL
    );
    if (!sws_ctx) {
        av_freep(&frame->data[0]);
        av_frame_free(&frame);
        avcodec_free_context(&ctx);
        return -1;
    }
    
    /* Extract region from full BGRA buffer */
    /* Calculate row pointers for the region */
    uint8_t* region_data = (uint8_t*)malloc(width * height * 4);
    if (!region_data) {
        sws_freeContext(sws_ctx);
        av_freep(&frame->data[0]);
        av_frame_free(&frame);
        avcodec_free_context(&ctx);
        return -1;
    }
    
    /* Copy region pixels */
    int full_stride = full_width * 4;
    for (int row = 0; row < height; row++) {
        memcpy(region_data + row * width * 4,
               bgra + (y + row) * full_stride + x * 4,
               width * 4);
    }
    
    /* Convert BGRA -> YUVJ420P using SWScale */
    int src_stride = width * 4;
    sws_scale(sws_ctx,
              (const uint8_t* const*)&region_data, &src_stride,
              0, height,
              frame->data, frame->linesize);
    
    free(region_data);
    sws_freeContext(sws_ctx);
    
    frame->pts = 0;
    
    /* Encode */
    AVPacket* pkt = av_packet_alloc();
    if (!pkt) {
        av_freep(&frame->data[0]);
        av_frame_free(&frame);
        avcodec_free_context(&ctx);
        return -1;
    }
    
    ret = avcodec_send_frame(ctx, frame);
    if (ret < 0) {
        snprintf(out->error_msg, sizeof(out->error_msg), "avcodec_send_frame failed: %d", ret);
        av_packet_free(&pkt);
        av_freep(&frame->data[0]);
        av_frame_free(&frame);
        avcodec_free_context(&ctx);
        return -1;
    }
    
    ret = avcodec_receive_packet(ctx, pkt);
    if (ret < 0) {
        snprintf(out->error_msg, sizeof(out->error_msg), "avcodec_receive_packet failed: %d", ret);
        av_packet_free(&pkt);
        av_freep(&frame->data[0]);
        av_frame_free(&frame);
        avcodec_free_context(&ctx);
        return -1;
    }
    
    /* Copy output */
    uint8_t* buf = (uint8_t*)malloc(pkt->size);
    if (!buf) {
        av_packet_free(&pkt);
        av_freep(&frame->data[0]);
        av_frame_free(&frame);
        avcodec_free_context(&ctx);
        return -1;
    }
    memcpy(buf, pkt->data, pkt->size);
    
    out->data = buf;
    out->size = pkt->size;
    
    av_packet_free(&pkt);
    av_freep(&frame->data[0]);
    av_frame_free(&frame);
    avcodec_free_context(&ctx);
    
    return 0;
}

void jpeg_free_packet(unsigned char* data) {
    free(data);
}