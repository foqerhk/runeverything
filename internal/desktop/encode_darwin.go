//go:build darwin

package desktop

/*
#cgo LDFLAGS: -framework VideoToolbox -framework CoreMedia -framework CoreVideo -framework CoreFoundation
#include <VideoToolbox/VideoToolbox.h>
#include <CoreMedia/CoreMedia.h>
#include <CoreVideo/CoreVideo.h>
#include <stdlib.h>
#include <string.h>
#include <pthread.h>

typedef struct {
	unsigned char *data;
	size_t len;
	int keyframe;
	int hevc; // set by owner before encode; callback reads it
	pthread_mutex_t mu;
	pthread_cond_t cv;
	int ready;
	int closed;
} re_vt_out;

static void re_vt_append_startcode(unsigned char *buf, size_t *off, const uint8_t *nal, size_t nalLen) {
	if (!nal || nalLen == 0) return;
	buf[(*off)++] = 0;
	buf[(*off)++] = 0;
	buf[(*off)++] = 0;
	buf[(*off)++] = 1;
	memcpy(buf + *off, nal, nalLen);
	*off += nalLen;
}

static void re_vt_callback(void *outputCallbackRefCon, void *sourceFrameRefCon,
	OSStatus status, VTEncodeInfoFlags infoFlags, CMSampleBufferRef sampleBuffer) {
	re_vt_out *o = (re_vt_out *)outputCallbackRefCon;
	if (!o) return;
	pthread_mutex_lock(&o->mu);
	if (o->closed) { pthread_mutex_unlock(&o->mu); return; }
	if (status != noErr || !sampleBuffer) {
		o->ready = 1;
		pthread_cond_signal(&o->cv);
		pthread_mutex_unlock(&o->mu);
		return;
	}

	int isKey = 0;
	CFArrayRef attachments = CMSampleBufferGetSampleAttachmentsArray(sampleBuffer, false);
	if (attachments && CFArrayGetCount(attachments) > 0) {
		CFDictionaryRef dict = CFArrayGetValueAtIndex(attachments, 0);
		CFBooleanRef notSync = CFDictionaryGetValue(dict, kCMSampleAttachmentKey_NotSync);
		if (!notSync || !CFBooleanGetValue(notSync)) isKey = 1;
	}

	size_t total = 64;
	CMFormatDescriptionRef fmt = NULL;
	size_t psCount = 0;
	if (isKey) {
		fmt = CMSampleBufferGetFormatDescription(sampleBuffer);
		if (fmt) {
			if (o->hevc) {
				const uint8_t *ps = NULL; size_t psLen = 0; int nh = 0;
				if (CMVideoFormatDescriptionGetHEVCParameterSetAtIndex(fmt, 0, &ps, &psLen, &psCount, &nh) == noErr) {
					for (size_t i = 0; i < psCount; i++) {
						ps = NULL; psLen = 0;
						if (CMVideoFormatDescriptionGetHEVCParameterSetAtIndex(fmt, i, &ps, &psLen, NULL, NULL) == noErr && ps && psLen)
							total += 4 + psLen;
					}
				}
			} else {
				const uint8_t *sps = NULL; size_t spsLen = 0;
				const uint8_t *pps = NULL; size_t ppsLen = 0;
				size_t nps = 0; int nh = 0;
				CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fmt, 0, &sps, &spsLen, &nps, &nh);
				CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fmt, 1, &pps, &ppsLen, &nps, &nh);
				if (sps && spsLen) total += 4 + spsLen;
				if (pps && ppsLen) total += 4 + ppsLen;
				psCount = nps;
			}
		}
	}

	CMBlockBufferRef block = CMSampleBufferGetDataBuffer(sampleBuffer);
	size_t blockLen = 0;
	char *blockData = NULL;
	if (block) {
		CMBlockBufferGetDataPointer(block, 0, NULL, &blockLen, &blockData);
		total += blockLen + 32;
	}

	unsigned char *buf = (unsigned char *)malloc(total + 64);
	size_t off = 0;
	if (isKey && fmt) {
		if (o->hevc) {
			for (size_t i = 0; i < psCount; i++) {
				const uint8_t *ps = NULL; size_t psLen = 0;
				if (CMVideoFormatDescriptionGetHEVCParameterSetAtIndex(fmt, i, &ps, &psLen, NULL, NULL) == noErr)
					re_vt_append_startcode(buf, &off, ps, psLen);
			}
		} else {
			const uint8_t *sps = NULL; size_t spsLen = 0;
			const uint8_t *pps = NULL; size_t ppsLen = 0;
			size_t nps = 0; int nh = 0;
			CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fmt, 0, &sps, &spsLen, &nps, &nh);
			CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fmt, 1, &pps, &ppsLen, &nps, &nh);
			re_vt_append_startcode(buf, &off, sps, spsLen);
			re_vt_append_startcode(buf, &off, pps, ppsLen);
		}
	}

	if (blockData && blockLen > 4) {
		size_t i = 0;
		while (i + 4 <= blockLen) {
			uint32_t naluLen = ((uint32_t)(unsigned char)blockData[i] << 24) |
				((uint32_t)(unsigned char)blockData[i+1] << 16) |
				((uint32_t)(unsigned char)blockData[i+2] << 8) |
				((uint32_t)(unsigned char)blockData[i+3]);
			i += 4;
			if (i + naluLen > blockLen) break;
			if (off + 4 + naluLen > total + 64) break;
			re_vt_append_startcode(buf, &off, (const uint8_t *)(blockData + i), naluLen);
			i += naluLen;
		}
	}

	if (o->data) free(o->data);
	o->data = buf;
	o->len = off;
	o->keyframe = isKey;
	o->ready = 1;
	pthread_cond_signal(&o->cv);
	pthread_mutex_unlock(&o->mu);
}

typedef struct {
	VTCompressionSessionRef session;
	re_vt_out out;
	int width;
	int height;
	int fps;
	int hevc;
	int64_t frameIndex;
} re_vt_enc;

static void re_vt_apply_bitrate(VTCompressionSessionRef session, int bitrateK) {
	if (!session || bitrateK <= 0) return;
	int64_t bps = (int64_t)bitrateK * 1000;
	CFNumberRef br = CFNumberCreate(NULL, kCFNumberSInt64Type, &bps);
	if (br) {
		VTSessionSetProperty(session, kVTCompressionPropertyKey_AverageBitRate, br);
		CFRelease(br);
	}
	// Soften fat IDRs: ~bitrate over 1s window (bytes, seconds).
	int64_t limits[2] = { bps / 8, 1 };
	CFNumberRef b0 = CFNumberCreate(NULL, kCFNumberSInt64Type, &limits[0]);
	CFNumberRef b1 = CFNumberCreate(NULL, kCFNumberSInt64Type, &limits[1]);
	if (b0 && b1) {
		const void *vals[2] = { b0, b1 };
		CFArrayRef arr = CFArrayCreate(NULL, vals, 2, &kCFTypeArrayCallBacks);
		if (arr) {
			VTSessionSetProperty(session, kVTCompressionPropertyKey_DataRateLimits, arr);
			CFRelease(arr);
		}
	}
	if (b0) CFRelease(b0);
	if (b1) CFRelease(b1);
	float q = 0.45f;
	if (bitrateK >= 12000) q = 0.55f;
	else if (bitrateK <= 6000) q = 0.35f;
	CFNumberRef qref = CFNumberCreate(NULL, kCFNumberFloatType, &q);
	if (qref) {
		VTSessionSetProperty(session, kVTCompressionPropertyKey_Quality, qref);
		CFRelease(qref);
	}
}

static OSStatus re_vt_create(re_vt_enc *e, int width, int height, int fps, int bitrateK, int hevc) {
	memset(e, 0, sizeof(*e));
	e->width = width;
	e->height = height;
	e->fps = fps > 0 ? fps : 15;
	e->hevc = hevc ? 1 : 0;
	e->out.hevc = e->hevc;
	pthread_mutex_init(&e->out.mu, NULL);
	pthread_cond_init(&e->out.cv, NULL);

	CFMutableDictionaryRef srcAttrs = CFDictionaryCreateMutable(NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	int32_t px = kCVPixelFormatType_32BGRA;
	CFNumberRef pixFmt = CFNumberCreate(NULL, kCFNumberSInt32Type, &px);
	CFDictionarySetValue(srcAttrs, kCVPixelBufferPixelFormatTypeKey, pixFmt);
	CFRelease(pixFmt);

	CMVideoCodecType codecType = e->hevc ? kCMVideoCodecType_HEVC : kCMVideoCodecType_H264;
	OSStatus st = VTCompressionSessionCreate(NULL, width, height, codecType,
		NULL, srcAttrs, NULL, re_vt_callback, &e->out, &e->session);
	CFRelease(srcAttrs);
	if (st != noErr) return st;

	VTSessionSetProperty(e->session, kVTCompressionPropertyKey_RealTime, kCFBooleanTrue);
	VTSessionSetProperty(e->session, kVTCompressionPropertyKey_AllowFrameReordering, kCFBooleanFalse);
	if (e->hevc) {
		VTSessionSetProperty(e->session, kVTCompressionPropertyKey_ProfileLevel, kVTProfileLevel_HEVC_Main_AutoLevel);
	} else {
		VTSessionSetProperty(e->session, kVTCompressionPropertyKey_ProfileLevel, kVTProfileLevel_H264_Baseline_AutoLevel);
	}
	int32_t fpsNum = e->fps;
	CFNumberRef fpsRef = CFNumberCreate(NULL, kCFNumberSInt32Type, &fpsNum);
	VTSessionSetProperty(e->session, kVTCompressionPropertyKey_ExpectedFrameRate, fpsRef);
	CFRelease(fpsRef);
	int32_t gop = e->fps * 5;
	if (gop < 30) gop = 30;
	CFNumberRef gopRef = CFNumberCreate(NULL, kCFNumberSInt32Type, &gop);
	VTSessionSetProperty(e->session, kVTCompressionPropertyKey_MaxKeyFrameInterval, gopRef);
	CFRelease(gopRef);
	re_vt_apply_bitrate(e->session, bitrateK > 0 ? bitrateK : 2500);
	VTCompressionSessionPrepareToEncodeFrames(e->session);
	return noErr;
}

static OSStatus re_vt_set_bitrate(re_vt_enc *e, int bitrateK) {
	if (!e || !e->session || bitrateK <= 0) return -1;
	re_vt_apply_bitrate(e->session, bitrateK);
	return noErr;
}

static void re_vt_destroy(re_vt_enc *e) {
	if (!e) return;
	pthread_mutex_lock(&e->out.mu);
	e->out.closed = 1;
	pthread_cond_signal(&e->out.cv);
	pthread_mutex_unlock(&e->out.mu);
	if (e->session) {
		VTCompressionSessionCompleteFrames(e->session, kCMTimeInvalid);
		VTCompressionSessionInvalidate(e->session);
		CFRelease(e->session);
		e->session = NULL;
	}
	if (e->out.data) free(e->out.data);
	pthread_mutex_destroy(&e->out.mu);
	pthread_cond_destroy(&e->out.cv);
}

static OSStatus re_vt_encode_rgba(re_vt_enc *e, const unsigned char *rgba, int stride,
	int forceKey, unsigned char **outAnnexB, size_t *outLen, int *outKey) {
	if (!e || !e->session || !rgba) return -1;
	// Zero-copy wrap of the Go BGRA buffer (SCK keeps BGRA). Avoids a 56MB memcpy at 5K.
	CVPixelBufferRef pb = NULL;
	OSStatus st = CVPixelBufferCreateWithBytes(NULL, e->width, e->height, kCVPixelFormatType_32BGRA,
		(void *)rgba, (size_t)stride, NULL, NULL, NULL, &pb);
	if (st != noErr) return st;

	pthread_mutex_lock(&e->out.mu);
	e->out.ready = 0;
	e->out.hevc = e->hevc;
	if (e->out.data) { free(e->out.data); e->out.data = NULL; e->out.len = 0; }
	pthread_mutex_unlock(&e->out.mu);

	CMTime pts = CMTimeMake(e->frameIndex++, e->fps);
	CFMutableDictionaryRef frameProps = NULL;
	if (forceKey) {
		frameProps = CFDictionaryCreateMutable(NULL, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
		CFDictionarySetValue(frameProps, kVTEncodeFrameOptionKey_ForceKeyFrame, kCFBooleanTrue);
	}
	VTEncodeInfoFlags flags = 0;
	st = VTCompressionSessionEncodeFrame(e->session, pb, pts, kCMTimeInvalid, frameProps, NULL, &flags);
	if (frameProps) CFRelease(frameProps);
	CFRelease(pb);
	if (st != noErr) return st;

	pthread_mutex_lock(&e->out.mu);
	while (!e->out.ready && !e->out.closed) {
		pthread_cond_wait(&e->out.cv, &e->out.mu);
	}
	if (e->out.closed || !e->out.data || e->out.len == 0) {
		pthread_mutex_unlock(&e->out.mu);
		return -2;
	}
	*outAnnexB = (unsigned char *)malloc(e->out.len);
	memcpy(*outAnnexB, e->out.data, e->out.len);
	*outLen = e->out.len;
	*outKey = e->out.keyframe;
	pthread_mutex_unlock(&e->out.mu);
	return noErr;
}
*/
import "C"

