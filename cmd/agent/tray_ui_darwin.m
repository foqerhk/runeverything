//go:build darwin

#import <Cocoa/Cocoa.h>
#import <stdlib.h>
#import "tray_ui_darwin.h"

static void reBringFront(NSWindow *win) {
  win.level = NSFloatingWindowLevel;
  win.hidesOnDeactivate = NO;
  win.collectionBehavior =
      NSWindowCollectionBehaviorMoveToActiveSpace |
      NSWindowCollectionBehaviorFullScreenAuxiliary;
  [win orderFrontRegardless];
  [win makeKeyAndOrderFront:nil];
  [NSApp activateIgnoringOtherApps:YES];
}

// Resize content height to Auto Layout fitting size, then optionally center on screen.
static void reFitContentWindow(NSWindow *win, CGFloat width, BOOL centerOnScreen) {
  if (!win) return;
  NSView *content = win.contentView;
  [content layoutSubtreeIfNeeded];
  CGFloat contentH = content.fittingSize.height;
  NSScreen *screen = win.screen ?: [NSScreen mainScreen];
  CGFloat screenH = screen ? NSHeight(screen.visibleFrame) : 900;
  CGFloat maxH = MAX(160, screenH * 0.85);
  if (contentH > maxH) contentH = maxH;
  if (contentH < 100) contentH = 100;
  NSRect contentRect = NSMakeRect(0, 0, width, contentH);
  NSRect frame = [win frameRectForContentRect:contentRect];
  if (centerOnScreen) {
    NSRect vis = screen ? screen.visibleFrame : NSMakeRect(0, 0, 1280, 800);
    frame.origin.x = NSMinX(vis) + (NSWidth(vis) - NSWidth(frame)) / 2.0;
    frame.origin.y = NSMinY(vis) + (NSHeight(vis) - NSHeight(frame)) / 2.0;
  } else {
    NSRect current = win.frame;
    frame.origin.x = current.origin.x;
    frame.origin.y = current.origin.y + (current.size.height - frame.size.height);
  }
  [win setFrame:frame display:YES];
}

static CGFloat reMeasureTextHeight(NSString *text, NSFont *font, CGFloat width) {
  if (text.length == 0) return 40;
  NSDictionary *attrs = @{NSFontAttributeName : font};
  NSRect r = [text boundingRectWithSize:NSMakeSize(width, CGFLOAT_MAX)
                                options:(NSStringDrawingUsesLineFragmentOrigin |
                                         NSStringDrawingUsesFontLeading)
                             attributes:attrs
                                context:nil];
  return ceil(NSHeight(r)) + 16;
}

void re_ui_set_app_icon_png(const unsigned char *data, int len) {
  if (!data || len <= 0) return;
  void (^block)(void) = ^{
    NSData *ns = [NSData dataWithBytes:data length:(NSUInteger)len];
    NSImage *img = [[NSImage alloc] initWithData:ns];
    if (img) {
      [NSApplication sharedApplication].applicationIconImage = img;
    }
  };
  if ([NSThread isMainThread]) block();
  else dispatch_async(dispatch_get_main_queue(), block);
}

@interface RETextWindowController : NSObject
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, strong) NSScrollView *scrollView;
@property(nonatomic, strong) NSLayoutConstraint *scrollHeight;
@property(nonatomic, strong) NSTextView *textView;
@property(nonatomic, copy) NSString *body;
@property(nonatomic, assign) CGFloat width;
- (void)fit;
@end
@implementation RETextWindowController
- (void)onClose:(id)sender { [self.window close]; }
- (void)fit {
  NSTextView *tv = self.textView;
  NSFont *font = tv.font ?: [NSFont monospacedSystemFontOfSize:12 weight:NSFontWeightRegular];
  // Match real layout width: window − scroll insets(14×2) − bezel(~4) − text inset(6×2).
  // Using a wider measure width under-counted wrapped lines → short scroll view → scrollbar.
  CGFloat textW = MAX(80, self.width - 14 * 2 - 4 - 12);
  tv.string = self.body ?: @"";
  tv.textContainer.containerSize = NSMakeSize(textW, CGFLOAT_MAX);
  tv.textContainer.widthTracksTextView = NO;
  [tv.layoutManager ensureLayoutForTextContainer:tv.textContainer];
  NSRect used = [tv.layoutManager usedRectForTextContainer:tv.textContainer];
  // inset top+bottom + bezel + a few px so last line isn't clipped into a scroller.
  CGFloat textH = ceil(NSHeight(used)) + tv.textContainerInset.height * 2 + 12;
  if (textH < 48) {
    textH = reMeasureTextHeight(self.body ?: @"", font, textW);
  }
  NSScreen *screen = self.window.screen ?: [NSScreen mainScreen];
  CGFloat screenH = screen ? NSHeight(screen.visibleFrame) : 900;
  CGFloat maxScroll = MAX(100, screenH * 0.65);
  CGFloat needed = MAX(textH, 48);
  BOOL fits = needed <= maxScroll + 0.5;
  self.scrollHeight.constant = MIN(needed, maxScroll);
  // Hide scroller entirely when content fits — autohide still flashes a gutter if
  // documentView is 1–2px taller than the clip view from under-measurement.
  self.scrollView.hasVerticalScroller = !fits;
  self.scrollView.hasHorizontalScroller = NO;
  NSSize doc = NSMakeSize(textW + tv.textContainerInset.width * 2, needed);
  [tv setFrameSize:doc];
  [self.window.contentView layoutSubtreeIfNeeded];
  reFitContentWindow(self.window, self.width, YES);
}
@end

