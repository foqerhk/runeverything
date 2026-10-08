//go:build darwin

#import <Cocoa/Cocoa.h>
#import "tray_perms_panel_darwin.h"

@interface REPermsPanelController : NSObject <NSWindowDelegate>
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, strong) NSStackView *list;
@property(nonatomic, strong) NSTextField *hintLabel;
@property(nonatomic, strong) NSButton *doneButton;
@property(nonatomic, copy) NSString *btnSettingsTitle;
@property(nonatomic, copy) NSString *btnDoneTitle;
@property(nonatomic, copy) NSString *btnDoneRestartTitle;
@property(nonatomic, assign) BOOL didPlace;
@property(nonatomic, assign) BOOL observing;
- (void)rebuild;
- (void)fitWindow;
- (void)openID:(NSString *)ident;
- (void)refreshIfVisible;
@end

static REPermsPanelController *gPanel;

@implementation REPermsPanelController

- (void)rebuild {
  char *raw = rePermsStatusJSON();
  if (!raw) return;
  NSData *data = [NSData dataWithBytes:raw length:strlen(raw)];
  free(raw);
  NSArray *rows = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
  if (![rows isKindOfClass:[NSArray class]]) return;

  for (NSView *v in [self.list.views copy]) {
    [self.list removeView:v];
  }

  BOOL needsRestart = NO;

  for (NSDictionary *row in rows) {
    if (![row isKindOfClass:[NSDictionary class]]) continue;
    NSString *title = row[@"title"] ?: @"";
    NSString *desc = row[@"desc"] ?: @"";
    NSString *status = row[@"status"] ?: @"";
    NSString *action = row[@"action"] ?: @"";
    NSString *ident = row[@"id"] ?: @"";
    NSString *kind = row[@"badge"] ?: @"";
    BOOL required = [row[@"required"] boolValue];
    if ([action isEqualToString:@"restart"]) needsRestart = YES;

    NSView *card = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 520, 56)];
    card.wantsLayer = YES;
    card.layer.backgroundColor = NSColor.controlBackgroundColor.CGColor;
    card.layer.cornerRadius = 10;

    NSTextField *t = [NSTextField labelWithString:title];
    t.font = [NSFont systemFontOfSize:13 weight:NSFontWeightSemibold];
    t.translatesAutoresizingMaskIntoConstraints = NO;

    // Plain colored kind text only (必需 / 可选) — no chip / accent bar.
    NSTextField *kindLabel = [NSTextField labelWithString:kind];
    kindLabel.font = [NSFont systemFontOfSize:11 weight:NSFontWeightSemibold];
    kindLabel.translatesAutoresizingMaskIntoConstraints = NO;
    if (required) {
      kindLabel.textColor = [NSColor colorWithCalibratedRed:0.82 green:0.32 blue:0.08 alpha:1];
    } else {
      kindLabel.textColor = [NSColor colorWithCalibratedRed:0.30 green:0.48 blue:0.78 alpha:1];
    }

    NSTextField *d = [NSTextField wrappingLabelWithString:desc];
    d.font = [NSFont systemFontOfSize:11];
    d.textColor = NSColor.secondaryLabelColor;
    d.translatesAutoresizingMaskIntoConstraints = NO;
    d.preferredMaxLayoutWidth = 360;

    NSTextField *st = [NSTextField labelWithString:status];
    st.font = [NSFont systemFontOfSize:11 weight:NSFontWeightSemibold];
    st.translatesAutoresizingMaskIntoConstraints = NO;
    if ([action isEqualToString:@"ok"]) {
      st.stringValue = [NSString stringWithFormat:@"✓ %@", status];
      st.textColor = [NSColor colorWithCalibratedRed:0.10 green:0.55 blue:0.40 alpha:1];
    } else if ([action isEqualToString:@"restart"]) {
      st.textColor = [NSColor colorWithCalibratedRed:0.75 green:0.45 blue:0.10 alpha:1];
    } else if (required) {
      st.textColor = [NSColor colorWithCalibratedRed:0.75 green:0.35 blue:0.12 alpha:1];
    } else {
      st.textColor = NSColor.secondaryLabelColor;
    }

    [card addSubview:t];
    [card addSubview:kindLabel];
    [card addSubview:d];
    [card addSubview:st];

    NSButton *btn = nil;
    if ([action isEqualToString:@"settings"]) {
      btn = [NSButton buttonWithTitle:self.btnSettingsTitle target:self action:@selector(onSettings:)];
      btn.bezelStyle = NSBezelStyleRounded;
      btn.identifier = ident;
      btn.translatesAutoresizingMaskIntoConstraints = NO;
      [card addSubview:btn];
    }

    [NSLayoutConstraint activateConstraints:@[
      [t.leadingAnchor constraintEqualToAnchor:card.leadingAnchor constant:14],
      [t.topAnchor constraintEqualToAnchor:card.topAnchor constant:10],
      [t.trailingAnchor constraintLessThanOrEqualToAnchor:st.leadingAnchor constant:-10],

      [kindLabel.leadingAnchor constraintEqualToAnchor:t.leadingAnchor],
      [kindLabel.topAnchor constraintEqualToAnchor:t.bottomAnchor constant:3],

      [d.leadingAnchor constraintEqualToAnchor:kindLabel.trailingAnchor constant:6],
      [d.firstBaselineAnchor constraintEqualToAnchor:kindLabel.firstBaselineAnchor],
      [d.trailingAnchor constraintLessThanOrEqualToAnchor:st.leadingAnchor constant:-10],
      [d.bottomAnchor constraintEqualToAnchor:card.bottomAnchor constant:-10],

      [st.trailingAnchor constraintEqualToAnchor:card.trailingAnchor constant:-12],
      [st.topAnchor constraintEqualToAnchor:card.topAnchor constant:10],
      [card.heightAnchor constraintGreaterThanOrEqualToConstant:52],
    ]];
    if (btn) {
      [NSLayoutConstraint activateConstraints:@[
        [btn.trailingAnchor constraintEqualToAnchor:card.trailingAnchor constant:-12],
        [btn.topAnchor constraintEqualToAnchor:st.bottomAnchor constant:6],
        [btn.bottomAnchor constraintLessThanOrEqualToAnchor:card.bottomAnchor constant:-8],
        [st.trailingAnchor constraintEqualToAnchor:btn.trailingAnchor],
      ]];
    }

    [self.list addView:card inGravity:NSStackViewGravityTop];
    [card.widthAnchor constraintEqualToAnchor:self.list.widthAnchor].active = YES;
  }

  // Footer Done ↔ Done & relaunch when Screen Recording needs restart.
  if (needsRestart) {
    self.doneButton.title = self.btnDoneRestartTitle;
    self.doneButton.target = self;
    self.doneButton.action = @selector(onRestart:);
  } else {
    self.doneButton.title = self.btnDoneTitle;
    self.doneButton.target = self;
    self.doneButton.action = @selector(onDone:);
  }

  [self fitWindow];
}

