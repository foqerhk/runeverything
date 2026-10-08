// Continuous ScreenCaptureKit stream + one-shot helpers.
//go:build darwin

#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <CoreGraphics/CoreGraphics.h>
#import <CoreFoundation/CoreFoundation.h>
#import <CoreVideo/CoreVideo.h>
#import <CoreMedia/CoreMedia.h>
#import <VideoToolbox/VideoToolbox.h>
#import <dispatch/dispatch.h>
#import <stdlib.h>
#import <string.h>
#import <pthread.h>

typedef struct {
	unsigned char *data;
	int w;
	int h;
	int stride;
	int err;
} re_sck_shot;

typedef struct {
	unsigned char *data;
	int len;
	int keyframe;
	int w;
	int h;
	int err;
} re_sck_nal;

static unsigned char *cgimage_to_rgba(CGImageRef img, int *w, int *h, int *stride) {
	size_t width = CGImageGetWidth(img);
	size_t height = CGImageGetHeight(img);
	size_t bpr = width * 4;
	unsigned char *buf = (unsigned char *)malloc(bpr * height);
	if (!buf) return NULL;
	memset(buf, 0, bpr * height);
	CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
	CGContextRef ctx = CGBitmapContextCreate(buf, width, height, 8, bpr, cs,
		kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Little);
	CGColorSpaceRelease(cs);
	if (!ctx) { free(buf); return NULL; }
	CGContextDrawImage(ctx, CGRectMake(0, 0, width, height), img);
	CGContextRelease(ctx);
	*w = (int)width;
	*h = (int)height;
	*stride = (int)bpr;
	return buf;
}

void re_sck_capture_display_ex(re_sck_shot *out, int display_index, int show_cursor);

void re_sck_request_access(void) {
	if (@available(macOS 14.0, *)) {
		dispatch_semaphore_t sem = dispatch_semaphore_create(0);
		[SCShareableContent getShareableContentWithCompletionHandler:^(SCShareableContent * _Nullable content, NSError * _Nullable error) {
			(void)content; (void)error;
			dispatch_semaphore_signal(sem);
		}];
		dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(3 * NSEC_PER_SEC)));
	}
}

void re_sck_capture(re_sck_shot *out) { re_sck_capture_display_ex(out, 0, 1); }
void re_sck_capture_display(re_sck_shot *out, int display_index) { re_sck_capture_display_ex(out, display_index, 1); }

void re_sck_capture_display_ex(re_sck_shot *out, int display_index, int show_cursor) {
	memset(out, 0, sizeof(*out));
	if (@available(macOS 14.0, *)) {
		dispatch_semaphore_t sem = dispatch_semaphore_create(0);
		__block SCShareableContent *contentHold = nil;
		__block NSError *contentErr = nil;
		[SCShareableContent getShareableContentWithCompletionHandler:^(SCShareableContent * _Nullable content, NSError * _Nullable error) {
			contentHold = content; contentErr = error; dispatch_semaphore_signal(sem);
		}];
		dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(8 * NSEC_PER_SEC)));
		if (contentErr || contentHold == nil || contentHold.displays.count == 0) { out->err = -1; return; }
		SCDisplay *display = nil;
		// Prefer CGDirectDisplayID match; fall back to array index for legacy clients (0/1).
		for (SCDisplay *d in contentHold.displays) {
			if ((int)d.displayID == display_index) { display = d; break; }
		}
		if (!display) {
			NSUInteger idx = display_index < 0 ? 0 : (NSUInteger)display_index;
			if (idx >= contentHold.displays.count) idx = 0;
			display = contentHold.displays[idx];
		}
		SCContentFilter *filter = [[SCContentFilter alloc] initWithDisplay:display excludingWindows:@[]];
		SCStreamConfiguration *config = [SCStreamConfiguration new];
		config.width = display.width; config.height = display.height;
		config.showsCursor = show_cursor ? YES : NO;
		config.pixelFormat = kCVPixelFormatType_32BGRA;
		__block CGImageRef shot = NULL;
		__block NSError *shotErr = nil;
		dispatch_semaphore_t sem2 = dispatch_semaphore_create(0);
		[SCScreenshotManager captureImageWithFilter:filter configuration:config completionHandler:^(CGImageRef _Nullable image, NSError * _Nullable error) {
			if (image) shot = CGImageRetain(image); shotErr = error; dispatch_semaphore_signal(sem2);
		}];
		dispatch_semaphore_wait(sem2, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(8 * NSEC_PER_SEC)));
		if (shotErr || shot == NULL) { out->err = -2; if (shot) CGImageRelease(shot); return; }
		int w=0,h=0,stride=0;
		unsigned char *rgba = cgimage_to_rgba(shot, &w, &h, &stride);
		CGImageRelease(shot);
		if (!rgba) { out->err = -3; return; }
		out->data = rgba; out->w = w; out->h = h; out->stride = stride; out->err = 0;
		return;
	}
	out->err = -1;
}

