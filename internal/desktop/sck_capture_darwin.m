// Continuous ScreenCaptureKit stream + one-shot helpers.
//go:build darwin

#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <CoreGraphics/CoreGraphics.h>
#import <CoreFoundation/CoreFoundation.h>
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
		NSUInteger idx = display_index < 0 ? 0 : (NSUInteger)display_index;
		if (idx >= contentHold.displays.count) idx = 0;
		SCDisplay *display = contentHold.displays[idx];
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

// ---- Continuous SCStream ----

@interface REStreamOut : NSObject <SCStreamOutput, SCStreamDelegate>
@property (atomic) unsigned char *frame;
@property (atomic) int w;
@property (atomic) int h;
@property (atomic) int stride;
@property (atomic) int ready;
@end

@implementation REStreamOut
- (void)stream:(SCStream *)stream didOutputSampleBuffer:(CMSampleBufferRef)sampleBuffer ofType:(SCStreamOutputType)type {
	if (type != SCStreamOutputTypeScreen) return;
	CVImageBufferRef imgBuf = CMSampleBufferGetImageBuffer(sampleBuffer);
	if (!imgBuf) return;
	CVPixelBufferLockBaseAddress(imgBuf, kCVPixelBufferLock_ReadOnly);
	size_t width = CVPixelBufferGetWidth(imgBuf);
	size_t height = CVPixelBufferGetHeight(imgBuf);
	size_t bpr = CVPixelBufferGetBytesPerRow(imgBuf);
	uint8_t *base = (uint8_t *)CVPixelBufferGetBaseAddress(imgBuf);
	size_t outBpr = width * 4;
	unsigned char *buf = (unsigned char *)malloc(outBpr * height);
	if (buf && base) {
		for (size_t y = 0; y < height; y++) {
			uint8_t *src = base + y * bpr;
			uint8_t *dst = buf + y * outBpr;
			for (size_t x = 0; x < width; x++) {
				// BGRA -> RGBA
				dst[x*4+0] = src[x*4+2];
				dst[x*4+1] = src[x*4+1];
				dst[x*4+2] = src[x*4+0];
				dst[x*4+3] = 255;
			}
		}
		unsigned char *old = self.frame;
		self.frame = buf; self.w = (int)width; self.h = (int)height; self.stride = (int)outBpr; self.ready = 1;
		if (old) free(old);
	}
	CVPixelBufferUnlockBaseAddress(imgBuf, kCVPixelBufferLock_ReadOnly);
}
@end

static SCStream *g_stream = nil;
static REStreamOut *g_out = nil;
static pthread_mutex_t g_mu = PTHREAD_MUTEX_INITIALIZER;

int re_sck_stream_start(int display_index, int show_cursor, int *w, int *h) {
	if (@available(macOS 14.0, *)) {
		pthread_mutex_lock(&g_mu);
		if (g_stream) {
			[g_stream stopCaptureWithCompletionHandler:^(__unused NSError *e){}];
			g_stream = nil;
		}
		dispatch_semaphore_t sem = dispatch_semaphore_create(0);
		__block SCShareableContent *contentHold = nil;
		[SCShareableContent getShareableContentWithCompletionHandler:^(SCShareableContent * _Nullable content, NSError * _Nullable error) {
			contentHold = content; dispatch_semaphore_signal(sem);
		}];
		dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(8 * NSEC_PER_SEC)));
		if (!contentHold || contentHold.displays.count == 0) { pthread_mutex_unlock(&g_mu); return -1; }
		NSUInteger idx = display_index < 0 ? 0 : (NSUInteger)display_index;
		if (idx >= contentHold.displays.count) idx = 0;
		SCDisplay *display = contentHold.displays[idx];
		SCContentFilter *filter = [[SCContentFilter alloc] initWithDisplay:display excludingWindows:@[]];
		SCStreamConfiguration *config = [SCStreamConfiguration new];
		config.width = display.width; config.height = display.height;
		config.minimumFrameInterval = CMTimeMake(1, 30);
		config.showsCursor = show_cursor ? YES : NO;
		config.pixelFormat = kCVPixelFormatType_32BGRA;
		config.queueDepth = 3;
		g_out = [REStreamOut new];
		g_stream = [[SCStream alloc] initWithFilter:filter configuration:config delegate:g_out];
		NSError *err = nil;
		BOOL ok = [g_stream addStreamOutput:g_out type:SCStreamOutputTypeScreen sampleHandlerQueue:dispatch_get_global_queue(QOS_CLASS_USER_INTERACTIVE, 0) error:&err];
		if (!ok) { g_stream = nil; pthread_mutex_unlock(&g_mu); return -2; }
		dispatch_semaphore_t sem2 = dispatch_semaphore_create(0);
		[g_stream startCaptureWithCompletionHandler:^(NSError * _Nullable error) {
			(void)error; dispatch_semaphore_signal(sem2);
		}];
		dispatch_semaphore_wait(sem2, dispatch_time(DISPATCH_TIME_NOW, (int64_t)(5 * NSEC_PER_SEC)));
		*w = (int)display.width; *h = (int)display.height;
		pthread_mutex_unlock(&g_mu);
		return 0;
	}
	return -1;
}

int re_sck_stream_poll(re_sck_shot *out) {
	memset(out, 0, sizeof(*out));
	pthread_mutex_lock(&g_mu);
	if (!g_out || !g_out.ready || !g_out.frame) { pthread_mutex_unlock(&g_mu); out->err = -1; return -1; }
	int w = g_out.w, h = g_out.h, str = g_out.stride;
	size_t n = (size_t)str * (size_t)h;
	unsigned char *copy = (unsigned char *)malloc(n);
	if (!copy) { pthread_mutex_unlock(&g_mu); out->err = -3; return -3; }
	memcpy(copy, g_out.frame, n);
	g_out.ready = 0;
	pthread_mutex_unlock(&g_mu);
	out->data = copy; out->w = w; out->h = h; out->stride = str; out->err = 0;
	return 0;
}

void re_sck_stream_stop(void) {
	pthread_mutex_lock(&g_mu);
	if (g_stream) {
		[g_stream stopCaptureWithCompletionHandler:^(__unused NSError *e){}];
		g_stream = nil;
	}
	if (g_out) {
		if (g_out.frame) free(g_out.frame);
		g_out = nil;
	}
	pthread_mutex_unlock(&g_mu);
}