import (
	"fmt"
	"log"
	"unsafe"
)

type vtEncoder struct {
	enc      *C.re_vt_enc
	w        int
	h        int
	fps      int
	bitrateK int
	hevc     bool
}

func newVideoToolboxEncoder(width, height, fps, bitrateK int, hevc bool) (Encoder, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("desktop: invalid encode size")
	}
	width &^= 1
	height &^= 1
	if bitrateK <= 0 {
		bitrateK = 2500
	}
	e := (*C.re_vt_enc)(C.malloc(C.size_t(unsafe.Sizeof(C.re_vt_enc{}))))
	if e == nil {
		return nil, fmt.Errorf("desktop: alloc vt encoder")
	}
	hevcFlag := 0
	if hevc {
		hevcFlag = 1
	}
	if st := C.re_vt_create(e, C.int(width), C.int(height), C.int(fps), C.int(bitrateK), C.int(hevcFlag)); st != 0 {
		C.free(unsafe.Pointer(e))
		return nil, fmt.Errorf("desktop: VideoToolbox create failed: %d hevc=%v", int(st), hevc)
	}
	return &vtEncoder{enc: e, w: width, h: height, fps: fps, bitrateK: bitrateK, hevc: hevc}, nil
}

// NewEncoder prefers native VideoToolbox hard encode, then ffmpeg, then synthetic.
func NewEncoder(width, height, fps int) (Encoder, error) {
	return NewEncoderBitrate(width, height, fps, 2500)
}