void re_ui_show_text_window(const char *title, const char *body, const char *btn_close, int width) {
  NSString *nsTitle = title ? [NSString stringWithUTF8String:title] : @"Info";
  NSString *nsBody = body ? [NSString stringWithUTF8String:body] : @"";
  NSString *nsClose = btn_close ? [NSString stringWithUTF8String:btn_close] : @"OK";
  CGFloat w = width > 0 ? (CGFloat)width : 560;

  void (^build)(void) = ^{
    [NSApplication sharedApplication];
    NSWindow *win = [[NSWindow alloc]
        initWithContentRect:NSMakeRect(0, 0, w, 160)
                  styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskResizable)
                    backing:NSBackingStoreBuffered
                      defer:NO];
    win.title = nsTitle;
    win.releasedWhenClosed = NO;

    RETextWindowController *c = [RETextWindowController new];
    c.window = win;
    c.width = w;
    c.body = nsBody;

    NSFont *font = [NSFont monospacedSystemFontOfSize:12 weight:NSFontWeightRegular];
    NSTextView *tv = [[NSTextView alloc] initWithFrame:NSMakeRect(0, 0, w - 40, 40)];
    tv.string = nsBody;
    tv.editable = NO;
    tv.selectable = YES;
    tv.font = font;
    tv.textColor = NSColor.labelColor;
    tv.backgroundColor = NSColor.textBackgroundColor;
    tv.drawsBackground = YES;
    tv.verticallyResizable = YES;
    tv.horizontallyResizable = NO;
    tv.textContainerInset = NSMakeSize(6, 8);
    tv.textContainer.widthTracksTextView = NO;
    tv.textContainer.containerSize = NSMakeSize(MAX(80, w - 14 * 2 - 4 - 12), CGFLOAT_MAX);
    c.textView = tv;

    NSScrollView *scroll = [[NSScrollView alloc] initWithFrame:NSZeroRect];
    scroll.hasVerticalScroller = YES;
    scroll.hasHorizontalScroller = NO;
    scroll.autohidesScrollers = YES;
    scroll.borderType = NSBezelBorder;
    scroll.documentView = tv;
    scroll.translatesAutoresizingMaskIntoConstraints = NO;
    c.scrollView = scroll;

    NSButton *close = [NSButton buttonWithTitle:nsClose target:c action:@selector(onClose:)];
    close.bezelStyle = NSBezelStyleRounded;
    close.keyEquivalent = @"\r";
    close.translatesAutoresizingMaskIntoConstraints = NO;

    NSView *content = win.contentView;
    [content addSubview:scroll];
    [content addSubview:close];
    NSLayoutConstraint *scrollH = [scroll.heightAnchor constraintEqualToConstant:48];
    c.scrollHeight = scrollH;
    [NSLayoutConstraint activateConstraints:@[
      [scroll.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:14],
      [scroll.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-14],
      [scroll.topAnchor constraintEqualToAnchor:content.topAnchor constant:14],
      scrollH,
      [close.topAnchor constraintEqualToAnchor:scroll.bottomAnchor constant:10],
      [close.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-14],
      [close.bottomAnchor constraintEqualToAnchor:content.bottomAnchor constant:-12],
    ]];
    [c fit];
    reBringFront(win);
  };
  if ([NSThread isMainThread]) build();
  else dispatch_async(dispatch_get_main_queue(), build);
}

