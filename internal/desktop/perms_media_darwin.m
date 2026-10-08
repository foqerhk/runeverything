//go:build darwin

#import <AVFoundation/AVFoundation.h>
#import "perms_media_darwin.h"

static int re_media_ok(AVMediaType type) {
  AVAuthorizationStatus s = [AVCaptureDevice authorizationStatusForMediaType:type];
  return s == AVAuthorizationStatusAuthorized ? 1 : 0;
}

int re_mic_authorized(void) {
  return re_media_ok(AVMediaTypeAudio);
}

int re_camera_authorized(void) {
  return re_media_ok(AVMediaTypeVideo);
}

/* Returns 1 if a system prompt was shown / request started, 0 if already decided. */
static int re_media_request(AVMediaType type) {
  AVAuthorizationStatus s = [AVCaptureDevice authorizationStatusForMediaType:type];
  if (s == AVAuthorizationStatusAuthorized) {
    return 0;
  }
  if (s == AVAuthorizationStatusNotDetermined) {
    // Needs NSMicrophoneUsageDescription / NSCameraUsageDescription in Info.plist.
    [AVCaptureDevice requestAccessForMediaType:type completionHandler:^(__unused BOOL granted){
    }];
    return 1;
  }
  // Denied / Restricted: OS will not re-prompt; caller should open Privacy pane.
  return 0;
}

void re_mic_request(void) {
  (void)re_media_request(AVMediaTypeAudio);
}

void re_camera_request(void) {
  (void)re_media_request(AVMediaTypeVideo);
}

int re_mic_can_prompt(void) {
  return [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeAudio] ==
                 AVAuthorizationStatusNotDetermined
             ? 1
             : 0;
}

int re_camera_can_prompt(void) {
  return [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeVideo] ==
                 AVAuthorizationStatusNotDetermined
             ? 1
             : 0;
}
