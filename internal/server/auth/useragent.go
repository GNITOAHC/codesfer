package auth

import "strings"

// shortUserAgent reduces a full User-Agent string to "Browser (OS)".
func shortUserAgent(ua string) string {
	if ua == "" {
		return "Unknown"
	}
	if strings.HasPrefix(ua, "codesfer-cli/") {
		return ua
	}
	browser := "Unknown"
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/"):
		browser = "Opera"
	case strings.Contains(ua, "Chrome/"):
		browser = "Chrome"
	case strings.Contains(ua, "Firefox/"):
		browser = "Firefox"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	case strings.Contains(ua, "curl/"):
		browser = "curl"
	}
	os := "Unknown"
	switch {
	case strings.Contains(ua, "Android"):
		os = "Android"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"):
		os = "iOS"
	case strings.Contains(ua, "Windows"):
		os = "Windows"
	case strings.Contains(ua, "Mac OS X"):
		os = "macOS"
	case strings.Contains(ua, "Linux"):
		os = "Linux"
	}
	if browser == "Unknown" && os == "Unknown" {
		// Unrecognized client (e.g. "Go-http-client/2.0"): raw UA beats "Unknown (Unknown)"
		return ua
	}
	return browser + " (" + os + ")"
}