@interface REAboutController : NSObject
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, strong) NSTextField *status;
@property(nonatomic, copy) NSString *pendingURL;
@property(nonatomic, copy) NSString *githubURL;
@property(nonatomic, copy) NSString *email;
@end
@implementation REAboutController
- (void)onClose:(id)sender { [self.window close]; }
- (void)onGitHub:(id)sender {
  if (self.githubURL.length == 0) return;
  char *u = strdup(self.githubURL.UTF8String);
  reUIOpenURL(u);
  free(u);
}
- (void)onEmail:(id)sender {
  if (self.email.length == 0) return;
  NSString *mailto = [NSString stringWithFormat:@"mailto:%@", self.email];
  char *u = strdup(mailto.UTF8String);
  reUIOpenURL(u);
  free(u);
}
- (void)onCheck:(id)sender {
  self.status.stringValue = @"…";
  dispatch_async(dispatch_get_global_queue(DISPATCH_QUEUE_PRIORITY_DEFAULT, 0), ^{
    char *raw = reUICheckUpdate();
    NSString *s = raw ? [NSString stringWithUTF8String:raw] : @"";
    if (raw) free(raw);
    NSArray *parts = [s componentsSeparatedByString:@"|"];
    BOOL newer = parts.count > 0 && [parts[0] isEqualToString:@"1"];
    NSString *msg = parts.count > 1 ? parts[1] : s;
    NSString *url = parts.count > 2 ? parts[2] : @"";
    dispatch_async(dispatch_get_main_queue(), ^{
      self.status.stringValue = msg ?: @"";
      self.pendingURL = url;
      reFitContentWindow(self.window, 460, NO);
      if (newer && url.length > 0) {
        char *u = strdup(url.UTF8String);
        reUIOpenURL(u);
        free(u);
      }
    });
  });
}
@end

