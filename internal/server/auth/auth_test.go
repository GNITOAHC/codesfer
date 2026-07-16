package auth

import "testing"

func TestShortUserAgent(t *testing.T) {
	cases := map[string]string{
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36":                   "Chrome (macOS)",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Edg/126.0.0.0":           "Edge (Windows)",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1": "Safari (iOS)",
		"Mozilla/5.0 (X11; Linux x86_64; rv:127.0) Gecko/20100101 Firefox/127.0":                                                                  "Firefox (Linux)",
		"curl/8.6.0":                "curl (Unknown)",
		"":                          "Unknown",
		"Go-http-client/2.0":        "Go-http-client/2.0",
		"codesfer-cli/dev (darwin)": "codesfer-cli/dev (darwin)",
	}
	for ua, want := range cases {
		if got := shortUserAgent(ua); got != want {
			t.Errorf("shortUserAgent(%q) = %q, want %q", ua, got, want)
		}
	}
}
