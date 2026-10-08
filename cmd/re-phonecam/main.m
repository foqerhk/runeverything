// re-phonecam — publish phone frames as a host webcam (+ optional preview).
//
// Protocol on stdin (binary):
//   'P''C''A''M' | u32le width | u32le height | u32le jpegLen | jpegBytes
//   'P''C''A''H' | u32le width|keybit | u32le height | u32le annexBLen | annexB
//       width high bit (1<<31) marks H.264 keyframe / IDR
//   len==0 → quit
//
// stdout lines:
//   READY <device name>
//   ERROR <message>
//
// Backends (first that works):
//   1) AkVCamManager (Webcamoid / AkVirtualCamera) if installed
//   2) Preview NSWindow (unless --no-preview)

#import <AppKit/AppKit.h>
#import <CoreGraphics/CoreGraphics.h>
#import <CoreMedia/CoreMedia.h>
#import <CoreVideo/CoreVideo.h>
#import <Foundation/Foundation.h>
#import <ImageIO/ImageIO.h>
#import <VideoToolbox/VideoToolbox.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

// Fixed AkVCam output canvas (must match `stream … WIDTH HEIGHT`).
static int g_outW = 1280;
static int g_outH = 720;
static int g_fps = 15;
static int g_noPreview = 0;

@interface REPhoneCamH264Decoder : NSObject
- (CGImageRef)decodeAnnexB:(NSData *)annexB isKey:(BOOL)isKey CF_RETURNS_RETAINED;
- (void)invalidate;
@end

@implementation REPhoneCamH264Decoder {
	CMVideoFormatDescriptionRef _fmt;
	VTDecompressionSessionRef _session;
	NSData *_sps;
	NSData *_pps;
	BOOL _gotKey;
	int64_t _pts;
}

- (void)dealloc {
	[self invalidate];
}

- (void)invalidate {
	if (_session) {
		VTDecompressionSessionInvalidate(_session);
		CFRelease(_session);
		_session = NULL;
	}
	if (_fmt) {
		CFRelease(_fmt);
		_fmt = NULL;
	}
	_sps = nil;
	_pps = nil;
	_gotKey = NO;
}

static NSArray<NSData *> *SplitAnnexB(NSData *data) {
	const uint8_t *b = data.bytes;
	NSUInteger n = data.length;
	NSMutableArray<NSData *> *out = [NSMutableArray array];
	NSUInteger i = 0;
	while (i + 3 < n) {
		NSUInteger start = NSNotFound;
		NSUInteger sc = 0;
		if (i + 3 < n && b[i] == 0 && b[i + 1] == 0 && b[i + 2] == 0 && b[i + 3] == 1) {
			start = i + 4;
			sc = 4;
		} else if (b[i] == 0 && b[i + 1] == 0 && b[i + 2] == 1) {
			start = i + 3;
			sc = 3;
		}
		if (start == NSNotFound) {
			i++;
			continue;
		}
		NSUInteger j = start;
		while (j + 3 < n) {
			if ((b[j] == 0 && b[j + 1] == 0 && b[j + 2] == 0 && b[j + 3] == 1) ||
			    (b[j] == 0 && b[j + 1] == 0 && b[j + 2] == 1)) {
				break;
			}
			j++;
		}
		if (j + 3 >= n) {
			j = n;
		}
		if (j > start) {
			[out addObject:[NSData dataWithBytes:b + start length:j - start]];
		}
		i = j;
		(void)sc;
	}
	return out;
}