void re_ui_show_about_window(const char *title, const char *app_name, const char *version_line,
                             const unsigned char *icon_png, int icon_len,
                             const char *github_url, const char *contact_email,
                             const char *btn_check, const char *btn_close) {
  NSString *nsTitle = title ? [NSString stringWithUTF8String:title] : @"About";
  NSString *nsName = app_name ? [NSString stringWithUTF8String:app_name] : @"RunEverything";
  NSString *nsVer = version_line ? [NSString stringWithUTF8String:version_line] : @"";
  NSString *nsGit = github_url ? [NSString stringWithUTF8String:github_url] : @"";
  NSString *nsMail = contact_email ? [NSString stringWithUTF8String:contact_email] : @"";
  NSString *nsCheck = btn_check ? [NSString stringWithUTF8String:btn_check] : @"Check";
  NSString *nsClose = btn_close ? [NSString stringWithUTF8String:btn_close] : @"OK";
  NSImage *icon = nil;
  if (icon_png && icon_len > 0) {
    NSData *ns = [NSData dataWithBytes:icon_png length:(NSUInteger)icon_len];
    icon = [[NSImage alloc] initWithData:ns];
  }

  void (^build)(void) = ^{
    [NSApplication sharedApplication];
    NSWindow *win = [[NSWindow alloc]
        initWithContentRect:NSMakeRect(0, 0, 460, 200)
                  styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable)
                    backing:NSBackingStoreBuffered
                      defer:NO];
    win.title = nsTitle;
    win.releasedWhenClosed = NO;
    REAboutController *c = [REAboutController new];
    c.window = win;
    c.githubURL = nsGit;
    c.email = nsMail;

    NSImageView *iv = [[NSImageView alloc] initWithFrame:NSZeroRect];
    iv.image = icon;
    iv.imageScaling = NSImageScaleProportionallyUpOrDown;
    iv.translatesAutoresizingMaskIntoConstraints = NO;

    NSTextField *name = [NSTextField labelWithString:nsName];
    name.font = [NSFont systemFontOfSize:18 weight:NSFontWeightBold];
    name.translatesAutoresizingMaskIntoConstraints = NO;

    NSTextField *ver = [NSTextField labelWithString:nsVer];
    ver.font = [NSFont systemFontOfSize:13];
    ver.textColor = NSColor.secondaryLabelColor;
    ver.translatesAutoresizingMaskIntoConstraints = NO;

    NSButton *gitBtn = [NSButton buttonWithTitle:nsGit target:c action:@selector(onGitHub:)];
    gitBtn.bezelStyle = NSBezelStyleInline;
    gitBtn.bordered = NO;
    gitBtn.contentTintColor = [NSColor linkColor];
    gitBtn.translatesAutoresizingMaskIntoConstraints = NO;
    gitBtn.font = [NSFont systemFontOfSize:12];

    NSButton *mailBtn = [NSButton buttonWithTitle:nsMail target:c action:@selector(onEmail:)];
    mailBtn.bezelStyle = NSBezelStyleInline;
    mailBtn.bordered = NO;
    mailBtn.contentTintColor = [NSColor linkColor];
    mailBtn.translatesAutoresizingMaskIntoConstraints = NO;
    mailBtn.font = [NSFont systemFontOfSize:12];

    NSTextField *st = [NSTextField wrappingLabelWithString:@""];
    st.font = [NSFont systemFontOfSize:12];
    st.textColor = NSColor.secondaryLabelColor;
    st.translatesAutoresizingMaskIntoConstraints = NO;
    st.preferredMaxLayoutWidth = 400;
    c.status = st;

    NSButton *check = [NSButton buttonWithTitle:nsCheck target:c action:@selector(onCheck:)];
    check.bezelStyle = NSBezelStyleRounded;
    check.translatesAutoresizingMaskIntoConstraints = NO;
    NSButton *close = [NSButton buttonWithTitle:nsClose target:c action:@selector(onClose:)];
    close.bezelStyle = NSBezelStyleRounded;
    close.keyEquivalent = @"\r";
    close.translatesAutoresizingMaskIntoConstraints = NO;

    NSView *content = win.contentView;
    for (NSView *v in @[ iv, name, ver, gitBtn, mailBtn, st, check, close ]) {
      [content addSubview:v];
    }
    [NSLayoutConstraint activateConstraints:@[
      [iv.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:20],
      [iv.topAnchor constraintEqualToAnchor:content.topAnchor constant:20],
      [iv.widthAnchor constraintEqualToConstant:64],
      [iv.heightAnchor constraintEqualToConstant:64],

      [name.leadingAnchor constraintEqualToAnchor:iv.trailingAnchor constant:14],
      [name.topAnchor constraintEqualToAnchor:iv.topAnchor constant:2],
      [name.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-20],

      [ver.leadingAnchor constraintEqualToAnchor:name.leadingAnchor],
      [ver.topAnchor constraintEqualToAnchor:name.bottomAnchor constant:4],
      [ver.trailingAnchor constraintEqualToAnchor:name.trailingAnchor],

      [gitBtn.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:20],
      [gitBtn.topAnchor constraintEqualToAnchor:iv.bottomAnchor constant:14],
      [gitBtn.trailingAnchor constraintLessThanOrEqualToAnchor:content.trailingAnchor constant:-20],

      [mailBtn.leadingAnchor constraintEqualToAnchor:gitBtn.leadingAnchor],
      [mailBtn.topAnchor constraintEqualToAnchor:gitBtn.bottomAnchor constant:4],
      [mailBtn.trailingAnchor constraintLessThanOrEqualToAnchor:content.trailingAnchor constant:-20],

      [st.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:20],
      [st.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-20],
      [st.topAnchor constraintEqualToAnchor:mailBtn.bottomAnchor constant:12],

      [check.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:20],
      [check.topAnchor constraintEqualToAnchor:st.bottomAnchor constant:14],
      [check.bottomAnchor constraintEqualToAnchor:content.bottomAnchor constant:-16],
      [close.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-20],
      [close.centerYAnchor constraintEqualToAnchor:check.centerYAnchor],
    ]];
    reFitContentWindow(win, 460, YES);
    reBringFront(win);
  };
  if ([NSThread isMainThread]) build();
  else dispatch_async(dispatch_get_main_queue(), build);
}

