#ifndef RE_TRAY_PERMS_PANEL_DARWIN_H
#define RE_TRAY_PERMS_PANEL_DARWIN_H

#ifdef __cplusplus
extern "C" {
#endif

/* Show (or raise) the native permissions window. Strings are copied. */
void re_perms_panel_show(const char *title, const char *subtitle,
                         const char *btn_settings, const char *btn_done,
                         const char *hint);

/* Implemented in Go (//export). Caller must free the returned C string. */
char *rePermsStatusJSON(void);
void rePermsOpenID(char *id);

#ifdef __cplusplus
}
#endif

#endif