- (BOOL)ensureSession {
	if (_session && _fmt) {
		return YES;
	}
	if (_sps.length == 0 || _pps.length == 0) {
		return NO;
	}
	const uint8_t *sets[2] = {_sps.bytes, _pps.bytes};
	size_t sizes[2] = {_sps.length, _pps.length};
	CMVideoFormatDescriptionRef fmt = NULL;
	OSStatus st = CMVideoFormatDescriptionCreateFromH264ParameterSets(
	    kCFAllocatorDefault, 2, sets, sizes, 4, &fmt);
	if (st != noErr || !fmt) {
		return NO;
	}
	if (_fmt) {
		CFRelease(_fmt);
	}
	_fmt = fmt;

	if (_session) {
		VTDecompressionSessionInvalidate(_session);
		CFRelease(_session);
		_session = NULL;
	}
	VTDecompressionOutputCallbackRecord cb = {0};
	NSDictionary *dst = @{
		(id)kCVPixelBufferPixelFormatTypeKey : @(kCVPixelFormatType_32BGRA),
		(id)kCVPixelBufferIOSurfacePropertiesKey : @{},
	};
	st = VTDecompressionSessionCreate(
	    kCFAllocatorDefault, _fmt, NULL, (__bridge CFDictionaryRef)dst, &cb, &_session);
	if (st != noErr || !_session) {
		fprintf(stderr, "re-phonecam: VTDecompressionSessionCreate failed %d\n", (int)st);
		return NO;
	}
	fprintf(stderr, "re-phonecam: H264 decoder ready\n");
	return YES;
}

- (CGImageRef)decodeAnnexB:(NSData *)annexB isKey:(BOOL)isKey CF_RETURNS_RETAINED {
	if (annexB.length == 0) {
		return NULL;
	}
	NSArray<NSData *> *nals = SplitAnnexB(annexB);
	NSMutableData *avcc = [NSMutableData data];
	BOOL sawSlice = NO;
	BOOL sawIDR = NO;
	for (NSData *nal in nals) {
		if (nal.length == 0) {
			continue;
		}
		uint8_t nt = ((const uint8_t *)nal.bytes)[0] & 0x1F;
		if (nt == 7) {
			_sps = nal;
			[self invalidateSessionOnly];
		} else if (nt == 8) {
			_pps = nal;
			[self invalidateSessionOnly];
		} else if (nt == 5 || nt == 1) {
			sawSlice = YES;
			if (nt == 5) {
				sawIDR = YES;
			}
			uint32_t len = OSSwapHostToBigInt32((uint32_t)nal.length);
			[avcc appendBytes:&len length:4];
			[avcc appendData:nal];
		}
	}
	if (!sawSlice) {
		return NULL;
	}
	if (isKey || sawIDR) {
		_gotKey = YES;
	}
	if (!_gotKey) {
		return NULL; // wait for first IDR
	}
	if (![self ensureSession]) {
		return NULL;
	}

	// Own the AVCC bytes — VT may finish after the NSData stack frame returns.
	void *owned = malloc(avcc.length);
	if (!owned) {
		return NULL;
	}
	memcpy(owned, avcc.bytes, avcc.length);
	CMBlockBufferRef block = NULL;
	OSStatus st = CMBlockBufferCreateWithMemoryBlock(
	    kCFAllocatorDefault, owned, avcc.length, kCFAllocatorMalloc, NULL, 0,
	    avcc.length, 0, &block);
	if (st != noErr || !block) {
		free(owned);
		return NULL;
	}
	CMSampleBufferRef sample = NULL;
	size_t sampleSize = avcc.length;
	CMSampleTimingInfo timing = {
	    .duration = kCMTimeInvalid,
	    .presentationTimeStamp = CMTimeMake(_pts++, 30),
	    .decodeTimeStamp = kCMTimeInvalid,
	};
	st = CMSampleBufferCreate(
	    kCFAllocatorDefault, block, true, NULL, NULL, _fmt, 1, 1, &timing, 1, &sampleSize,
	    &sample);
	CFRelease(block);
	if (st != noErr || !sample) {
		return NULL;
	}

	__block CVPixelBufferRef outPB = NULL;
	__block OSStatus decSt = noErr;
	dispatch_semaphore_t sem = dispatch_semaphore_create(0);
	VTDecodeFrameFlags flags = 0;
	VTDecodeInfoFlags info = 0;
	st = VTDecompressionSessionDecodeFrameWithOutputHandler(
	    _session, sample, flags, &info,
	    ^(OSStatus status, VTDecodeInfoFlags infoFlags, CVImageBufferRef imageBuffer,
	      CMTime presentationTimeStamp, CMTime presentationDuration) {
		    (void)infoFlags;
		    (void)presentationTimeStamp;
		    (void)presentationDuration;
		    decSt = status;
		    if (status == noErr && imageBuffer) {
			    outPB = (CVPixelBufferRef)CFRetain(imageBuffer);
		    }
		    dispatch_semaphore_signal(sem);
	    });
	CFRelease(sample);
	if (st != noErr) {
		static int once;
		if (once++ < 5) {
			fprintf(stderr, "re-phonecam: VT decode submit failed %d key=%d\n", (int)st, (int)isKey);
		}
		return NULL;
	}
	if (dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 500 * NSEC_PER_MSEC)) != 0) {
		static int once;
		if (once++ < 5) {
			fprintf(stderr, "re-phonecam: VT decode timeout key=%d\n", (int)isKey);
		}
		return NULL;
	}
	if (!outPB) {
		static int once;
		if (once++ < 5) {
			fprintf(stderr, "re-phonecam: VT decode empty status=%d key=%d bytes=%zu\n",
			        (int)decSt, (int)isKey, (size_t)annexB.length);
		}
		return NULL;
	}

	CGImageRef cg = [self cgImageFromPixelBuffer:outPB];
	CFRelease(outPB);
	return cg;
}

