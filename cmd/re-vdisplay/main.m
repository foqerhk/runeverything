// re-vdisplay — retain CGVirtualDisplay objects for RunEverything Agent.
// Private CoreGraphics API via runtime-declared interfaces; line-delimited JSON on stdin/stdout.
//
// Requests (one JSON object per line):
//   {"cmd":"ping"}
//   {"cmd":"create","mode":"5k"|"8k"|"16k"}
//   {"cmd":"list"}
//   {"cmd":"destroy","display_id":123}
//   {"cmd":"destroy_all"}
//   {"cmd":"quit"}
//
// Responses: {"ok":true,...} or {"ok":false,"error":"..."}

#import <Foundation/Foundation.h>
#import <CoreGraphics/CoreGraphics.h>
#import <dispatch/dispatch.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#pragma mark - Private CGVirtualDisplay API (may break on OS updates)

@interface CGVirtualDisplayDescriptor : NSObject
@property(nonatomic, assign) CGSize sizeInMillimeters;
@property(nonatomic, assign) NSUInteger maxPixelsWide;
@property(nonatomic, assign) NSUInteger maxPixelsHigh;
@property(nonatomic, assign) CGPoint whitePoint;
@property(nonatomic, assign) CGPoint redPrimary;
@property(nonatomic, assign) CGPoint greenPrimary;
@property(nonatomic, assign) CGPoint bluePrimary;
@property(nonatomic, strong) dispatch_queue_t queue;
@property(nonatomic, copy) NSString *name;
@property(nonatomic, assign) uint32_t vendorID;
@property(nonatomic, assign) uint32_t productID;
@property(nonatomic, assign) uint32_t serialNum;
@end

@interface CGVirtualDisplayMode : NSObject
- (instancetype)initWithWidth:(NSUInteger)width
                       height:(NSUInteger)height
                  refreshRate:(double)refreshRate;
@property(nonatomic, readonly) NSUInteger width;
@property(nonatomic, readonly) NSUInteger height;
@property(nonatomic, readonly) double refreshRate;
@end

@interface CGVirtualDisplaySettings : NSObject
@property(nonatomic, strong) NSArray<CGVirtualDisplayMode *> *modes;
@property(nonatomic, assign) NSUInteger hiDPI;
@end

@interface CGVirtualDisplay : NSObject
- (instancetype)initWithDescriptor:(CGVirtualDisplayDescriptor *)descriptor;
- (BOOL)applySettings:(CGVirtualDisplaySettings *)settings;
@property(nonatomic, readonly) CGDirectDisplayID displayID;
@end

#pragma mark -

@interface REVirtualSlot : NSObject
@property(nonatomic, strong) CGVirtualDisplay *display;
@property(nonatomic, copy) NSString *mode;
@property(nonatomic, assign) CGDirectDisplayID displayID;
@property(nonatomic, assign) NSUInteger fbW;
@property(nonatomic, assign) NSUInteger fbH;
@property(nonatomic, assign) NSUInteger logicalW;
@property(nonatomic, assign) NSUInteger logicalH;
@end
@implementation REVirtualSlot
@end

static NSMutableDictionary<NSNumber *, REVirtualSlot *> *g_slots;
static BOOL g_apiOK;

typedef struct {
	const char *mode;
	NSUInteger maxW, maxH;
	NSUInteger logicalW, logicalH;
} REProfile;

static const REProfile kProfiles[] = {
	{"5k", 5120, 2880, 2560, 1440},
	{"8k", 7680, 4320, 3840, 2160},
	{"16k", 15360, 8640, 7680, 4320},
};

static const REProfile *profileForMode(NSString *mode) {
	const char *m = mode.UTF8String ?: "";
	for (size_t i = 0; i < sizeof(kProfiles) / sizeof(kProfiles[0]); i++) {
		if (strcasecmp(m, kProfiles[i].mode) == 0) return &kProfiles[i];
	}
	return NULL;
}

static void writeJSON(NSDictionary *obj) {
	NSError *err = nil;
	NSData *data = [NSJSONSerialization dataWithJSONObject:obj options:0 error:&err];
	if (!data) {
		fprintf(stdout, "{\"ok\":false,\"error\":\"json_encode\"}\n");
		fflush(stdout);
		return;
	}
	fwrite(data.bytes, 1, data.length, stdout);
	fputc('\n', stdout);
	fflush(stdout);
}

