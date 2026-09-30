//go:build darwin

#import <Cocoa/Cocoa.h>
#import "tray_perms_panel_darwin.h"

@interface REPermsPanelController : NSObject
@property(nonatomic, strong) NSWindow *window;
@property(nonatomic, strong) NSStackView *list;
@property(nonatomic, strong) NSTextField *hintLabel;
@property(nonatomic, copy) NSString *btnSettingsTitle;
@property(nonatomic, strong) NSTimer *timer;
- (void)rebuild;
- (void)openID:(NSString *)ident;
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
    [self.list removeArrangedSubview:v];
    [v removeFromSuperview];
  }

  for (NSDictionary *row in rows) {
    if (![row isKindOfClass:[NSDictionary class]]) continue;
    NSString *title = row[@"title"] ?: @"";
    NSString *desc = row[@"desc"] ?: @"";
    NSString *status = row[@"status"] ?: @"";
    NSString *action = row[@"action"] ?: @"";
    NSString *ident = row[@"id"] ?: @"";
    BOOL required = [row[@"required"] boolValue];

    NSView *card = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, 560, 72)];
    card.wantsLayer = YES;
    card.layer.backgroundColor = NSColor.controlBackgroundColor.CGColor;
    card.layer.cornerRadius = 10;

    NSTextField *t = [NSTextField labelWithString:title];
    t.font = [NSFont systemFontOfSize:14 weight:NSFontWeightSemibold];
    t.translatesAutoresizingMaskIntoConstraints = NO;

    NSTextField *d = [NSTextField wrappingLabelWithString:desc];
    d.font = [NSFont systemFontOfSize:12];
    d.textColor = NSColor.secondaryLabelColor;
    d.translatesAutoresizingMaskIntoConstraints = NO;
    d.preferredMaxLayoutWidth = 360;

    NSTextField *st = [NSTextField labelWithString:status];
    st.font = [NSFont systemFontOfSize:12 weight:NSFontWeightSemibold];
    st.translatesAutoresizingMaskIntoConstraints = NO;
    if ([action isEqualToString:@"ok"]) {
      st.stringValue = [NSString stringWithFormat:@"✓ %@", status];
      st.textColor = [NSColor colorWithCalibratedRed:0.10 green:0.55 blue:0.40 alpha:1];
    } else if (required) {
      st.textColor = [NSColor colorWithCalibratedRed:0.75 green:0.35 blue:0.12 alpha:1];
    } else {
      st.textColor = NSColor.secondaryLabelColor;
    }

    [card addSubview:t];
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
      [t.topAnchor constraintEqualToAnchor:card.topAnchor constant:12],
      [t.trailingAnchor constraintLessThanOrEqualToAnchor:st.leadingAnchor constant:-12],
      [d.leadingAnchor constraintEqualToAnchor:t.leadingAnchor],
      [d.topAnchor constraintEqualToAnchor:t.bottomAnchor constant:4],
      [d.trailingAnchor constraintLessThanOrEqualToAnchor:st.leadingAnchor constant:-12],
      [d.bottomAnchor constraintEqualToAnchor:card.bottomAnchor constant:-12],
      [st.trailingAnchor constraintEqualToAnchor:card.trailingAnchor constant:-14],
      [st.topAnchor constraintEqualToAnchor:card.topAnchor constant:14],
      [card.heightAnchor constraintGreaterThanOrEqualToConstant:68],
    ]];
    if (btn) {
      [NSLayoutConstraint activateConstraints:@[
        [btn.trailingAnchor constraintEqualToAnchor:card.trailingAnchor constant:-14],
        [btn.topAnchor constraintEqualToAnchor:st.bottomAnchor constant:8],
        [btn.bottomAnchor constraintLessThanOrEqualToAnchor:card.bottomAnchor constant:-10],
        [st.trailingAnchor constraintEqualToAnchor:btn.trailingAnchor],
      ]];
    }

    [self.list addArrangedSubview:card];
    [card.widthAnchor constraintEqualToAnchor:self.list.widthAnchor].active = YES;
  }
}

- (void)onSettings:(NSButton *)sender {
  [self openID:sender.identifier];
  dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.4 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
    [self rebuild];
  });
}

- (void)openID:(NSString *)ident {
  if (ident.length == 0) return;
  char *cid = strdup(ident.UTF8String);
  rePermsOpenID(cid);
  free(cid);
}

- (void)onDone:(id)sender {
  [self.timer invalidate];
  self.timer = nil;
  [self.window close];
}

- (void)onTick:(NSTimer *)t {
  [self rebuild];
}

@end

