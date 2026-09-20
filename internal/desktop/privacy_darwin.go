//go:build darwin

package desktop

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework Foundation
#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>

static NSWindow *re_privacy_win = nil;

void re_privacy_show(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (re_privacy_win != nil) {
			[re_privacy_win orderFrontRegardless];
			return;
		}
		NSRect frame = NSZeroRect;
		for (NSScreen *s in [NSScreen screens]) {
			frame = NSUnionRect(frame, s.frame);
		}
		re_privacy_win = [[NSWindow alloc] initWithContentRect:frame
			styleMask:NSWindowStyleMaskBorderless
			backing:NSBackingStoreBuffered defer:NO];
		re_privacy_win.backgroundColor = [NSColor blackColor];
		re_privacy_win.level = NSScreenSaverWindowLevel;
		re_privacy_win.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces | NSWindowCollectionBehaviorStationary;
		re_privacy_win.sharingType = NSWindowSharingNone; // exclude from capture when possible
		re_privacy_win.ignoresMouseEvents = NO;
		[re_privacy_win setOpaque:YES];
		[re_privacy_win orderFrontRegardless];
	});
}

void re_privacy_hide(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (re_privacy_win != nil) {
			[re_privacy_win orderOut:nil];
			re_privacy_win = nil;
		}
	});
}
*/
import "C"

import "time"

func startPrivacyBlankOS() (func(), error) {
	C.re_privacy_show()
	return func() {
		C.re_privacy_hide()
		time.Sleep(50 * time.Millisecond)
	}, nil
}
