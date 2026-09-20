//go:build darwin

package desktop

/*
#cgo LDFLAGS: -framework AppKit
#cgo CFLAGS: -x objective-c -fobjc-arc
#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>

static void re_clip_write(const char *s) {
	@autoreleasepool {
		NSString *str = [NSString stringWithUTF8String:s];
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		[pb clearContents];
		[pb setString:str forType:NSPasteboardTypeString];
	}
}

static char *re_clip_read(void) {
	@autoreleasepool {
		NSString *str = [[NSPasteboard generalPasteboard] stringForType:NSPasteboardTypeString];
		if (!str) return NULL;
		const char *u = [str UTF8String];
		if (!u) return NULL;
		return strdup(u);
	}
}

static int re_clip_write_png(const unsigned char *data, int len) {
	@autoreleasepool {
		if (!data || len <= 0) return -1;
		NSData *d = [NSData dataWithBytes:data length:(NSUInteger)len];
		NSImage *img = [[NSImage alloc] initWithData:d];
		if (!img) return -2;
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		[pb clearContents];
		[pb writeObjects:@[img]];
		return 0;
	}
}

static unsigned char *re_clip_read_png(int *out_len) {
	@autoreleasepool {
		*out_len = 0;
		NSPasteboard *pb = [NSPasteboard generalPasteboard];
		NSData *tiff = [pb dataForType:NSPasteboardTypeTIFF];
		NSData *png = [pb dataForType:NSPasteboardTypePNG];
		NSData *src = png ?: tiff;
		if (!src) return NULL;
		NSImage *img = [[NSImage alloc] initWithData:src];
		if (!img) return NULL;
		NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithData:[img TIFFRepresentation]];
		if (!rep) return NULL;
		NSData *out = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
		if (!out) return NULL;
		*out_len = (int)out.length;
		unsigned char *buf = (unsigned char *)malloc((size_t)out.length);
		if (!buf) return NULL;
		memcpy(buf, out.bytes, (size_t)out.length);
		return buf;
	}
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

func writeClipboardText(s string) error {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	C.re_clip_write(cs)
	return nil
}

func readClipboardText() (string, error) {
	p := C.re_clip_read()
	if p == nil {
		return "", fmt.Errorf("empty")
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoString(p), nil
}

func writeClipboardPNGDarwin(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	r := C.re_clip_write_png((*C.uchar)(unsafe.Pointer(&b[0])), C.int(len(b)))
	if r != 0 {
		return fmt.Errorf("clipboard png write failed: %d", int(r))
	}
	return nil
}

func readClipboardPNGDarwin() ([]byte, error) {
	var n C.int
	p := C.re_clip_read_png(&n)
	if p == nil || n <= 0 {
		return nil, fmt.Errorf("no image")
	}
	defer C.free(unsafe.Pointer(p))
	return C.GoBytes(unsafe.Pointer(p), n), nil
}

func writeClipboardPNG(b []byte) error { return writeClipboardPNGDarwin(b) }
func readClipboardPNG() ([]byte, error) { return readClipboardPNGDarwin() }
