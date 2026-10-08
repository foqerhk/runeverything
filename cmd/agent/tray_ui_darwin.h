#ifndef RE_TRAY_UI_DARWIN_H
#define RE_TRAY_UI_DARWIN_H

#ifdef __cplusplus
extern "C" {
#endif

void re_ui_set_app_icon_png(const unsigned char *data, int len);
void re_ui_show_text_window(const char *title, const char *body, const char *btn_close, int width);
void re_ui_show_about_window(const char *title, const char *app_name, const char *version_line,
                             const unsigned char *icon_png, int icon_len,
                             const char *github_url, const char *contact_email,
                             const char *btn_check, const char *btn_close);
void re_ui_show_config_window(const char *title, const char *relay_label, const char *public_label,
                              const char *manual_label, const char *manual_hint,
                              const char *share_label, const char *share_hint,
                              const char *relay, const char *pub,
                              int manual, int share,
                              const char *btn_save, const char *btn_cancel);
/* Show pairing QR. Auto-refresh is driven from Go (reUIRequestPairQRRefresh / timer). */
void re_ui_show_qr_window(const char *title, const unsigned char *png, int png_len,
                          long long expires_at, const char *link,
                          const char *expires_fmt, const char *refreshing_text,
                          const char *btn_refresh, const char *btn_close);
/* Apply a newly minted QR (called from Go on the ObjC main queue path). err may be empty. */
void re_ui_qr_apply(const unsigned char *png, int png_len, long long expires_at,
                    const char *link, const char *err);
/* Close pairing QR after a successful phone connect (Noise / desktop open). */
void re_ui_close_qr_window(void);

/* Go exports */
char *reUIStatusText(void);
char *reUIHelpText(void);
char *reUIAboutVersion(void);
char *reUICheckUpdate(void); /* returns "1|<msg>|<url>" or "0|<msg>|" */
void reUIOpenURL(char *url);
char *reUIConfigLoad(void); /* "relay\x1fpublic\x1fmanual\x1fshare" */
char *reUIConfigSave(char *relay, char *pub, int manual, int share);
/* Non-blocking: ask Go goroutine to mint a new QR (safe from AppKit / GCD threads). */
void reUIRequestPairQRRefresh(void);
void reUIQRWindowClosed(void);

#ifdef __cplusplus
}
#endif

#endif
