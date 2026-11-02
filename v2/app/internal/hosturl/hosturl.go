// Package hosturl turns the configured host and port into the URLs the API
// is reached at: http for local addresses, https for everything else.
package hosturl

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// localHosts are the hostnames that always mean this machine.
var localHosts = []string{"localhost", "127.0.0.1", "::1", "0.0.0.0", "[::1]"}

// CleanHost removes an http:// or https:// prefix.
//
//	CleanHost("https://example.com")   // "example.com"
//	CleanHost("http://localhost:8080") // "localhost:8080"
func CleanHost(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	rawURL = strings.TrimPrefix(rawURL, "http://")
	return strings.TrimPrefix(rawURL, "https://")
}

// IsLocalhost reports whether a URL or host points at this machine: localhost,
// any 127.x.x.x address, ::1 or 0.0.0.0, with or without scheme and port.
func IsLocalhost(urlOrHost string) bool {
	var host string
	if u, err := url.Parse(urlOrHost); err == nil && u.Host != "" {
		host = u.Hostname()
	} else {
		host = urlOrHost
		if idx := strings.LastIndex(host, ":"); idx != -1 {
			if port := host[idx+1:]; len(port) > 0 && port[0] >= '0' && port[0] <= '9' {
				host = host[:idx]
			}
		}
	}

	host = strings.ToLower(strings.TrimSpace(host))

	return slices.Contains(localHosts, host) || strings.HasPrefix(host, "127.")
}

// IsHTTPS reports whether a full URL uses https. A host without a scheme is
// not https.
func IsHTTPS(urlOrHost string) bool {
	if !strings.Contains(urlOrHost, "://") {
		return false
	}

	u, err := url.Parse(urlOrHost)
	if err != nil {
		return false
	}

	return strings.EqualFold(u.Scheme, "https")
}

// Complete adds a scheme to a host: http for local addresses, https for the
// rest. Input that already has a scheme is returned unchanged.
//
//	Complete("example.com")    // "https://example.com"
//	Complete("localhost:8080") // "http://localhost:8080"
func Complete(urlOrHost string) string {
	urlOrHost = strings.TrimSpace(urlOrHost)

	if strings.Contains(urlOrHost, "://") {
		if u, err := url.Parse(urlOrHost); err == nil && u.Scheme != "" {
			return urlOrHost
		}
	}

	scheme := "https"
	if IsLocalhost(urlOrHost) {
		scheme = "http"
	}

	completed := fmt.Sprintf("%s://%s", scheme, urlOrHost)
	if _, err := url.Parse(completed); err != nil {
		return urlOrHost
	}

	return completed
}

// AppURL is the base URL of the API. The port is only part of it for local
// addresses; anything else is assumed to sit behind a proxy on the default
// port.
//
//	AppURL("localhost", 8080)   // "http://localhost:8080"
//	AppURL("example.com", 443)  // "https://example.com"
func AppURL(host string, port int) string {
	host = CleanHost(host)

	if IsLocalhost(host) {
		return fmt.Sprintf("http://%s:%d", host, port)
	}

	return Complete(host)
}

// DocsURL is where Swagger UI is served.
func DocsURL(host string, port int) string {
	return AppURL(host, port) + "/docs/index.html"
}
