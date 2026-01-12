package tools

import (
	"fmt"
	"net/url"
	"strings"
)

func CleanHostURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	rawURL = strings.TrimPrefix(rawURL, "http://")
	rawURL = strings.TrimPrefix(rawURL, "https://")
	return rawURL
}

func IsHttpsURL(rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	return strings.HasPrefix(rawURL, "https://")
}

func IsLocalhostURL(rawURL string) bool {
	rawURL = CleanHostURL(rawURL)
	host := strings.SplitN(rawURL, "/", 2)[0]
	return host == "localhost" || host == "127.0.0.1"
}

func ToQueryParams(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}

	var queryParams []string
	for key, value := range params {
		if value != "" {
			queryParams = append(queryParams, key+"="+value)
		}
	}

	return strings.Join(queryParams, "&")
}

func IsLocalhost(urlOrHost string) bool {

	u, err := url.Parse(urlOrHost)
	var host string

	if err == nil && u.Host != "" {
		host = u.Hostname()
	} else {

		host = urlOrHost

		if idx := strings.LastIndex(host, ":"); idx != -1 {
			possiblePort := host[idx+1:]

			if len(possiblePort) > 0 && possiblePort[0] >= '0' && possiblePort[0] <= '9' {
				host = host[:idx]
			}
		}
	}

	host = strings.ToLower(strings.TrimSpace(host))

	localhosts := []string{
		"localhost",
		"127.0.0.1",
		"::1",
		"0.0.0.0",
		"[::1]",
	}

	for _, lh := range localhosts {
		if host == lh {
			return true
		}
	}

	if strings.HasPrefix(host, "127.") {
		return true
	}

	return false
}

func IsHTTPS(urlOrHost string) bool {

	if !strings.Contains(urlOrHost, "://") {

		return false
	}

	u, err := url.Parse(urlOrHost)
	if err != nil {
		return false
	}

	return strings.ToLower(u.Scheme) == "https"
}

func ToCompleteURL(urlOrHost string) string {
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

	completeURL := fmt.Sprintf("%s://%s", scheme, urlOrHost)

	if _, err := url.Parse(completeURL); err != nil {

		return urlOrHost
	}

	return completeURL
}