- (void)invalidateSessionOnly {
	if (_session) {
		VTDecompressionSessionInvalidate(_session);
		CFRelease(_session);
		_session = NULL;
	}
	if (_fmt) {
		CFRelease(_fmt);
		_fmt = NULL;
	}
}

- (CGImageRef)cgImageFromPixelBuffer:(CVPixelBufferRef)pb CF_RETURNS_RETAINED {
	size_t w = CVPixelBufferGetWidth(pb);
	size_t h = CVPixelBufferGetHeight(pb);
	CVPixelBufferLockBaseAddress(pb, kCVPixelBufferLock_ReadOnly);
	void *base = CVPixelBufferGetBaseAddress(pb);
	size_t bpr = CVPixelBufferGetBytesPerRow(pb);
	CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
	CGContextRef ctx = CGBitmapContextCreate(
	    base, w, h, 8, bpr, cs,
	    kCGImageAlphaPremultipliedFirst | kCGBitmapByteOrder32Little);
	CGColorSpaceRelease(cs);
	CGImageRef img = ctx ? CGBitmapContextCreateImage(ctx) : NULL;
	if (ctx) {
		CGContextRelease(ctx);
	}
	CVPixelBufferUnlockBaseAddress(pb, kCVPixelBufferLock_ReadOnly);
	return img;
}

@end

@interface REPhoneCamApp : NSObject <NSApplicationDelegate>
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, strong) NSImageView *imageView;
@property(nonatomic, strong) NSTask *akTask;
@property(nonatomic, strong) NSFileHandle *akStdin;
@property(nonatomic, copy) NSString *deviceName;
@property(nonatomic, assign) BOOL readySent;
@property(nonatomic, strong) REPhoneCamH264Decoder *h264;
@end

@implementation REPhoneCamApp

- (void)applicationDidFinishLaunching:(NSNotification *)n {
	self.deviceName = @"KoKo Phone Camera";
	self.h264 = [REPhoneCamH264Decoder new];
	if (!g_noPreview) {
		[self setupWindow];
	}
	[self tryStartAkVCam];
	[self sendReady];
	dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0), ^{
		[self readLoop];
	});
}

