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
	pthread_mutex_t mu;
	pthread_cond_t cv;
	int ready;
	int closed;
} re_vt_out;

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

	// Build Annex-B: parameter sets on keyframes + length-prefixed NALs -> start codes
	size_t total = 0;
	if (isKey) {
		CMFormatDescriptionRef fmt = CMSampleBufferGetFormatDescription(sampleBuffer);
		size_t spsCount = 0;
		const uint8_t *sps = NULL;
		size_t spsLen = 0;
		size_t ppsCount = 0;
		const uint8_t *pps = NULL;
		size_t ppsLen = 0;
		if (fmt) {
			CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fmt, 0, &sps, &spsLen, &spsCount, NULL);
			CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fmt, 1, &pps, &ppsLen, &ppsCount, NULL);
			if (sps && spsLen) total += 4 + spsLen;
			if (pps && ppsLen) total += 4 + ppsLen;
		}
		(void)spsCount; (void)ppsCount;
	}

	CMBlockBufferRef block = CMSampleBufferGetDataBuffer(sampleBuffer);
	size_t blockLen = 0;
	char *blockData = NULL;
	if (block) {
		CMBlockBufferGetDataPointer(block, 0, NULL, &blockLen, &blockData);
		total += blockLen + 32; // room for start-code conversion overhead
	}

	unsigned char *buf = (unsigned char *)malloc(total + 64);
	size_t off = 0;
	if (isKey) {
		CMFormatDescriptionRef fmt = CMSampleBufferGetFormatDescription(sampleBuffer);
		const uint8_t *sps = NULL; size_t spsLen = 0;
		const uint8_t *pps = NULL; size_t ppsLen = 0;
		size_t nps = 0; int nalHeaderLen = 0;
		if (fmt) {
			CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fmt, 0, &sps, &spsLen, &nps, &nalHeaderLen);
			CMVideoFormatDescriptionGetH264ParameterSetAtIndex(fmt, 1, &pps, &ppsLen, &nps, &nalHeaderLen);
		}
		if (sps && spsLen) {
			buf[off++]=0; buf[off++]=0; buf[off++]=0; buf[off++]=1;
			memcpy(buf+off, sps, spsLen); off += spsLen;
		}
		if (pps && ppsLen) {
			buf[off++]=0; buf[off++]=0; buf[off++]=0; buf[off++]=1;
			memcpy(buf+off, pps, ppsLen); off += ppsLen;
		}
		(void)nalHeaderLen;
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
			buf[off++]=0; buf[off++]=0; buf[off++]=0; buf[off++]=1;
			memcpy(buf+off, blockData+i, naluLen);
			off += naluLen;
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
	int64_t frameIndex;
} re_vt_enc;

