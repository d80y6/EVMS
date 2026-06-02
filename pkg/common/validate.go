package common

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

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