@interface REConfigController : NSObject
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, strong) NSTextField *relayField;
@property(nonatomic, strong) NSTextField *publicField;
@property(nonatomic, strong) NSButton *manualBox;
@property(nonatomic, strong) NSButton *shareBox;
@end
@implementation REConfigController
- (void)onCancel:(id)sender { [self.window close]; }
- (void)onSave:(id)sender {
  char *r = strdup(self.relayField.stringValue.UTF8String ?: "");
  char *p = strdup(self.publicField.stringValue.UTF8String ?: "");
  char *err = reUIConfigSave(r, p, self.manualBox.state == NSControlStateValueOn ? 1 : 0,
                             self.shareBox.state == NSControlStateValueOn ? 1 : 0);
  free(r); free(p);
  if (err && err[0]) {
    NSAlert *a = [[NSAlert alloc] init];
    a.messageText = [NSString stringWithUTF8String:err];
    [a runModal];
    free(err);
    return;
  }
  if (err) free(err);
  [self.window close];
}
@end

void re_ui_show_config_window(const char *title, const char *relay_label, const char *public_label,
                              const char *manual_label, const char *manual_hint,
                              const char *share_label, const char *share_hint,
                              const char *relay, const char *pub,
                              int manual, int share,
                              const char *btn_save, const char *btn_cancel) {
  NSString *nsTitle = title ? [NSString stringWithUTF8String:title] : @"Config";
  NSString *nsRelayL = relay_label ? [NSString stringWithUTF8String:relay_label] : @"Relay";
  NSString *nsPubL = public_label ? [NSString stringWithUTF8String:public_label] : @"Public";
  NSString *nsManL = manual_label ? [NSString stringWithUTF8String:manual_label] : @"Manual";
  NSString *nsManH = manual_hint ? [NSString stringWithUTF8String:manual_hint] : @"";
  NSString *nsShareL = share_label ? [NSString stringWithUTF8String:share_label] : @"Share";
  NSString *nsShareH = share_hint ? [NSString stringWithUTF8String:share_hint] : @"";
  NSString *nsRelay = relay ? [NSString stringWithUTF8String:relay] : @"";
  NSString *nsPub = pub ? [NSString stringWithUTF8String:pub] : @"";
  NSString *nsSave = btn_save ? [NSString stringWithUTF8String:btn_save] : @"Save";
  NSString *nsCancel = btn_cancel ? [NSString stringWithUTF8String:btn_cancel] : @"Cancel";

  void (^build)(void) = ^{
    [NSApplication sharedApplication];
    NSWindow *win = [[NSWindow alloc]
        initWithContentRect:NSMakeRect(0, 0, 560, 200)
                  styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable)
                    backing:NSBackingStoreBuffered
                      defer:NO];
    win.title = nsTitle;
    win.releasedWhenClosed = NO;
    REConfigController *c = [REConfigController new];
    c.window = win;

    NSTextField *rl = [NSTextField labelWithString:nsRelayL];
    rl.translatesAutoresizingMaskIntoConstraints = NO;
    NSTextField *rf = [[NSTextField alloc] initWithFrame:NSZeroRect];
    rf.stringValue = nsRelay;
    rf.translatesAutoresizingMaskIntoConstraints = NO;
    c.relayField = rf;

    NSTextField *pl = [NSTextField labelWithString:nsPubL];
    pl.translatesAutoresizingMaskIntoConstraints = NO;
    NSTextField *pf = [[NSTextField alloc] initWithFrame:NSZeroRect];
    pf.stringValue = nsPub;
    pf.translatesAutoresizingMaskIntoConstraints = NO;
    c.publicField = pf;

    NSButton *mb = [NSButton checkboxWithTitle:nsManL target:nil action:nil];
    mb.state = manual ? NSControlStateValueOn : NSControlStateValueOff;
    mb.translatesAutoresizingMaskIntoConstraints = NO;
    c.manualBox = mb;
    NSTextField *mh = [NSTextField wrappingLabelWithString:nsManH];
    mh.font = [NSFont systemFontOfSize:11];
    mh.textColor = NSColor.secondaryLabelColor;
    mh.translatesAutoresizingMaskIntoConstraints = NO;
    mh.preferredMaxLayoutWidth = 520;

    NSButton *sb = [NSButton checkboxWithTitle:nsShareL target:nil action:nil];
    sb.state = share ? NSControlStateValueOn : NSControlStateValueOff;
    sb.translatesAutoresizingMaskIntoConstraints = NO;
    c.shareBox = sb;
    NSTextField *sh = [NSTextField wrappingLabelWithString:nsShareH];
    sh.font = [NSFont systemFontOfSize:11];
    sh.textColor = NSColor.secondaryLabelColor;
    sh.translatesAutoresizingMaskIntoConstraints = NO;
    sh.preferredMaxLayoutWidth = 520;

    NSButton *save = [NSButton buttonWithTitle:nsSave target:c action:@selector(onSave:)];
    save.bezelStyle = NSBezelStyleRounded;
    save.keyEquivalent = @"\r";
    save.translatesAutoresizingMaskIntoConstraints = NO;
    NSButton *cancel = [NSButton buttonWithTitle:nsCancel target:c action:@selector(onCancel:)];
    cancel.bezelStyle = NSBezelStyleRounded;
    cancel.translatesAutoresizingMaskIntoConstraints = NO;

    NSView *content = win.contentView;
    for (NSView *v in @[ rl, rf, pl, pf, mb, mh, sb, sh, save, cancel ]) [content addSubview:v];
    [NSLayoutConstraint activateConstraints:@[
      [rl.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:16],
      [rl.topAnchor constraintEqualToAnchor:content.topAnchor constant:16],
      [rf.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:16],
      [rf.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-16],
      [rf.topAnchor constraintEqualToAnchor:rl.bottomAnchor constant:4],
      [pl.leadingAnchor constraintEqualToAnchor:rl.leadingAnchor],
      [pl.topAnchor constraintEqualToAnchor:rf.bottomAnchor constant:12],
      [pf.leadingAnchor constraintEqualToAnchor:rf.leadingAnchor],
      [pf.trailingAnchor constraintEqualToAnchor:rf.trailingAnchor],
      [pf.topAnchor constraintEqualToAnchor:pl.bottomAnchor constant:4],
      [mb.leadingAnchor constraintEqualToAnchor:rf.leadingAnchor],
      [mb.topAnchor constraintEqualToAnchor:pf.bottomAnchor constant:14],
      [mh.leadingAnchor constraintEqualToAnchor:mb.leadingAnchor constant:20],
      [mh.trailingAnchor constraintEqualToAnchor:rf.trailingAnchor],
      [mh.topAnchor constraintEqualToAnchor:mb.bottomAnchor constant:2],
      [sb.leadingAnchor constraintEqualToAnchor:mb.leadingAnchor],
      [sb.topAnchor constraintEqualToAnchor:mh.bottomAnchor constant:12],
      [sh.leadingAnchor constraintEqualToAnchor:mh.leadingAnchor],
      [sh.trailingAnchor constraintEqualToAnchor:mh.trailingAnchor],
      [sh.topAnchor constraintEqualToAnchor:sb.bottomAnchor constant:2],
      [cancel.topAnchor constraintEqualToAnchor:sh.bottomAnchor constant:16],
      [cancel.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-16],
      [cancel.bottomAnchor constraintEqualToAnchor:content.bottomAnchor constant:-14],
      [save.trailingAnchor constraintEqualToAnchor:cancel.leadingAnchor constant:-10],
      [save.centerYAnchor constraintEqualToAnchor:cancel.centerYAnchor],
    ]];
    reFitContentWindow(win, 560, YES);
    reBringFront(win);
  };
  if ([NSThread isMainThread]) build();
  else dispatch_async(dispatch_get_main_queue(), build);
}