int re_sck_main_size(int *w, int *h) {
	if (@available(macOS 14.0, *)) {
		dispatch_semaphore_t sem = dispatch_semaphore_create(0);
		__block SCShareableContent *contentHold = nil;
		[SCShareableContent getShareableContentWithCompletionHandler:^(SCShareableContent * _Nullable content, NSError * _Nullable error) {
			contentHold = content; dispatch_semaphore_signal(sem);
		}];
		dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(5 * NSEC_PER_SEC)));
		if (contentHold && contentHold.displays.count > 0) {
			SCDisplay *d = contentHold.displays.firstObject;
			*w = (int)d.width; *h = (int)d.height; return 0;
		}
	}
	CGDirectDisplayID id = CGMainDisplayID();
	*w = (int)CGDisplayPixelsWide(id);
	*h = (int)CGDisplayPixelsHigh(id);
	return 0;
}

// ---- Continuous SCStream (+ optional inline HEVC) ----

@class REStreamOut;
@interface REStreamOut : NSObject <SCStreamOutput, SCStreamDelegate>
@property (atomic) unsigned char *frame;
@property (atomic) int w;
@property (atomic) int h;
@property (atomic) int stride;
@property (atomic) int ready;
@property (atomic) unsigned char *nal;
@property (atomic) int nalLen;
@property (atomic) int nalKey;
@property (atomic) int nalReady;
@end

static SCStream *g_stream = nil;
static REStreamOut *g_out = nil;
static pthread_mutex_t g_mu = PTHREAD_MUTEX_INITIALIZER;
static VTCompressionSessionRef g_vt = NULL;
static int g_inline_hevc = 0;
static int g_force_key = 0;
static int64_t g_frame_index = 0;
static int g_fps = 15;
static int g_out_w = 0;
static int g_out_h = 0;
static dispatch_queue_t g_encode_q = NULL;
static int g_encode_busy = 0;

int re_sck_stream_start_ex(int display_index, int show_cursor, int maxW, int maxH,
	int bitrateK, int fps, int hevc, int *w, int *h);

static void re_vt_append_startcode(unsigned char *buf, size_t *off, const uint8_t *nal, size_t nalLen) {
	if (!nal || nalLen == 0) return;
	buf[(*off)++] = 0; buf[(*off)++] = 0; buf[(*off)++] = 0; buf[(*off)++] = 1;
	memcpy(buf + *off, nal, nalLen);
	*off += nalLen;
}