- (void)setupWindow {
	NSRect rect = NSMakeRect(80, 80, MIN(g_outW, 960), MIN(g_outH, 540) + 28);
	self.window = [[NSWindow alloc] initWithContentRect:rect
	                                          styleMask:(NSWindowStyleMaskTitled |
	                                                     NSWindowStyleMaskClosable |
	                                                     NSWindowStyleMaskMiniaturizable |
	                                                     NSWindowStyleMaskResizable)
	                                            backing:NSBackingStoreBuffered
	                                              defer:NO];
	self.window.title = @"KoKo Phone Camera · 手机摄像头";
	self.window.level = NSFloatingWindowLevel;
	self.window.releasedWhenClosed = NO;
	self.imageView = [[NSImageView alloc] initWithFrame:self.window.contentView.bounds];
	self.imageView.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
	self.imageView.imageScaling = NSImageScaleProportionallyUpOrDown;
	self.imageView.wantsLayer = YES;
	self.imageView.layer.backgroundColor = NSColor.blackColor.CGColor;
	[self.window.contentView addSubview:self.imageView];
	[self.window makeKeyAndOrderFront:nil];
	[NSApp activateIgnoringOtherApps:YES];
}

- (NSString *)findAkVCamManager {
	NSArray *cands = @[
		@"/Applications/AkVirtualCamera/AkVirtualCamera.plugin/Contents/Resources/AkVCamManager",
		@"/usr/local/bin/AkVCamManager",
		@"/opt/homebrew/bin/AkVCamManager",
		[NSHomeDirectory() stringByAppendingPathComponent:
		    @"Applications/AkVirtualCamera/AkVirtualCamera.plugin/Contents/Resources/AkVCamManager"],
	];
	for (NSString *p in cands) {
		if ([[NSFileManager defaultManager] isExecutableFileAtPath:p]) {
			return p;
		}
	}
	return nil;
}

- (void)tryStartAkVCam {
	NSString *mgr = [self findAkVCamManager];
	if (!mgr) {
		fprintf(stderr, "re-phonecam: AkVCamManager not found — preview window only "
		                "(install AkVirtualCamera for Zoom/Meet device)\n");
		return;
	}
	NSTask *add = [[NSTask alloc] init];
	add.launchPath = mgr;
	add.arguments = @[ @"add-device", @"-i", @"KoKoPhoneCam0", @"KoKo Phone Camera" ];
	add.standardOutput = [NSPipe pipe];
	add.standardError = [NSPipe pipe];
	@try {
		[add launch];
		[add waitUntilExit];
	} @catch (NSException *ex) {
		fprintf(stderr, "re-phonecam: add-device failed: %s\n", ex.reason.UTF8String);
		return;
	}
	NSTask *fmt = [[NSTask alloc] init];
	fmt.launchPath = mgr;
	fmt.arguments = @[
		@"add-format", @"KoKoPhoneCam0",
		@"RGB32", [NSString stringWithFormat:@"%d", g_outW],
		[NSString stringWithFormat:@"%d", g_outH],
		[NSString stringWithFormat:@"%d", MAX(g_fps, 1)]
	];
	fmt.standardOutput = [NSPipe pipe];
	fmt.standardError = [NSPipe pipe];
	@try {
		[fmt launch];
		[fmt waitUntilExit];
	} @catch (NSException *ex) {
		fprintf(stderr, "re-phonecam: add-format failed: %s\n", ex.reason.UTF8String);
	}
	NSTask *upd = [[NSTask alloc] init];
	upd.launchPath = mgr;
	upd.arguments = @[ @"update" ];
	upd.standardOutput = [NSPipe pipe];
	upd.standardError = [NSPipe pipe];
	@try {
		[upd launch];
		[upd waitUntilExit];
	} @catch (NSException *ex) {
	}

	NSPipe *inPipe = [NSPipe pipe];
	self.akTask = [[NSTask alloc] init];
	self.akTask.launchPath = mgr;
	self.akTask.arguments = @[
		@"stream", @"KoKoPhoneCam0", @"RGB32",
		[NSString stringWithFormat:@"%d", g_outW],
		[NSString stringWithFormat:@"%d", g_outH],
		@"-f", [NSString stringWithFormat:@"%d", MAX(g_fps, 1)]
	];
	self.akTask.standardInput = inPipe;
	self.akTask.standardOutput = [NSPipe pipe];
	self.akTask.standardError = [NSPipe pipe];
	@try {
		[self.akTask launch];
		self.akStdin = inPipe.fileHandleForWriting;
		self.deviceName = @"KoKo Phone Camera";
		fprintf(stderr, "re-phonecam: streaming RGB32 %dx%d@%d into AkVirtualCamera\n",
		        g_outW, g_outH, g_fps);
	} @catch (NSException *ex) {
		fprintf(stderr, "re-phonecam: AkVCam stream failed: %s\n", ex.reason.UTF8String);
		self.akTask = nil;
		self.akStdin = nil;
	}
}

