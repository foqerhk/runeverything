package netutil

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/foqerhk/runeverything/internal/region"
)

// IsLoopbackHost reports whether host is empty, localhost, or a loopback IP.
func IsLoopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || host == "localhost" {
		return true
	}
	// strip port if present
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return strings.EqualFold(host, "localhost")
}

// IsPrivateOrLocalIP is true for loopback, link-local, and RFC1918/ULA addresses.
func IsPrivateOrLocalIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	return ip.IsPrivate()
}

// LocalPublicIPs returns non-private IPv4/IPv6 addresses on up interfaces.
func LocalPublicIPs() []net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []net.IP
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.ToLower(iface.Name)
		// skip common virtual / container bridges
		if strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "br-") ||
			strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "virbr") ||
			strings.HasPrefix(name, "cni") || strings.HasPrefix(name, "flannel") {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || IsPrivateOrLocalIP(ip) {
				continue
			}
			out = append(out, ip.To16())
		}
	}
	// stable-ish: IPv4 first
	var v4, v6 []net.IP
	for _, ip := range out {
		if x := ip.To4(); x != nil {
			v4 = append(v4, x)
		} else {
			v6 = append(v6, ip)
		}
	}
	return append(v4, v6...)
}

// PortListening reports whether something accepts TCP on local :port
// (often :80 on public Linux hosts).
func PortListening(port string) bool {
	d := net.Dialer{Timeout: 400 * time.Millisecond}
	for _, addr := range []string{"127.0.0.1:" + port, "[::1]:" + port} {
		c, err := d.Dial("tcp", addr)
		if err == nil {
			_ = c.Close()
			return true
		}
	}
	for _, ip := range LocalPublicIPs() {
		c, err := d.Dial("tcp", net.JoinHostPort(ip.String(), port))
		if err == nil {
			_ = c.Close()
			return true
		}
	}
	return false
}

// Mainland China egress-IP echo services (plain-text IP body).
var echoURLsCN = []string{
	"https://ip.3322.net",
	"https://myip.ipip.net/s",
}

// International egress-IP echo services (plain-text IP body).
var echoURLsIntl = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
}

// Optional overrides from config.json (env still wins when set).
var (
	echoOverrideCN   []string
	echoOverrideIntl []string
)

// DefaultEchoURLsCN returns built-in mainland echo endpoints.
func DefaultEchoURLsCN() []string { return append([]string(nil), echoURLsCN...) }

// DefaultEchoURLsIntl returns built-in international echo endpoints.
func DefaultEchoURLsIntl() []string { return append([]string(nil), echoURLsIntl...) }

// SetEchoURLOverrides installs config.json overrides (empty slices clear).
// Call at process start after LoadConfig. Env RE_IP_ECHO_CN / RE_IP_ECHO_INTL still win.
func SetEchoURLOverrides(cn, intl []string) {
	echoOverrideCN = sanitizeURLList(cn)
	echoOverrideIntl = sanitizeURLList(intl)
}

func sanitizeURLList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, u := range in {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		out = append(out, u)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ParseEchoURLList splits a comma/space separated URL list.
func ParseEchoURLList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	})
	return sanitizeURLList(parts)
}

func effectiveEchoCN() []string {
	if v := strings.TrimSpace(os.Getenv("RE_IP_ECHO_CN")); v != "" {
		if list := ParseEchoURLList(v); len(list) > 0 {
			return list
		}
	}
	if len(echoOverrideCN) > 0 {
		return echoOverrideCN
	}
	return echoURLsCN
}

func effectiveEchoIntl() []string {
	if v := strings.TrimSpace(os.Getenv("RE_IP_ECHO_INTL")); v != "" {
		if list := ParseEchoURLList(v); len(list) > 0 {
			return list
		}
	}
	if len(echoOverrideIntl) > 0 {
		return echoOverrideIntl
	}
	return echoURLsIntl
}

// echoURLsOrdered returns preferred echo endpoints for the region, with the
// other region as mutual fallback (CN prefers CN first; intl prefers intl first).
func echoURLsOrdered(reg string) []string {
	cn := effectiveEchoCN()
	intl := effectiveEchoIntl()
	if reg == region.CN {
		out := make([]string, 0, len(cn)+len(intl))
		out = append(out, cn...)
		out = append(out, intl...)
		return out
	}
	out := make([]string, 0, len(cn)+len(intl))
	out = append(out, intl...)
	out = append(out, cn...)
	return out
}

// ActiveEchoURLs reports the effective CN/intl lists (after env + config overrides).
func ActiveEchoURLs() (cn, intl []string) {
	return append([]string(nil), effectiveEchoCN()...), append([]string(nil), effectiveEchoIntl()...)
}

