package main

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// privateRanges defines CIDR blocks for private/internal networks.
var privateRanges []*net.IPNet

func init() {
	cidrs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}
	for _, cidr := range cidrs {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("bad CIDR %s: %v", cidr, err))
		}
		privateRanges = append(privateRanges, block)
	}
}

// isPrivateIP reports whether ip falls within a private/internal range.
func isPrivateIP(ip net.IP) bool {
	for _, block := range privateRanges {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// blockedHostnames contains common internal hostnames that must be rejected.
var blockedHostnames = []string{
	"localhost",
	"metadata.google.internal",
	"metadata",
	"kubernetes.default",
	"kubernetes",
}

// validateTunnelURL checks that rawURL is a safe, externally-routable HTTPS URL.
func validateTunnelURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid tunnel_url: %v", err)
	}

	// Must have a scheme and host.
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("tunnel_url must include scheme and host")
	}

	// Only HTTPS is allowed.
	if u.Scheme != "https" {
		return fmt.Errorf("tunnel_url must use https scheme, got %q", u.Scheme)
	}

	hostname := u.Hostname() // strips port if present

	// Reject known internal hostnames.
	lower := strings.ToLower(hostname)
	for _, blocked := range blockedHostnames {
		if lower == blocked {
			return fmt.Errorf("tunnel_url hostname %q is not allowed", hostname)
		}
	}

	// Trusted tunnel providers — skip DNS resolution (quick tunnels may not
	// have propagated yet).
	trustedSuffixes := []string{".trycloudflare.com", ".ngrok.io", ".ngrok-free.app"}
	trusted := false
	for _, suffix := range trustedSuffixes {
		if strings.HasSuffix(lower, suffix) {
			trusted = true
			break
		}
	}

	if !trusted {
		// Resolve the hostname and check every returned IP.
		ips, err := net.LookupIP(hostname)
		if err != nil {
			return fmt.Errorf("cannot resolve tunnel_url host %q: %v", hostname, err)
		}

		for _, ip := range ips {
			if isPrivateIP(ip) {
				return fmt.Errorf("tunnel_url resolves to private/internal address %s", ip)
			}
		}
	}

	return nil
}