@interface REQRController : NSObject <NSWindowDelegate>
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, strong) NSImageView *imageView;
@property(nonatomic, strong) NSTextField *expireLabel;
@property(nonatomic, strong) NSTextField *statusLabel;
@property(nonatomic, assign) long long expiresAt;
@property(nonatomic, copy) NSString *link;
@property(nonatomic, copy) NSString *expiresFmt;     // contains one %@ for remaining time
@property(nonatomic, copy) NSString *refreshingText;
@property(nonatomic, strong) NSTimer *timer;
@property(nonatomic, assign) BOOL refreshing;
@property(nonatomic, assign) BOOL refreshRequested; // coalesce expiry→Go signals
- (void)applyPNG:(NSData *)png expires:(long long)exp link:(NSString *)link;
- (void)applyError:(NSString *)err;
- (void)tick;
- (void)requestRefresh;
@end

static REQRController *gQRController = nil;

@implementation REQRController
- (void)onClose:(id)sender {
  [self.window close];
}
- (void)windowWillClose:(NSNotification *)n {
  [self.timer invalidate];
  self.timer = nil;
  if (gQRController == self) gQRController = nil;
  reUIQRWindowClosed();
}
- (void)onRefresh:(id)sender { [self requestRefresh]; }

- (void)applyPNG:(NSData *)png expires:(long long)exp link:(NSString *)link {
  if (png.length > 0) {
    NSImage *img = [[NSImage alloc] initWithData:png];
    if (img) self.imageView.image = img;
  }
  self.expiresAt = exp;
  if (link.length > 0) {
    self.link = link;
    NSPasteboard *pb = [NSPasteboard generalPasteboard];
    [pb clearContents];
    [pb setString:link forType:NSPasteboardTypeString];
  }
  self.refreshing = NO;
  self.refreshRequested = NO;
  self.statusLabel.stringValue = @"";
  [self tick];
}

