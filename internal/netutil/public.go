package netutil

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"
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
			// prefer IPv4 first by appending v4 before collecting; handle below
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

// LookupPublicIP asks an external echo service for the outbound public IP.
func LookupPublicIP(ctx context.Context) (string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	urls := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
	}
	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", "runeverything/0.1")
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
		if net.ParseIP(ipStr) != nil {
			return ipStr, nil
		}
	}
	return "", os.ErrNotExist
}

// DetectPublicHost picks a host suitable for client-facing URLs.
// Linux prefers public addressing; other OSes only rewrite when a local public IP exists.
func DetectPublicHost(ctx context.Context) string {
	if v := strings.TrimSpace(os.Getenv("RE_PUBLIC_HOST")); v != "" {
		return v
	}
	locals := LocalPublicIPs()
	if len(locals) > 0 {
		return locals[0].String()
	}
	// Linux / hosts serving :80 are usually internet-facing (cloud EIP, DNAT).
	if runtime.GOOS == "linux" || PortListening("80") {
		ip, err := LookupPublicIP(ctx)
		if err == nil && ip != "" {
			return ip
		}
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
