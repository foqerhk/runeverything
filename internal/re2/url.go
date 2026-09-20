package re2

import (
	"net/url"
)

// EnsurePath rewrites the URL path to /re2 (keeps scheme/host/port; clears query).
func EnsurePath(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return raw
	}
	u.Path = "/re2"
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
