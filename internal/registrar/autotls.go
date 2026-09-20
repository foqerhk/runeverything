package registrar

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/caddyserver/certmagic"
)

// ServeAutoTLS serves mux on :443 with ACME certs for hostname, and :80 for HTTP-01.
func ServeAutoTLS(hostname string, mux http.Handler) error {
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return fmt.Errorf("hostname required for auto TLS")
	}
	email := strings.TrimSpace(os.Getenv("RE_ACME_EMAIL"))
	certmagic.DefaultACME.Agreed = true
	if email != "" {
		certmagic.DefaultACME.Email = email
	}
	if os.Getenv("RE_ACME_STAGING") == "1" {
		certmagic.DefaultACME.CA = certmagic.LetsEncryptStagingCA
	}
	storage := &certmagic.FileStorage{Path: CertStorageDir()}
	certmagic.Default.Storage = storage
	log.Printf("auto-tls: obtaining cert for %s (ports 80/443 must be open)", hostname)
	return certmagic.HTTPS([]string{hostname}, mux)
}