- (void)sendReady {
	if (self.readySent) {
		return;
	}
	self.readySent = YES;
	printf("READY %s\n", self.deviceName.UTF8String);
	fflush(stdout);
}

- (BOOL)readFully:(void *)buf length:(size_t)len {
	uint8_t *p = buf;
	size_t left = len;
	while (left > 0) {
		ssize_t n = read(STDIN_FILENO, p, left);
		if (n <= 0) {
			return NO;
		}
		p += n;
		left -= (size_t)n;
	}
	return YES;
}

- (void)readLoop {
	for (;;) {
		uint8_t hdr[16];
		if (![self readFully:hdr length:16]) {
			break;
		}
		BOOL isH264 = (memcmp(hdr, "PCAH", 4) == 0);
		BOOL isJPEG = (memcmp(hdr, "PCAM", 4) == 0);
		if (!isH264 && !isJPEG) {
			fprintf(stderr, "re-phonecam: bad magic\n");
			break;
		}
		uint32_t wRaw, h, payloadLen;
		memcpy(&wRaw, hdr + 4, 4);
		memcpy(&h, hdr + 8, 4);
		memcpy(&payloadLen, hdr + 12, 4);
		BOOL key = NO;
		uint32_t w = wRaw;
		if (isH264) {
			key = (wRaw & (1u << 31)) != 0;
			w = wRaw & 0x7fffffffu;
		}
		(void)w;
		(void)h;
		if (payloadLen == 0) {
			break;
		}
		if (payloadLen > 16 * 1024 * 1024) {
			fprintf(stderr, "re-phonecam: frame too large %u\n", payloadLen);
			break;
		}
		NSMutableData *payload = [NSMutableData dataWithLength:payloadLen];
		if (![self readFully:payload.mutableBytes length:payloadLen]) {
			break;
		}
		if (isH264) {
			[self handleH264:payload isKey:key];
		} else {
			[self handleJPEG:payload];
		}
	}
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp terminate:nil];
	});
}

- (void)handleH264:(NSData *)annexB isKey:(BOOL)isKey {
	static int seen;
	if (seen++ < 8) {
		fprintf(stderr, "re-phonecam: PCAH frame bytes=%zu key=%d\n",
		        (size_t)annexB.length, (int)isKey);
	}
	CGImageRef cg = [self.h264 decodeAnnexB:annexB isKey:isKey];
	if (!cg) {
		return;
	}
	[self publishCGImage:cg];
	CGImageRelease(cg);
}

- (void)handleJPEG:(NSData *)jpeg {
	CGImageSourceRef src = CGImageSourceCreateWithData((__bridge CFDataRef)jpeg, NULL);
	if (!src) {
		return;
	}
	CGImageRef cg = CGImageSourceCreateImageAtIndex(src, 0, NULL);
	CFRelease(src);
	if (!cg) {
		return;
	}
	[self publishCGImage:cg];
	CGImageRelease(cg);
}

- (void)publishCGImage:(CGImageRef)cg {
	if (self.imageView) {
		NSImage *img = [[NSImage alloc] initWithCGImage:cg size:NSZeroSize];
		dispatch_async(dispatch_get_main_queue(), ^{
			self.imageView.image = img;
		});
	}
	if (self.akStdin) {
		NSData *rgb = [self rgb32LetterboxFromCGImage:cg width:g_outW height:g_outH];
		if (rgb.length > 0) {
			@try {
				[self.akStdin writeData:rgb];
			} @catch (NSException *ex) {
				fprintf(stderr, "re-phonecam: ak write failed: %s\n", ex.reason.UTF8String);
				self.akStdin = nil;
			}
		}
	}
}