static void re_inline_vt_callback(void *outputCallbackRefCon, void *sourceFrameRefCon,
	OSStatus status, VTEncodeInfoFlags infoFlags, CMSampleBufferRef sampleBuffer) {
	(void)outputCallbackRefCon; (void)sourceFrameRefCon; (void)infoFlags;
	REStreamOut *o = g_out;
	if (!o || status != noErr || !sampleBuffer) return;

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
			const uint8_t *ps = NULL; size_t psLen = 0; int nh = 0;
			if (CMVideoFormatDescriptionGetHEVCParameterSetAtIndex(fmt, 0, &ps, &psLen, &psCount, &nh) == noErr) {
				for (size_t i = 0; i < psCount; i++) {
					ps = NULL; psLen = 0;
					if (CMVideoFormatDescriptionGetHEVCParameterSetAtIndex(fmt, i, &ps, &psLen, NULL, NULL) == noErr && ps && psLen)
						total += 4 + psLen;
				}
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
	if (!buf) return;
	size_t off = 0;
	if (isKey && fmt) {
		for (size_t i = 0; i < psCount; i++) {
			const uint8_t *ps = NULL; size_t psLen = 0;
			if (CMVideoFormatDescriptionGetHEVCParameterSetAtIndex(fmt, i, &ps, &psLen, NULL, NULL) == noErr)
				re_vt_append_startcode(buf, &off, ps, psLen);
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

	pthread_mutex_lock(&g_mu);
	if (g_out != o) {
		pthread_mutex_unlock(&g_mu);
		free(buf);
		return;
	}
	if (o.nal) free(o.nal);
	o.nal = buf;
	o.nalLen = (int)off;
	o.nalKey = isKey;
	o.nalReady = 1;
	o.w = g_out_w;
	o.h = g_out_h;
	pthread_mutex_unlock(&g_mu);
}

@implementation REStreamOut
- (void)stream:(SCStream *)stream didOutputSampleBuffer:(CMSampleBufferRef)sampleBuffer ofType:(SCStreamOutputType)type {
	if (type != SCStreamOutputTypeScreen) return;
	CVImageBufferRef imgBuf = CMSampleBufferGetImageBuffer(sampleBuffer);
	if (!imgBuf) return;

	pthread_mutex_lock(&g_mu);
	int inlineHEVC = g_inline_hevc;
	VTCompressionSessionRef vt = g_vt;
	int forceKey = g_force_key;
	int fps = g_fps > 0 ? g_fps : 15;
	int64_t idx = g_frame_index++;
	if (forceKey) g_force_key = 0;
	REStreamOut *selfRef = g_out;
	pthread_mutex_unlock(&g_mu);
	if (selfRef != self) return;

	if (inlineHEVC && vt) {
		// Never block the SCK sample-handler queue on EncodeFrame. Cap in-flight
		// encodes so the serial queue cannot accumulate seconds of latency.
		pthread_mutex_lock(&g_mu);
		if (g_encode_busy >= 2) {
			pthread_mutex_unlock(&g_mu);
			return;
		}
		g_encode_busy++;
		pthread_mutex_unlock(&g_mu);
		CVPixelBufferRetain(imgBuf);
		CMTime pts = CMTimeMake(idx, fps);
		dispatch_async(g_encode_q, ^{
			CFMutableDictionaryRef frameProps = NULL;
			if (forceKey) {
				frameProps = CFDictionaryCreateMutable(NULL, 1, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
				CFDictionarySetValue(frameProps, kVTEncodeFrameOptionKey_ForceKeyFrame, kCFBooleanTrue);
			}
			VTEncodeInfoFlags flags = 0;
			VTCompressionSessionEncodeFrame(vt, imgBuf, pts, kCMTimeInvalid, frameProps, NULL, &flags);
			if (frameProps) CFRelease(frameProps);
			CVPixelBufferRelease(imgBuf);
			pthread_mutex_lock(&g_mu);
			if (g_encode_busy > 0) g_encode_busy--;
			pthread_mutex_unlock(&g_mu);
		});
		return;
	}

	CVPixelBufferLockBaseAddress(imgBuf, kCVPixelBufferLock_ReadOnly);
	size_t width = CVPixelBufferGetWidth(imgBuf);
	size_t height = CVPixelBufferGetHeight(imgBuf);
	size_t bpr = CVPixelBufferGetBytesPerRow(imgBuf);
	uint8_t *base = (uint8_t *)CVPixelBufferGetBaseAddress(imgBuf);
	size_t outBpr = width * 4;
	unsigned char *buf = NULL;
	if (base && width > 0 && height > 0) {
		buf = (unsigned char *)malloc(outBpr * height);
		if (buf) {
			for (size_t y = 0; y < height; y++) {
				memcpy(buf + y * outBpr, base + y * bpr, outBpr);
			}
		}
	}
	CVPixelBufferUnlockBaseAddress(imgBuf, kCVPixelBufferLock_ReadOnly);
	if (!buf) return;

	pthread_mutex_lock(&g_mu);
	if (g_out != self) {
		pthread_mutex_unlock(&g_mu);
		free(buf);
		return;
	}
	unsigned char *old = self.frame;
	self.frame = buf;
	self.w = (int)width;
	self.h = (int)height;
	self.stride = (int)outBpr;
	self.ready = 1;
	pthread_mutex_unlock(&g_mu);
	if (old) free(old);
}
@end

static void re_inline_vt_destroy_locked(void) {
	if (g_vt) {
		VTCompressionSessionCompleteFrames(g_vt, kCMTimeInvalid);
		VTCompressionSessionInvalidate(g_vt);
		CFRelease(g_vt);
		g_vt = NULL;
	}
	g_inline_hevc = 0;
}

static OSStatus re_inline_vt_create(int width, int height, int fps, int bitrateK) {
	re_inline_vt_destroy_locked();
	if (!g_encode_q) {
		g_encode_q = dispatch_queue_create("re.sck.hevc", DISPATCH_QUEUE_SERIAL);
	}
	g_encode_busy = 0;
	CFMutableDictionaryRef srcAttrs = CFDictionaryCreateMutable(NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	int32_t px = kCVPixelFormatType_32BGRA;
	CFNumberRef pixFmt = CFNumberCreate(NULL, kCFNumberSInt32Type, &px);
	CFDictionarySetValue(srcAttrs, kCVPixelBufferPixelFormatTypeKey, pixFmt);
	CFRelease(pixFmt);
	CFDictionaryRef iosurface = CFDictionaryCreate(NULL, NULL, NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	if (iosurface) {
		CFDictionarySetValue(srcAttrs, kCVPixelBufferIOSurfacePropertiesKey, iosurface);
		CFRelease(iosurface);
	}
	CFMutableDictionaryRef encSpec = CFDictionaryCreateMutable(NULL, 0,
		&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFDictionarySetValue(encSpec, kVTVideoEncoderSpecification_EnableHardwareAcceleratedVideoEncoder, kCFBooleanTrue);
	CFDictionarySetValue(encSpec, kVTVideoEncoderSpecification_RequireHardwareAcceleratedVideoEncoder, kCFBooleanTrue);
	OSStatus st = VTCompressionSessionCreate(NULL, width, height, kCMVideoCodecType_HEVC,
		encSpec, srcAttrs, NULL, re_inline_vt_callback, NULL, &g_vt);
	if (st != noErr) {
		CFRelease(encSpec);
		encSpec = CFDictionaryCreateMutable(NULL, 0,
			&kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
		CFDictionarySetValue(encSpec, kVTVideoEncoderSpecification_EnableHardwareAcceleratedVideoEncoder, kCFBooleanTrue);
		st = VTCompressionSessionCreate(NULL, width, height, kCMVideoCodecType_HEVC,
			encSpec, srcAttrs, NULL, re_inline_vt_callback, NULL, &g_vt);
	}
	CFRelease(encSpec);
	CFRelease(srcAttrs);
	if (st != noErr) return st;
	VTSessionSetProperty(g_vt, kVTCompressionPropertyKey_RealTime, kCFBooleanTrue);
	CFBooleanRef usingHW = NULL;
	if (VTSessionCopyProperty(g_vt, kVTCompressionPropertyKey_UsingHardwareAcceleratedVideoEncoder,
			kCFAllocatorDefault, (void *)&usingHW) == noErr && usingHW) {
		NSLog(@"re_sck inline HEVC hardware=%@", CFBooleanGetValue(usingHW) ? @"YES" : @"NO");
		CFRelease(usingHW);
	}
	VTSessionSetProperty(g_vt, kVTCompressionPropertyKey_AllowFrameReordering, kCFBooleanFalse);
	VTSessionSetProperty(g_vt, kVTCompressionPropertyKey_ProfileLevel, kVTProfileLevel_HEVC_Main_AutoLevel);
	g_fps = fps > 0 ? fps : 15;
	int32_t fpsNum = g_fps;
	CFNumberRef fpsRef = CFNumberCreate(NULL, kCFNumberSInt32Type, &fpsNum);
	VTSessionSetProperty(g_vt, kVTCompressionPropertyKey_ExpectedFrameRate, fpsRef);
	CFRelease(fpsRef);
	int32_t gop = g_fps * 8;
	if (gop < 60) gop = 60;
	CFNumberRef gopRef = CFNumberCreate(NULL, kCFNumberSInt32Type, &gop);
	VTSessionSetProperty(g_vt, kVTCompressionPropertyKey_MaxKeyFrameInterval, gopRef);
	CFRelease(gopRef);
	int64_t bps = (int64_t)(bitrateK > 0 ? bitrateK : 8000) * 1000;
	CFNumberRef br = CFNumberCreate(NULL, kCFNumberSInt64Type, &bps);
	if (br) {
		VTSessionSetProperty(g_vt, kVTCompressionPropertyKey_AverageBitRate, br);
		CFRelease(br);
	}
	float q = 0.4f;
	CFNumberRef qref = CFNumberCreate(NULL, kCFNumberFloatType, &q);
	if (qref) {
		VTSessionSetProperty(g_vt, kVTCompressionPropertyKey_Quality, qref);
		CFRelease(qref);
	}
	VTCompressionSessionPrepareToEncodeFrames(g_vt);
	g_inline_hevc = 1;
	g_frame_index = 0;
	return noErr;
}

int re_sck_stream_start(int display_index, int show_cursor, int *w, int *h) {
	return re_sck_stream_start_ex(display_index, show_cursor, 0, 0, 0, 0, 0, w, h);
}

// maxW/maxH: optional GPU-side scale target. hevc!=0 enables inline VT HEVC (no Go pixel path).
int re_sck_stream_start_ex(int display_index, int show_cursor, int maxW, int maxH,
	int bitrateK, int fps, int hevc, int *w, int *h) {
	if (@available(macOS 14.0, *)) {
		pthread_mutex_lock(&g_mu);
		if (g_stream) {
			[g_stream stopCaptureWithCompletionHandler:^(__unused NSError *e){}];
			g_stream = nil;
		}
		re_inline_vt_destroy_locked();
		dispatch_semaphore_t sem = dispatch_semaphore_create(0);
		__block SCShareableContent *contentHold = nil;
		[SCShareableContent getShareableContentWithCompletionHandler:^(SCShareableContent * _Nullable content, NSError * _Nullable error) {
			contentHold = content; dispatch_semaphore_signal(sem);
		}];
		dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(8 * NSEC_PER_SEC)));
		if (!contentHold || contentHold.displays.count == 0) { pthread_mutex_unlock(&g_mu); return -1; }
		SCDisplay *display = nil;
		// Prefer CGDirectDisplayID match; fall back to array index for legacy clients (0/1).
		for (SCDisplay *d in contentHold.displays) {
			if ((int)d.displayID == display_index) { display = d; break; }
		}
		if (!display) {
			NSUInteger idx = display_index < 0 ? 0 : (NSUInteger)display_index;
			if (idx >= contentHold.displays.count) idx = 0;
			display = contentHold.displays[idx];
		}
		int outW = (int)display.width, outH = (int)display.height;
		// Virtual 8K/16K: SCDisplay may expose HiDPI logical size while the
		// client OPEN targets framebuffer pixels (fb_width/height). Honor a
		// larger maxW×maxH so we can stream full-blood 16K, never upscale a
		// real panel past its native pixels unless the client asks higher.
		if (maxW > 0 && maxH > 0 && maxW > outW && maxH > outH) {
			outW = maxW;
			outH = maxH;
		} else {
			if (maxW > 0 && maxW < outW) {
				outH = (int)((int64_t)outH * maxW / outW);
				outW = maxW;
			}
			if (maxH > 0 && maxH < outH) {
				outW = (int)((int64_t)outW * maxH / outH);
				outH = maxH;
			}
		}
		outW &= ~1; outH &= ~1;
		if (outW < 2) outW = 2;
		if (outH < 2) outH = 2;

		SCContentFilter *filter = [[SCContentFilter alloc] initWithDisplay:display excludingWindows:@[]];
		SCStreamConfiguration *config = [SCStreamConfiguration new];
		config.width = outW; config.height = outH;
		int useFps = fps > 0 ? fps : 15;
		// Never *raise* fps for hi-res — 8K VT cannot drain 15fps (encode_busy
		// drops every subsequent sample → one IDR then freeze). Cap by class.
		if (outW > 8192 || outH > 8192) {
			if (useFps > 2) useFps = 2;
		} else if (outW >= 7680 || outH >= 4320) {
			if (useFps > 5) useFps = 5;
		} else if (outW >= 3840 || outH >= 2160) {
			if (useFps > 15) useFps = 15;
		}
		config.minimumFrameInterval = CMTimeMake(1, useFps);
		config.showsCursor = show_cursor ? YES : NO;
		config.pixelFormat = kCVPixelFormatType_32BGRA;
		config.queueDepth = 2;
		g_out = [REStreamOut new];
		if (hevc) {
			// VT HEVC on Apple Silicon is unreliable / silent above ~8K.
			// Full-blood 16K must use pixel frames + Go/ffmpeg encode.
			BOOL tooBig = (outW > 8192 || outH > 8192);
			if (tooBig || re_inline_vt_create(outW, outH, useFps, bitrateK) != noErr) {
				fprintf(stderr, "re_sck: inline VT HEVC %dx%d skipped/failed — pixel fallback\n", outW, outH);
				hevc = 0;
			}
		}
		g_stream = [[SCStream alloc] initWithFilter:filter configuration:config delegate:g_out];
		NSError *err = nil;
		BOOL ok = [g_stream addStreamOutput:g_out type:SCStreamOutputTypeScreen sampleHandlerQueue:dispatch_get_global_queue(QOS_CLASS_USER_INTERACTIVE, 0) error:&err];
		if (!ok) {
			re_inline_vt_destroy_locked();
			g_stream = nil; g_out = nil;
			pthread_mutex_unlock(&g_mu);
			return -2;
		}
		dispatch_semaphore_t sem2 = dispatch_semaphore_create(0);
		[g_stream startCaptureWithCompletionHandler:^(NSError * _Nullable error) {
			(void)error; dispatch_semaphore_signal(sem2);
		}];
		dispatch_semaphore_wait(sem2, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(5 * NSEC_PER_SEC)));
		g_out_w = outW; g_out_h = outH;
		*w = outW; *h = outH;
		pthread_mutex_unlock(&g_mu);
		return 0;
	}
	return -1;
}

void re_sck_stream_request_keyframe(void) {
	pthread_mutex_lock(&g_mu);
	g_force_key = 1;
	pthread_mutex_unlock(&g_mu);
}

int re_sck_stream_poll(re_sck_shot *out) {
	memset(out, 0, sizeof(*out));
	pthread_mutex_lock(&g_mu);
	if (!g_out || !g_out.ready || !g_out.frame) { pthread_mutex_unlock(&g_mu); out->err = -1; return -1; }
	out->data = g_out.frame;
	out->w = g_out.w;
	out->h = g_out.h;
	out->stride = g_out.stride;
	out->err = 0;
	g_out.frame = NULL;
	g_out.ready = 0;
	pthread_mutex_unlock(&g_mu);
	return 0;
}

int re_sck_stream_poll_nal(re_sck_nal *out) {
	memset(out, 0, sizeof(*out));
	pthread_mutex_lock(&g_mu);
	if (!g_out || !g_out.nalReady || !g_out.nal || g_out.nalLen <= 0) {
		pthread_mutex_unlock(&g_mu);
		out->err = -1;
		return -1;
	}
	out->data = g_out.nal;
	out->len = g_out.nalLen;
	out->keyframe = g_out.nalKey;
	out->w = g_out.w > 0 ? g_out.w : 0;
	out->h = g_out.h > 0 ? g_out.h : 0;
	// Prefer stream config dims from VT — stash on first encode via pixel buffer dims in callback.
	out->err = 0;
	g_out.nal = NULL;
	g_out.nalLen = 0;
	g_out.nalReady = 0;
	pthread_mutex_unlock(&g_mu);
	return 0;
}

int re_sck_stream_inline_hevc(void) {
	pthread_mutex_lock(&g_mu);
	int v = g_inline_hevc;
	pthread_mutex_unlock(&g_mu);
	return v;
}

void re_sck_stream_stop(void) {
	pthread_mutex_lock(&g_mu);
	SCStream *stream = g_stream;
	g_stream = nil;
	REStreamOut *out = g_out;
	g_out = nil;
	re_inline_vt_destroy_locked();
	pthread_mutex_unlock(&g_mu);

	if (stream) {
		dispatch_semaphore_t sem = dispatch_semaphore_create(0);
		[stream stopCaptureWithCompletionHandler:^(__unused NSError *e) {
			dispatch_semaphore_signal(sem);
		}];
		dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(3 * NSEC_PER_SEC)));
	}
	if (out) {
		pthread_mutex_lock(&g_mu);
		if (out.frame) { free(out.frame); out.frame = NULL; }
		if (out.nal) { free(out.nal); out.nal = NULL; }
		out.ready = 0;
		out.nalReady = 0;
		pthread_mutex_unlock(&g_mu);
	}
}