- (void)applyError:(NSString *)err {
  self.refreshing = NO;
  self.refreshRequested = NO;
  self.statusLabel.stringValue = err.length ? err : @"refresh failed";
}

- (void)tick {
  if (!self.window || !self.window.isVisible) return;
  long long now = (long long)[[NSDate date] timeIntervalSince1970];
  long long left = self.expiresAt - now;
  if (left <= 0) {
    self.expireLabel.stringValue = self.refreshingText ?: @"…";
    // Go owns the mint (avoids cgo-from-GCD deadlock with LockOSThread systray).
    [self requestRefresh];
    return;
  }
  long long m = left / 60;
  long long s = left % 60;
  NSString *remain = [NSString stringWithFormat:@"%lld:%02lld", m, s];
  NSString *fmt = self.expiresFmt.length ? self.expiresFmt : @"%@";
  @try {
    self.expireLabel.stringValue = [NSString stringWithFormat:fmt, remain];
  } @catch (NSException *ex) {
    self.expireLabel.stringValue = remain;
  }
}

- (void)requestRefresh {
  if (self.refreshRequested) return;
  self.refreshRequested = YES;
  self.refreshing = YES;
  self.statusLabel.stringValue = self.refreshingText ?: @"…";
  reUIRequestPairQRRefresh();
}
@end

void re_ui_close_qr_window(void) {
  void (^close)(void) = ^{
    if (!gQRController) return;
    NSWindow *win = gQRController.window;
    if (win) [win close];
  };
  if ([NSThread isMainThread]) close();
  else dispatch_async(dispatch_get_main_queue(), close);
}

void re_ui_qr_apply(const unsigned char *png, int png_len, long long expires_at,
                    const char *link, const char *err) {
  NSData *pngData = (png && png_len > 0) ? [NSData dataWithBytes:png length:(NSUInteger)png_len] : nil;
  NSString *nsLink = link ? [NSString stringWithUTF8String:link] : @"";
  NSString *nsErr = err ? [NSString stringWithUTF8String:err] : @"";
  void (^apply)(void) = ^{
    if (!gQRController) return;
    if (nsErr.length > 0 || pngData.length == 0) {
      [gQRController applyError:nsErr];
      // Ask Go to retry shortly via another request after a delay.
      dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(3 * NSEC_PER_SEC)),
                     dispatch_get_main_queue(), ^{
        if (gQRController && gQRController.window.isVisible) {
          long long now = (long long)[[NSDate date] timeIntervalSince1970];
          if (gQRController.expiresAt <= now) [gQRController requestRefresh];
        }
      });
      return;
    }
    [gQRController applyPNG:pngData expires:expires_at link:nsLink];
  };
  if ([NSThread isMainThread]) apply();
  else dispatch_async(dispatch_get_main_queue(), apply);
}