// AkVCam "RGB32" is PixelFormat_argb / kCMPixelFormat_32ARGB, but on LE macOS the
// in-memory layout expected by AkVCam's RGB32 struct is B,G,R,X (not A,R,G,B).
// Aspect-fit letterbox onto a black canvas of tw×th.
- (NSData *)rgb32LetterboxFromCGImage:(CGImageRef)image width:(int)tw height:(int)th {
	if (tw <= 0 || th <= 0 || !image) {
		return nil;
	}
	size_t bpr = (size_t)tw * 4;
	NSMutableData *bgra = [NSMutableData dataWithLength:bpr * (size_t)th];
	CGColorSpaceRef cs = CGColorSpaceCreateDeviceRGB();
	CGContextRef ctx = CGBitmapContextCreate(
	    bgra.mutableBytes, tw, th, 8, bpr, cs,
	    kCGImageAlphaPremultipliedFirst | kCGBitmapByteOrder32Little);
	CGColorSpaceRelease(cs);
	if (!ctx) {
		return nil;
	}
	CGContextSetRGBFillColor(ctx, 0, 0, 0, 1);
	CGContextFillRect(ctx, CGRectMake(0, 0, tw, th));
	size_t iw = CGImageGetWidth(image);
	size_t ih = CGImageGetHeight(image);
	if (iw > 0 && ih > 0) {
		CGFloat scale = MIN((CGFloat)tw / (CGFloat)iw, (CGFloat)th / (CGFloat)ih);
		CGFloat dw = (CGFloat)iw * scale;
		CGFloat dh = (CGFloat)ih * scale;
		CGFloat dx = ((CGFloat)tw - dw) * 0.5;
		CGFloat dy = ((CGFloat)th - dh) * 0.5;
		CGContextSetInterpolationQuality(ctx, kCGInterpolationMedium);
		CGContextDrawImage(ctx, CGRectMake(dx, dy, dw, dh), image);
	}
	CGContextRelease(ctx);
	return bgra;
}

- (void)applicationWillTerminate:(NSNotification *)n {
	[self.h264 invalidate];
	if (self.akStdin) {
		@try {
			[self.akStdin closeFile];
		} @catch (NSException *ex) {
		}
		self.akStdin = nil;
	}
	if (self.akTask && self.akTask.isRunning) {
		[self.akTask terminate];
	}
}

@end

static void usage(const char *argv0) {
	fprintf(stderr, "usage: %s [--width N] [--height N] [--fps N] [--no-preview]\n", argv0);
}

int main(int argc, const char *argv[]) {
	for (int i = 1; i < argc; i++) {
		if (strcmp(argv[i], "--width") == 0 && i + 1 < argc) {
			g_outW = atoi(argv[++i]);
		} else if (strcmp(argv[i], "--height") == 0 && i + 1 < argc) {
			g_outH = atoi(argv[++i]);
		} else if (strcmp(argv[i], "--fps") == 0 && i + 1 < argc) {
			g_fps = atoi(argv[++i]);
		} else if (strcmp(argv[i], "--no-preview") == 0) {
			g_noPreview = 1;
		} else if (strcmp(argv[i], "--help") == 0) {
			usage(argv[0]);
			return 0;
		}
	}
	if (g_outW <= 0) {
		g_outW = 1280;
	}
	if (g_outH <= 0) {
		g_outH = 720;
	}
	if (g_fps <= 0) {
		g_fps = 15;
	}

	@autoreleasepool {
		[NSApplication sharedApplication];
		REPhoneCamApp *app = [REPhoneCamApp new];
		NSApp.delegate = app;
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		[NSApp run];
	}
	return 0;
}
