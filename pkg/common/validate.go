package common

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var lookupHost = net.LookupHost

func ValidateRTSPURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL is empty")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "rtsp" && scheme != "rtsps" {
		return fmt.Errorf("unsupported URL scheme %q, only rtsp:// and rtsps:// are allowed", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("URL missing host")
	}
	return nil
}

func SanitizeCameraID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || id == "." || id == ".." {
		return ""
	}
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return -1
	}, id)
}

func ValidateFilePath(path string, allowedRoot string) error {
	if path == "" {
		return fmt.Errorf("path is empty")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}
	absRoot, err := filepath.Abs(allowedRoot)
	if err != nil {
		return fmt.Errorf("failed to resolve allowed root: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			rel, err := filepath.Rel(absRoot, absPath)
			if err != nil {
				return fmt.Errorf("path outside allowed root: %w", err)
			}
			if strings.HasPrefix(rel, "..") {
				return fmt.Errorf("path %q resolves outside allowed root %q", path, allowedRoot)
			}
			resolved = absPath
		} else {
			return fmt.Errorf("failed to evaluate symlinks: %w", err)
		}
	}

	rel, err := filepath.Rel(absRoot, resolved)
	if err != nil {
		return fmt.Errorf("path %q is not under allowed root %q", path, allowedRoot)
	}
	if strings.HasPrefix(rel, "..") {
		return fmt.Errorf("path %q resolves outside allowed root %q", path, allowedRoot)
	}

	return nil
}

func ValidateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported webhook URL scheme %q, only http/https allowed", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("webhook URL has no host")
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0" {
		return fmt.Errorf("webhook URL targets loopback address: %s", host)
	}
	if strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("webhook URL targets internal domain: %s", host)
	}
	ips, err := lookupHost(host)
	if err != nil {
		return fmt.Errorf("webhook URL host %q could not be resolved: %w", host, err)
	}
	for _, ipStr := range ips {
		parsed := net.ParseIP(ipStr)
		if parsed == nil {
			continue
		}
		if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() || parsed.IsUnspecified() {
			return fmt.Errorf("webhook URL resolves to forbidden address %s (%s)", ipStr, host)
		}
	}
	return nil
}

func ValidateRecordingPath(path string) error {
	path = filepath.Clean(path)

	if filepath.Ext(path) != ".mp4" {
		return fmt.Errorf("path must end with .mp4 extension")
	}

	if strings.Contains(path, "..") {
		return fmt.Errorf("path contains parent directory reference")
	}

	if strings.ContainsAny(path, "|;&$`'\n\r") {
		return fmt.Errorf("path contains shell-unsafe characters")
	}

	return nil
}