void re_perms_panel_show(const char *title, const char *subtitle,
                         const char *btn_settings, const char *btn_done,
                         const char *hint) {
  NSString *nsTitle = title ? [NSString stringWithUTF8String:title] : @"Permissions";
  NSString *nsSub = subtitle ? [NSString stringWithUTF8String:subtitle] : @"";
  NSString *nsBtnSet = btn_settings ? [NSString stringWithUTF8String:btn_settings] : @"Open Settings";
  NSString *nsBtnDone = btn_done ? [NSString stringWithUTF8String:btn_done] : @"Done";
  NSString *nsHint = hint ? [NSString stringWithUTF8String:hint] : @"";

  void (^build)(void) = ^{
    [NSApplication sharedApplication];
    if (gPanel.window && gPanel.window.isVisible) {
      [gPanel rebuild];
      [gPanel.window makeKeyAndOrderFront:nil];
      [NSApp activateIgnoringOtherApps:YES];
      return;
    }

    REPermsPanelController *c = [REPermsPanelController new];
    c.btnSettingsTitle = nsBtnSet;
    gPanel = c;

    NSRect frame = NSMakeRect(0, 0, 600, 520);
    NSWindow *win = [[NSWindow alloc]
        initWithContentRect:frame
                  styleMask:(NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable)
                    backing:NSBackingStoreBuffered
                      defer:NO];
    win.title = nsTitle;
    win.releasedWhenClosed = NO;
    c.window = win;

    NSView *content = win.contentView;
    NSTextField *sub = [NSTextField wrappingLabelWithString:nsSub];
    sub.font = [NSFont systemFontOfSize:13];
    sub.textColor = NSColor.secondaryLabelColor;
    sub.translatesAutoresizingMaskIntoConstraints = NO;
    sub.preferredMaxLayoutWidth = 560;

    NSStackView *list = [[NSStackView alloc] init];
    list.orientation = NSUserInterfaceLayoutOrientationVertical;
    list.spacing = 10;
    list.alignment = NSLayoutAttributeWidth;
    list.translatesAutoresizingMaskIntoConstraints = NO;
    c.list = list;

    NSScrollView *scroll = [[NSScrollView alloc] init];
    scroll.hasVerticalScroller = YES;
    scroll.borderType = NSNoBorder;
    scroll.drawsBackground = NO;
    scroll.translatesAutoresizingMaskIntoConstraints = NO;
    scroll.documentView = list;

    NSTextField *hintLabel = [NSTextField wrappingLabelWithString:nsHint];
    hintLabel.font = [NSFont systemFontOfSize:11];
    hintLabel.textColor = NSColor.tertiaryLabelColor;
    hintLabel.translatesAutoresizingMaskIntoConstraints = NO;
    hintLabel.preferredMaxLayoutWidth = 560;
    c.hintLabel = hintLabel;

    NSButton *done = [NSButton buttonWithTitle:nsBtnDone target:c action:@selector(onDone:)];
    done.bezelStyle = NSBezelStyleRounded;
    done.keyEquivalent = @"\r";
    done.translatesAutoresizingMaskIntoConstraints = NO;

    [content addSubview:sub];
    [content addSubview:scroll];
    [content addSubview:hintLabel];
    [content addSubview:done];

    [NSLayoutConstraint activateConstraints:@[
      [sub.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:20],
      [sub.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-20],
      [sub.topAnchor constraintEqualToAnchor:content.topAnchor constant:16],

      [scroll.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:20],
      [scroll.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-20],
      [scroll.topAnchor constraintEqualToAnchor:sub.bottomAnchor constant:14],
      [scroll.bottomAnchor constraintEqualToAnchor:hintLabel.topAnchor constant:-12],

      [list.leadingAnchor constraintEqualToAnchor:scroll.contentView.leadingAnchor],
      [list.trailingAnchor constraintEqualToAnchor:scroll.contentView.trailingAnchor],
      [list.topAnchor constraintEqualToAnchor:scroll.contentView.topAnchor],
      [list.widthAnchor constraintEqualToAnchor:scroll.contentView.widthAnchor],

      [hintLabel.leadingAnchor constraintEqualToAnchor:content.leadingAnchor constant:20],
      [hintLabel.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-20],
      [hintLabel.bottomAnchor constraintEqualToAnchor:done.topAnchor constant:-12],

      [done.trailingAnchor constraintEqualToAnchor:content.trailingAnchor constant:-20],
      [done.bottomAnchor constraintEqualToAnchor:content.bottomAnchor constant:-16],
    ]];

    [c rebuild];
    c.timer = [NSTimer scheduledTimerWithTimeInterval:1.5
                                               target:c
                                             selector:@selector(onTick:)
                                             userInfo:nil
                                              repeats:YES];
    [[NSRunLoop mainRunLoop] addTimer:c.timer forMode:NSRunLoopCommonModes];

    [win center];
    [win makeKeyAndOrderFront:nil];
    [NSApp activateIgnoringOtherApps:YES];
  };

  if ([NSThread isMainThread]) {
    build();
  } else {
    dispatch_async(dispatch_get_main_queue(), build);
  }
}