- (void)fitWindow {
  if (!self.window) return;
  NSView *content = self.window.contentView;
  [content layoutSubtreeIfNeeded];
  CGFloat contentH = content.fittingSize.height;
  CGFloat contentW = 560;
  NSScreen *screen = self.window.screen ?: [NSScreen mainScreen];
  CGFloat screenH = screen ? NSHeight(screen.visibleFrame) : 900;
  CGFloat maxH = MAX(200, screenH * 0.85);
  if (contentH > maxH) contentH = maxH;
  if (contentH < 140) contentH = 140;
  NSRect frame = [self.window frameRectForContentRect:NSMakeRect(0, 0, contentW, contentH)];
  if (!self.didPlace) {
    NSRect vis = screen ? screen.visibleFrame : NSMakeRect(0, 0, 1280, 800);
    frame.origin.x = NSMinX(vis) + (NSWidth(vis) - NSWidth(frame)) / 2.0;
    frame.origin.y = NSMinY(vis) + (NSHeight(vis) - NSHeight(frame)) / 2.0;
    self.didPlace = YES;
  } else {
    NSRect cur = self.window.frame;
    frame.origin.x = cur.origin.x;
    frame.origin.y = cur.origin.y + (NSHeight(cur) - NSHeight(frame));
  }
  [self.window setFrame:frame display:YES animate:NO];
}

- (void)onSettings:(NSButton *)sender {
  [self openID:sender.identifier];
  // System sheet may take a moment; refresh once after the prompt settles.
  dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.6 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
    [self refreshIfVisible];
  });
}

- (void)onRestart:(NSButton *)sender {
  rePermsRelaunch();
}

- (void)openID:(NSString *)ident {
  if (ident.length == 0) return;
  char *cid = strdup(ident.UTF8String);
  rePermsOpenID(cid);
  free(cid);
}

- (void)onDone:(id)sender {
  [self.window close];
}

- (void)refreshIfVisible {
  if (self.window && self.window.isVisible) {
    [self rebuild];
  }
}

- (void)onAppDidBecomeActive:(NSNotification *)n {
  [self refreshIfVisible];
}

- (void)startObserving {
  if (self.observing) return;
  self.observing = YES;
  [[NSNotificationCenter defaultCenter] addObserver:self
                                           selector:@selector(onAppDidBecomeActive:)
                                               name:NSApplicationDidBecomeActiveNotification
                                             object:nil];
}

- (void)stopObserving {
  if (!self.observing) return;
  self.observing = NO;
  [[NSNotificationCenter defaultCenter] removeObserver:self
                                                  name:NSApplicationDidBecomeActiveNotification
                                                object:nil];
}

- (void)windowWillClose:(NSNotification *)n {
  [self stopObserving];
}

@end