// NewEncoderBitrate creates an H.264 encoder with an explicit target bitrate (kbps).
func NewEncoderBitrate(width, height, fps, bitrateK int) (Encoder, error) {
	return NewEncoderBitrateCodec(width, height, fps, bitrateK, false)
}

// vtHardMax is the largest dimension Apple Silicon VideoToolbox HEVC will
// actually encode. Above this, VTCompressionSessionCreate may succeed without
// RequireHardware but EncodeFrame returns empty (−2) — see 16K probe on M5.
const vtHardMaxDim = 8192

// NewEncoderBitrateCodec creates H.264 or HEVC (hevc=true) VideoToolbox encoder.
// HEVC create failure falls back to H.264 so OPEN never hard-fails on older hosts.
// Dimensions above vtHardMaxDim skip VT and use ffmpeg libx265 for full-blood 16K.
func NewEncoderBitrateCodec(width, height, fps, bitrateK int, hevc bool) (Encoder, error) {
	if fps <= 0 {
		fps = 15
	}
	if bitrateK <= 0 {
		bitrateK = 2500
	}
	width &^= 1
	height &^= 1
	overVT := width > vtHardMaxDim || height > vtHardMaxDim
	if !overVT {
		if enc, err := newVideoToolboxEncoder(width, height, fps, bitrateK, hevc); err == nil {
			return enc, nil
		} else if hevc {
			// Fall back so 5K hosts without HEVC encode still stream.
			return NewEncoderBitrateCodec(width, height, fps, bitrateK, false)
		}
	} else {
		log.Printf("desktop: %dx%d exceeds VT max %d — using ffmpeg/libx265 for full-blood encode", width, height, vtHardMaxDim)
	}
	if _, err := lookPath("ffmpeg"); err == nil {
		if hevc || overVT {
			// 16K full-blood: VT cannot emit frames; libx265 is required.
			if enc, err := newFFmpegEncoder(width, height, fps, bitrateK, "libx265"); err == nil {
				return enc, nil
			}
			if !overVT {
				if enc, err := newFFmpegEncoder(width, height, fps, bitrateK, "hevc_videotoolbox"); err == nil {
					return enc, nil
				}
			}
		}
		codec := "h264_videotoolbox"
		if enc, err := newFFmpegEncoder(width, height, fps, bitrateK, codec); err == nil {
			return enc, nil
		}
		return newFFmpegEncoder(width, height, fps, bitrateK, "libx264")
	}
	if overVT {
		return nil, fmt.Errorf("desktop: %dx%d needs ffmpeg/libx265 (VideoToolbox max %d)", width, height, vtHardMaxDim)
	}
	return &syntheticEncoder{w: width, h: height}, nil
}

