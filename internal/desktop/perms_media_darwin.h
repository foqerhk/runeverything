#ifndef RE_PERMS_MEDIA_DARWIN_H
#define RE_PERMS_MEDIA_DARWIN_H

#ifdef __cplusplus
extern "C" {
#endif

/* 1 = authorized/granted, 0 = not */
int re_mic_authorized(void);
int re_camera_authorized(void);
/* Fire system permission prompt when status is NotDetermined. */
void re_mic_request(void);
void re_camera_request(void);
/* 1 = Still NotDetermined (prompt possible). */
int re_mic_can_prompt(void);
int re_camera_can_prompt(void);

#ifdef __cplusplus
}
#endif

#endif