static OSStatus re_vt_create(re_vt_enc *e, int width, int height, int fps) {
	memset(e, 0, sizeof(*e));
	e->width = width;
	e->height = height;
	e->fps = fps > 0 ? fps : 15;
	pthread_mutex_init(&e->out.mu, NULL);
	pthread_cond_init(&e->out.cv, NULL);

	CFMutableDictionaryRef srcAttrs = CFDictionaryCreateMutable(NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	int32_t px = kCVPixelFormatType_32BGRA;
	CFNumberRef pixFmt = CFNumberCreate(NULL, kCFNumberSInt32Type, &px);
	CFDictionarySetValue(srcAttrs, kCVPixelBufferPixelFormatTypeKey, pixFmt);
	CFRelease(pixFmt);

	OSStatus st = VTCompressionSessionCreate(NULL, width, height, kCMVideoCodecType_H264,
		NULL, srcAttrs, NULL, re_vt_callback, &e->out, &e->session);
	CFRelease(srcAttrs);
	if (st != noErr) return st;

	VTSessionSetProperty(e->session, kVTCompressionPropertyKey_RealTime, kCFBooleanTrue);
	VTSessionSetProperty(e->session, kVTCompressionPropertyKey_ProfileLevel, kVTProfileLevel_H264_Baseline_AutoLevel);
	VTSessionSetProperty(e->session, kVTCompressionPropertyKey_AllowFrameReordering, kCFBooleanFalse);
	int32_t fpsNum = e->fps;
	CFNumberRef fpsRef = CFNumberCreate(NULL, kCFNumberSInt32Type, &fpsNum);
	VTSessionSetProperty(e->session, kVTCompressionPropertyKey_ExpectedFrameRate, fpsRef);
	CFRelease(fpsRef);
	int32_t gop = e->fps * 2;
	CFNumberRef gopRef = CFNumberCreate(NULL, kCFNumberSInt32Type, &gop);
	VTSessionSetProperty(e->session, kVTCompressionPropertyKey_MaxKeyFrameInterval, gopRef);
	CFRelease(gopRef);
	VTCompressionSessionPrepareToEncodeFrames(e->session);
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
	CVPixelBufferRef pb = NULL;
	OSStatus st = CVPixelBufferCreate(NULL, e->width, e->height, kCVPixelFormatType_32BGRA,
		NULL, &pb);
	if (st != noErr) return st;
	CVPixelBufferLockBaseAddress(pb, 0);
	unsigned char *dst = (unsigned char *)CVPixelBufferGetBaseAddress(pb);
	size_t dstStride = CVPixelBufferGetBytesPerRow(pb);
	for (int y = 0; y < e->height; y++) {
		const unsigned char *srcRow = rgba + y * stride;
		unsigned char *dstRow = dst + y * dstStride;
		for (int x = 0; x < e->width; x++) {
			// RGBA -> BGRA
			dstRow[x*4+0] = srcRow[x*4+2];
			dstRow[x*4+1] = srcRow[x*4+1];
			dstRow[x*4+2] = srcRow[x*4+0];
			dstRow[x*4+3] = srcRow[x*4+3];
		}
	}
	CVPixelBufferUnlockBaseAddress(pb, 0);

	pthread_mutex_lock(&e->out.mu);
	e->out.ready = 0;
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
	"unsafe"
)

type vtEncoder struct {
	enc *C.re_vt_enc
	w   int
	h   int
}

func newVideoToolboxEncoder(width, height, fps int) (Encoder, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("desktop: invalid encode size")
	}
	width &^= 1
	height &^= 1
	e := (*C.re_vt_enc)(C.malloc(C.size_t(unsafe.Sizeof(C.re_vt_enc{}))))
	if e == nil {
		return nil, fmt.Errorf("desktop: alloc vt encoder")
	}
	if st := C.re_vt_create(e, C.int(width), C.int(height), C.int(fps)); st != 0 {
		C.free(unsafe.Pointer(e))
		return nil, fmt.Errorf("desktop: VideoToolbox create failed: %d", int(st))
	}
	return &vtEncoder{enc: e, w: width, h: height}, nil
}

// NewEncoder prefers native VideoToolbox hard encode, then ffmpeg h264_videotoolbox, then libx264.
func NewEncoder(width, height, fps int) (Encoder, error) {
	return NewEncoderBitrate(width, height, fps, 2500)
}

// NewEncoderBitrate creates an encoder with an explicit target bitrate (kbps).
func NewEncoderBitrate(width, height, fps, bitrateK int) (Encoder, error) {
	if fps <= 0 {
		fps = 15
	}
	if bitrateK <= 0 {
		bitrateK = 2500
	}
	width &^= 1
	height &^= 1
	if enc, err := newVideoToolboxEncoder(width, height, fps); err == nil {
		return enc, nil
	}
	if _, err := lookPath("ffmpeg"); err == nil {
		if enc, err := newFFmpegEncoder(width, height, fps, bitrateK, "h264_videotoolbox"); err == nil {
			return enc, nil
		}
		return newFFmpegEncoder(width, height, fps, bitrateK, "libx264")
	}
	return &syntheticEncoder{w: width, h: height}, nil
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