func (e *vtEncoder) CodecName() string {
	if e != nil && e.hevc {
		return CodecH265
	}
	return CodecH264
}

// Reconfigure hot-updates VT bitrate; size changes recreate the session.
func (e *vtEncoder) Reconfigure(width, height, fps, bitrateK int) error {
	if e == nil || e.enc == nil {
		return fmt.Errorf("desktop: vt encoder not ready")
	}
	width &^= 1
	height &^= 1
	if fps <= 0 {
		fps = e.fps
	}
	if bitrateK <= 0 {
		bitrateK = e.bitrateK
	}
	if width == e.w && height == e.h {
		if bitrateK != e.bitrateK {
			_ = C.re_vt_set_bitrate(e.enc, C.int(bitrateK))
			e.bitrateK = bitrateK
		}
		e.fps = fps
		return nil
	}
	hevcFlag := 0
	if e.hevc {
		hevcFlag = 1
	}
	C.re_vt_destroy(e.enc)
	if st := C.re_vt_create(e.enc, C.int(width), C.int(height), C.int(fps), C.int(bitrateK), C.int(hevcFlag)); st != 0 {
		return fmt.Errorf("desktop: VT reconfigure create: %d", int(st))
	}
	e.w, e.h, e.fps, e.bitrateK = width, height, fps, bitrateK
	return nil
}

func (e *vtEncoder) Encode(f Frame, keyframe bool) ([]byte, error) {
	if e == nil || e.enc == nil || f.Img == nil {
		return nil, fmt.Errorf("desktop: vt encoder not ready")
	}
	img := f.Img
	if img.Bounds().Dx() != e.w || img.Bounds().Dy() != e.h {
		img = scaleRGBAExact(img, e.w, e.h)
	}
	force := C.int(0)
	if keyframe {
		force = 1
	}
	var out *C.uchar
	var outLen C.size_t
	var outKey C.int
	st := C.re_vt_encode_rgba(e.enc, (*C.uchar)(unsafe.Pointer(&img.Pix[0])), C.int(img.Stride),
		force, &out, &outLen, &outKey)
	if st != 0 {
		return nil, fmt.Errorf("desktop: VideoToolbox encode failed: %d", int(st))
	}
	defer C.free(unsafe.Pointer(out))
	b := C.GoBytes(unsafe.Pointer(out), C.int(outLen))
	return b, nil
}

func (e *vtEncoder) Close() error {
	if e.enc != nil {
		C.re_vt_destroy(e.enc)
		C.free(unsafe.Pointer(e.enc))
		e.enc = nil
	}
	return nil
}
