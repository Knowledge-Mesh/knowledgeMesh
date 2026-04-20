package control

import (
	"log"
	"strings"
)

// DefaultControlURL is used when --control-url is omitted (typical local control API).
const DefaultControlURL = "http://127.0.0.1:8090"

// ResolveControlURL returns a non-empty base URL: trimmed url, or DefaultControlURL if url is empty.
// usedDefault is true when the caller did not pass a URL and the default was applied.
func ResolveControlURL(url string) (resolved string, usedDefault bool) {
	url = strings.TrimSpace(url)
	if url == "" {
		return DefaultControlURL, true
	}
	return url, false
}

// WarnIfDefaultControlURL logs a warning when ResolveControlURL applied the default.
func WarnIfDefaultControlURL(usedDefault bool, resolved string) {
	if usedDefault {
		log.Printf("warning: no --control-url specified; using default %s", resolved)
	}
}