// LookupPublicIP asks an external echo service for the outbound public IP.
// Region (cn/intl) picks preferred endpoints; the other pair is tried as fallback
// so DNS pollution or blocked hosts do not break NAT detection.
func LookupPublicIP(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	urls := echoURLsOrdered(region.Detect(ctx))
	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "runeverything/0.2")
		req.Header.Set("Accept", "text/plain")
		res, err := client.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(res.Body, 64))
		_ = res.Body.Close()
		if res.StatusCode != 200 {
			continue
		}
		ipStr := strings.TrimSpace(string(body))
		// Some providers append a trailing CR or comment line; take first token.
		if i := strings.IndexAny(ipStr, " \t\r\n"); i >= 0 {
			ipStr = ipStr[:i]
		}
		if net.ParseIP(ipStr) != nil {
			return ipStr, nil
		}
	}
	return "", os.ErrNotExist
}

// DirectPublicIP reports the machine's own public IP when it is NOT behind NAT.
//
// Rule (OS-agnostic): the egress IP returned by an echo service must also be
// assigned to a local interface. Shared carrier/home NAT egress IPs are rejected.
//
// If the echo service is unreachable but a non-private address is on a local
// NIC, that address is used (typical locked-down server that still has a public
// binding). Otherwise the host is treated as behind NAT.
func DirectPublicIP(ctx context.Context) (string, bool) {
	return directPublicIP(ctx, LookupPublicIP, LocalPublicIPs)
}

func directPublicIP(ctx context.Context, lookup func(context.Context) (string, error), localsFn func() []net.IP) (string, bool) {
	locals := localsFn()
	egress, err := lookup(ctx)
	if err == nil && egress != "" {
		eip := net.ParseIP(strings.TrimSpace(egress))
		if eip == nil || IsPrivateOrLocalIP(eip) {
			return "", false
		}
		for _, lip := range locals {
			if lip != nil && lip.Equal(eip) {
				if v4 := eip.To4(); v4 != nil {
					return v4.String(), true
				}
				return eip.String(), true
			}
		}
		// Egress is public but not on any local NIC → classic NAT / CGNAT.
		return "", false
	}
	// No egress echo: fall back to a local public binding if present.
	if len(locals) > 0 {
		ip := locals[0]
		if v4 := ip.To4(); v4 != nil {
			return v4.String(), true
		}
		return ip.String(), true
	}
	return "", false
}

// BehindNAT is true when this host does not own a direct public IP on a NIC
// (typical home / office NAT). Independent of GOOS.
func BehindNAT(ctx context.Context) bool {
	_, ok := DirectPublicIP(ctx)
	return !ok
}

// DetectPublicHost picks a host suitable for client-facing URLs.
// Only returns an address when the host is not behind NAT (see DirectPublicIP).
func DetectPublicHost(ctx context.Context) string {
	if v := strings.TrimSpace(os.Getenv("RE_PUBLIC_HOST")); v != "" {
		return v
	}
	if ip, ok := DirectPublicIP(ctx); ok {
		return ip
	}
	return ""
}

// RewriteLoopbackRelayHost replaces a loopback relay URL host with publicHost.
// Non-loopback URLs are returned unchanged. Empty publicHost returns raw unchanged.
func RewriteLoopbackRelayHost(raw, publicHost string) string {
	if publicHost == "" || raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	host := u.Hostname()
	if !IsLoopbackHost(host) {
		return raw
	}
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "wss", "https":
			port = "443"
		default:
			port = "80"
		}
	}
	u.Host = net.JoinHostPort(publicHost, port)
	return u.String()
}

// ResolveClientRelay returns the relay URL to embed in QR / deep link.
// Agent connect URL (relay_url) is left alone; only the advertised URL is rewritten.
// Rewrites loopback → DirectPublicIP only when not behind NAT.
func ResolveClientRelay(publicRelay, relayURL string) string {
	adv := publicRelay
	if adv == "" {
		adv = relayURL
	}
	host := ""
	if u, err := url.Parse(adv); err == nil {
		host = u.Hostname()
	}
	if !IsLoopbackHost(host) {
		return adv
	}
	// Explicit opt-out
	if os.Getenv("RE_PUBLIC_HOST") == "localhost" || os.Getenv("RE_FORCE_LOCAL_RELAY") == "1" {
		return adv
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pub := DetectPublicHost(ctx)
	if pub == "" {
		return adv
	}
	return RewriteLoopbackRelayHost(adv, pub)
}

// AdvertiseRelayURL builds a client-facing relay URL for a direct public IP,
// preserving scheme/port/path from base (typically the local loopback relay).
func AdvertiseRelayURL(base, publicIP string) string {
	if publicIP == "" {
		return base
	}
	raw := strings.TrimSpace(base)
	if raw == "" {
		raw = "ws://127.0.0.1:8787/re2"
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "ws://" + net.JoinHostPort(publicIP, "8787") + "/re2"
	}
	port := u.Port()
	if port == "" {
		switch u.Scheme {
		case "wss", "https":
			port = "443"
		case "ws", "http":
			port = "80"
		default:
			port = "8787"
		}
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "ws"
	}
	path := u.Path
	if path == "" || path == "/" {
		path = "/re2"
	}
	return scheme + "://" + net.JoinHostPort(publicIP, port) + path
}