void re_ui_show_qr_window(const char *title, const unsigned char *png, int png_len,
                          long long expires_at, const char *link,
                          const char *expires_fmt, const char *refreshing_text,
                          const char *btn_refresh, const char *btn_close) {
  NSString *nsTitle = title ? [NSString stringWithUTF8String:title] : @"QR";
  NSString *nsLink = link ? [NSString stringWithUTF8String:link] : @"";
  NSString *nsFmt = expires_fmt ? [NSString stringWithUTF8String:expires_fmt] : @"%@";
  NSString *nsRefreshing = refreshing_text ? [NSString stringWithUTF8String:refreshing_text] : @"…";
  NSString *nsRefresh = btn_refresh ? [NSString stringWithUTF8String:btn_refresh] : @"Refresh";
  NSString *nsClose = btn_close ? [NSString stringWithUTF8String:btn_close] : @"OK";
  NSData *pngData = (png && png_len > 0) ? [NSData dataWithBytes:png length:(NSUInteger)png_len] : nil;

  void (^build)(void) = ^{
    [NSApplication sharedApplication];
    if (gQRController && gQRController.window) {
      gQRController.expiresFmt = nsFmt;
      gQRController.refreshingText = nsRefreshing;
      [gQRController applyPNG:pngData expires:expires_at link:nsLink];
      reBringFront(gQRController.window);
      return;
    }

    NSWindow *win = [[NSWindow alloc]
        initWithContentRect:NSMakeRect(0, 0, 360, 420)
                  styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable)
                    backing:NSBackingStoreBuffered
                      defer:NO];
    win.title = nsTitle;
    win.releasedWhenClosed = NO;

    REQRController *c = [REQRController new];
    c.window = win;
    c.expiresFmt = nsFmt;
    c.refreshingText = nsRefreshing;
    win.delegate = c;
    gQRController = c;

    NSImageView *iv = [[NSImageView alloc] initWithFrame:NSZeroRect];
    iv.imageScaling = NSImageScaleProportionallyUpOrDown;
    iv.translatesAutoresizingMaskIntoConstraints = NO;
    c.imageView = iv;

    NSTextField *exp = [NSTextField labelWithString:@""];
    exp.font = [NSFont systemFontOfSize:13 weight:NSFontWeightMedium];
    exp.alignment = NSTextAlignmentCenter;
    exp.translatesAutoresizingMaskIntoConstraints = NO;
    c.expireLabel = exp;

    NSTextField *st = [NSTextField wrappingLabelWithString:@""];
    st.font = [NSFont systemFontOfSize:11];
    st.textColor = NSColor.secondaryLabelColor;
    st.alignment = NSTextAlignmentCenter;
    st.translatesAutoresizingMaskIntoConstraints = NO;
    st.preferredMaxLayoutWidth = 320;
    c.statusLabel = st;

    NSButton *refresh = [NSButton buttonWithTitle:nsRefresh target:c action:@selector(onRefresh:)];
    refresh.bezelStyle = NSBezelStyleRounded;
    refresh.translatesAutoresizingMaskIntoConstraints = NO;
    NSButton *close = [NSButton buttonWithTitle:nsClose target:c action:@selector(onClose:)];
    close.bezelStyle = NSBezelStyleRounded;
    close.keyEquivalent = @"\r";
    close.translatesAutoresizingMaskIntoConstraints = NO;

    NSView *content = win.contentView;
    for (NSView *v in @[ iv, exp, st, refresh, close ]) {
      [content addSubview:v];
    }
    [NSLayoutConstraint activateConstraints:@[
      [iv.topAnchor constraintEqualToAnchor:content.topAnchor constant:16],
      [iv.centerXAnchor constraintEqualToAnchor:content.centerXAnchor],
      [iv.widthAnchor constraintEqualToConstant:280],
      [iv.heightAnchor constraintEqualToConstant:280],

      [exp.topAnchor constraintEqualToAnchor:iv.bottomAnchor constant:12],
      [exp.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:16],
      [exp.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-16],

      [st.topAnchor constraintEqualToAnchor:exp.bottomAnchor constant:4],
      [st.leadingAnchor constraintEqualToAnchor:exp.leadingAnchor],
      [st.trailingAnchor constraintEqualToAnchor:exp.trailingAnchor],

      [refresh.topAnchor constraintEqualToAnchor:st.bottomAnchor constant:12],
      [refresh.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:16],
      [refresh.bottomAnchor constraintEqualToAnchor:content.bottomAnchor constant:-14],
      [close.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-16],
      [close.centerYAnchor constraintEqualToAnchor:refresh.centerYAnchor],
    ]];

    [c applyPNG:pngData expires:expires_at link:nsLink];
    c.timer = [NSTimer scheduledTimerWithTimeInterval:1.0
                                               target:c
                                             selector:@selector(tick)
                                             userInfo:nil
                                              repeats:YES];
    [[NSRunLoop mainRunLoop] addTimer:c.timer forMode:NSRunLoopCommonModes];
    reFitContentWindow(win, 360, YES);
    reBringFront(win);
  };
  if ([NSThread isMainThread]) build();
  else dispatch_async(dispatch_get_main_queue(), build);
}