void re_perms_panel_show(const char *title, const char *subtitle,
                         const char *btn_settings, const char *btn_done,
                         const char *btn_done_restart, const char *hint) {
  NSString *nsTitle = title ? [NSString stringWithUTF8String:title] : @"Permissions";
  NSString *nsSub = subtitle ? [NSString stringWithUTF8String:subtitle] : @"";
  NSString *nsBtnSet = btn_settings ? [NSString stringWithUTF8String:btn_settings] : @"Open Settings";
  NSString *nsBtnDone = btn_done ? [NSString stringWithUTF8String:btn_done] : @"Done";
  NSString *nsBtnDoneRestart = btn_done_restart ? [NSString stringWithUTF8String:btn_done_restart] : @"Done & relaunch";
  NSString *nsHint = hint ? [NSString stringWithUTF8String:hint] : @"";

  // Normal key window: front when shown, yields when user focuses elsewhere.
  void (^bringFront)(NSWindow *) = ^(NSWindow *win) {
    win.level = NSNormalWindowLevel;
    win.hidesOnDeactivate = NO;
    [NSApp activateIgnoringOtherApps:YES];
    [win makeKeyAndOrderFront:nil];
  };

  void (^build)(void) = ^{
    [NSApplication sharedApplication];
    if (gPanel.window && gPanel.window.isVisible) {
      [gPanel rebuild];
      bringFront(gPanel.window);
      return;
    }

    REPermsPanelController *c = [REPermsPanelController new];
    c.btnSettingsTitle = nsBtnSet;
    c.btnDoneTitle = nsBtnDone;
    c.btnDoneRestartTitle = nsBtnDoneRestart;
    gPanel = c;

    NSWindow *win = [[NSWindow alloc]
        initWithContentRect:NSMakeRect(0, 0, 560, 200)
                  styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable)
                    backing:NSBackingStoreBuffered
                      defer:NO];
    win.title = nsTitle;
    win.releasedWhenClosed = NO;
    win.delegate = c;
    c.window = win;

    NSView *content = win.contentView;
    NSTextField *sub = [NSTextField wrappingLabelWithString:nsSub];
    sub.font = [NSFont systemFontOfSize:12];
    sub.textColor = NSColor.secondaryLabelColor;
    sub.translatesAutoresizingMaskIntoConstraints = NO;
    sub.preferredMaxLayoutWidth = 528;
    sub.maximumNumberOfLines = 0;

    NSStackView *list = [[NSStackView alloc] init];
    list.orientation = NSUserInterfaceLayoutOrientationVertical;
    list.spacing = 8;
    list.alignment = NSLayoutAttributeWidth;
    list.distribution = NSStackViewDistributionFill;
    list.translatesAutoresizingMaskIntoConstraints = NO;
    c.list = list;

    NSTextField *hintLabel = [NSTextField wrappingLabelWithString:nsHint];
    hintLabel.font = [NSFont systemFontOfSize:11];
    hintLabel.textColor = NSColor.tertiaryLabelColor;
    hintLabel.translatesAutoresizingMaskIntoConstraints = NO;
    hintLabel.preferredMaxLayoutWidth = 528;
    hintLabel.maximumNumberOfLines = 0;
    c.hintLabel = hintLabel;

    NSButton *done = [NSButton buttonWithTitle:nsBtnDone target:c action:@selector(onDone:)];
    done.bezelStyle = NSBezelStyleRounded;
    done.keyEquivalent = @"\r";
    done.translatesAutoresizingMaskIntoConstraints = NO;
    c.doneButton = done;

    [content addSubview:sub];
    [content addSubview:list];
    [content addSubview:hintLabel];
    [content addSubview:done];

    [list setContentHuggingPriority:NSLayoutPriorityRequired
                     forOrientation:NSLayoutConstraintOrientationVertical];
    [list setContentCompressionResistancePriority:NSLayoutPriorityRequired
                                   forOrientation:NSLayoutConstraintOrientationVertical];

    [NSLayoutConstraint activateConstraints:@[
      [sub.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:16],
      [sub.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-16],
      [sub.topAnchor constraintEqualToAnchor:content.topAnchor constant:10],

      [list.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:16],
      [list.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-16],
      [list.topAnchor constraintEqualToAnchor:sub.bottomAnchor constant:10],

      [hintLabel.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:16],
      [hintLabel.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-16],
      [hintLabel.topAnchor constraintEqualToAnchor:list.bottomAnchor constant:8],

      [done.topAnchor constraintEqualToAnchor:hintLabel.bottomAnchor constant:10],
      [done.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-16],
      [done.bottomAnchor constraintEqualToAnchor:content.bottomAnchor constant:-12],
    ]];

    [c rebuild];
    [c startObserving];
    [win center];
    bringFront(win);
  };

  if ([NSThread isMainThread]) {
    build();
  } else {
    dispatch_async(dispatch_get_main_queue(), build);
  }
}
