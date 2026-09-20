package registrar

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

var reportOnce sync.Map // url+reason -> last send unix

// ReportBadAsync best-effort abuse report from home agents / discovery.
// Set RE_REGISTRAR_URL (public registrar). Optional RE_REPORT=0 to disable.
func ReportBadAsync(relayURL, reason, detail, reporterID string) {
	if disabledReport() {
		return
	}
	base := strings.TrimSpace(os.Getenv("RE_REGISTRAR_URL"))
	if base == "" {
		return
	}
	relayURL = strings.TrimSpace(relayURL)
	if relayURL == "" {
		return
	}
	key := relayURL + "|" + reason
	now := time.Now().Unix()
	if v, ok := reportOnce.Load(key); ok {
		if last, _ := v.(int64); now-last < 30*60 {
			return
		}
	}
	reportOnce.Store(key, now)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		cli := &Client{BaseURL: base}
		resp, err := cli.Report(ctx, ReportRequest{
			URL:        relayURL,
			Reason:     reason,
			Detail:     detail,
			ReporterID: reporterID,
		})
		if err != nil {
			log.Printf("abuse report: %v", err)
			return
		}
		if resp != nil && resp.Revoked {
			log.Printf("abuse report: registrar revoked %s", resp.Hostname)
		}
	}()
}

func disabledReport() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("RE_REPORT")))
	return v == "0" || v == "false" || v == "off" || v == "no"
}