static BOOL probeAPI(void) {
	Class c1 = NSClassFromString(@"CGVirtualDisplay");
	Class c2 = NSClassFromString(@"CGVirtualDisplayDescriptor");
	Class c3 = NSClassFromString(@"CGVirtualDisplaySettings");
	Class c4 = NSClassFromString(@"CGVirtualDisplayMode");
	return c1 && c2 && c3 && c4;
}

static REVirtualSlot *createVirtual(const REProfile *p, NSError **outErr) {
	if (!probeAPI()) {
		if (outErr)
			*outErr = [NSError errorWithDomain:@"re-vdisplay" code:1
				userInfo:@{NSLocalizedDescriptionKey : @"CGVirtualDisplay API unavailable"}];
		return nil;
	}

	CGVirtualDisplayDescriptor *desc = [[CGVirtualDisplayDescriptor alloc] init];
	if (!desc) {
		if (outErr)
			*outErr = [NSError errorWithDomain:@"re-vdisplay" code:2
				userInfo:@{NSLocalizedDescriptionKey : @"descriptor init failed"}];
		return nil;
	}

	desc.name = [NSString stringWithFormat:@"RunEverything %s", p->mode];
	desc.maxPixelsWide = p->maxW;
	desc.maxPixelsHigh = p->maxH;
	desc.sizeInMillimeters = CGSizeMake(25.4 * (double)p->maxW / 110.0, 25.4 * (double)p->maxH / 110.0);
	desc.vendorID = 0x5245; // 'RE'
	desc.productID = 0x5644; // 'VD'
	desc.serialNum = arc4random();
	desc.redPrimary = CGPointMake(0.640, 0.330);
	desc.greenPrimary = CGPointMake(0.300, 0.600);
	desc.bluePrimary = CGPointMake(0.150, 0.060);
	desc.whitePoint = CGPointMake(0.3127, 0.3290);
	desc.queue = dispatch_get_global_queue(QOS_CLASS_USER_INITIATED, 0);

	CGVirtualDisplay *display = [[CGVirtualDisplay alloc] initWithDescriptor:desc];
	if (!display) {
		if (outErr)
			*outErr = [NSError errorWithDomain:@"re-vdisplay" code:3
				userInfo:@{NSLocalizedDescriptionKey : @"CGVirtualDisplay init failed"}];
		return nil;
	}

	CGVirtualDisplaySettings *settings = [[CGVirtualDisplaySettings alloc] init];
	settings.hiDPI = 1;
	CGVirtualDisplayMode *mode60 =
		[[CGVirtualDisplayMode alloc] initWithWidth:p->logicalW height:p->logicalH refreshRate:60.0];
	CGVirtualDisplayMode *mode30 =
		[[CGVirtualDisplayMode alloc] initWithWidth:p->logicalW height:p->logicalH refreshRate:30.0];
	NSMutableArray *modes = [NSMutableArray array];
	if (mode60) [modes addObject:mode60];
	if (mode30) [modes addObject:mode30];
	settings.modes = modes;

	if (![display applySettings:settings]) {
		if (outErr)
			*outErr = [NSError errorWithDomain:@"re-vdisplay" code:5
				userInfo:@{NSLocalizedDescriptionKey : @"applySettings returned NO"}];
		return nil;
	}

	CGDirectDisplayID did = display.displayID;
	if (did == 0 || did == kCGNullDirectDisplay) {
		if (outErr)
			*outErr = [NSError errorWithDomain:@"re-vdisplay" code:6
				userInfo:@{NSLocalizedDescriptionKey : @"null displayID after apply"}];
		return nil;
	}

	REVirtualSlot *slot = [REVirtualSlot new];
	slot.display = display;
	slot.mode = [NSString stringWithUTF8String:p->mode];
	slot.displayID = did;
	slot.fbW = p->maxW;
	slot.fbH = p->maxH;
	slot.logicalW = p->logicalW;
	slot.logicalH = p->logicalH;
	return slot;
}

static NSDictionary *slotToDict(REVirtualSlot *s) {
	return @{
		@"display_id" : @(s.displayID),
		@"mode" : s.mode ?: @"",
		@"width" : @(s.logicalW),
		@"height" : @(s.logicalH),
		@"fb_width" : @(s.fbW),
		@"fb_height" : @(s.fbH),
	};
}

static void handleLine(NSString *line) {
	NSData *data = [line dataUsingEncoding:NSUTF8StringEncoding];
	if (!data.length) return;
	NSError *err = nil;
	id obj = [NSJSONSerialization JSONObjectWithData:data options:0 error:&err];
	if (![obj isKindOfClass:[NSDictionary class]]) {
		writeJSON(@{@"ok" : @NO, @"error" : @"invalid_json"});
		return;
	}
	NSDictionary *req = (NSDictionary *)obj;
	NSString *cmd = [[req[@"cmd"] description] lowercaseString];
	if (!cmd.length) {
		writeJSON(@{@"ok" : @NO, @"error" : @"missing_cmd"});
		return;
	}

	if ([cmd isEqualToString:@"ping"]) {
		writeJSON(@{@"ok" : @YES, @"api" : @(g_apiOK), @"pid" : @(getpid())});
		return;
	}
	if ([cmd isEqualToString:@"quit"]) {
		writeJSON(@{@"ok" : @YES});
		exit(0);
	}
	if (!g_apiOK) {
		writeJSON(@{@"ok" : @NO, @"error" : @"CGVirtualDisplay API unavailable on this macOS"});
		return;
	}

	if ([cmd isEqualToString:@"list"]) {
		NSMutableArray *arr = [NSMutableArray array];
		for (REVirtualSlot *s in g_slots.allValues) {
			[arr addObject:slotToDict(s)];
		}
		writeJSON(@{@"ok" : @YES, @"displays" : arr});
		return;
	}

	if ([cmd isEqualToString:@"destroy_all"]) {
		[g_slots removeAllObjects];
		writeJSON(@{@"ok" : @YES});
		return;
	}

	if ([cmd isEqualToString:@"destroy"]) {
		NSNumber *did = req[@"display_id"];
		if (!did) {
			writeJSON(@{@"ok" : @NO, @"error" : @"missing_display_id"});
			return;
		}
		uint32_t idv = (uint32_t)did.unsignedIntValue;
		[g_slots removeObjectForKey:@(idv)];
		writeJSON(@{@"ok" : @YES, @"display_id" : @(idv)});
		return;
	}

	if ([cmd isEqualToString:@"create"]) {
		NSString *mode = [[req[@"mode"] description] lowercaseString];
		const REProfile *p = profileForMode(mode);
		if (!p) {
			writeJSON(@{@"ok" : @NO, @"error" : @"unknown_mode (use 5k|8k|16k)"});
			return;
		}
		for (REVirtualSlot *s in g_slots.allValues) {
			if ([s.mode isEqualToString:mode]) {
				NSMutableDictionary *resp = [slotToDict(s) mutableCopy];
				resp[@"ok"] = @YES;
				resp[@"reused"] = @YES;
				writeJSON(resp);
				return;
			}
		}
		NSError *cerr = nil;
		REVirtualSlot *slot = createVirtual(p, &cerr);
		if (!slot) {
			writeJSON(@{@"ok" : @NO, @"error" : cerr.localizedDescription ?: @"create_failed"});
			return;
		}
		g_slots[@(slot.displayID)] = slot;
		NSMutableDictionary *resp = [slotToDict(slot) mutableCopy];
		resp[@"ok"] = @YES;
		resp[@"reused"] = @NO;
		writeJSON(resp);
		return;
	}

	writeJSON(@{@"ok" : @NO, @"error" : [NSString stringWithFormat:@"unknown_cmd:%@", cmd]});
}

int main(int argc, const char *argv[]) {
	@autoreleasepool {
		g_slots = [NSMutableDictionary dictionary];
		g_apiOK = probeAPI();

		if (argc >= 3 && strcmp(argv[1], "create") == 0) {
			NSString *mode = [NSString stringWithUTF8String:argv[2]];
			handleLine([NSString stringWithFormat:@"{\"cmd\":\"create\",\"mode\":\"%@\"}", mode]);
			if (g_slots.count == 0) return 1;
			dispatch_main();
			return 0;
		}

		char *line = NULL;
		size_t cap = 0;
		while (1) {
			@autoreleasepool {
				ssize_t n = getline(&line, &cap, stdin);
				if (n < 0) break;
				while (n > 0 && (line[n - 1] == '\n' || line[n - 1] == '\r')) n--;
				line[n] = '\0';
				if (n == 0) continue;
				NSString *s = [NSString stringWithUTF8String:line];
				if (!s) {
					writeJSON(@{@"ok" : @NO, @"error" : @"utf8"});
					continue;
				}
				handleLine(s);
			}
		}
		free(line);
	}
	return 0;
}
