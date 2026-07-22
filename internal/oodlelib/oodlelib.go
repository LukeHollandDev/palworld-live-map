// Package oodlelib resolves the operator-provided Oodle runtime used to read
// Palworld's compressed saves. The application never embeds or redistributes
// that proprietary library.
package oodlelib

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxLibraryBytes = int64(64 << 20)

type Options struct {
	LibraryPath string
	DownloadURL string
	SHA256      string
	CacheDir    string
	HTTPClient  *http.Client
}

// Resolve validates a local library or downloads an explicitly configured,
// hash-pinned library into the application cache. DownloadURL has no default:
// opting in and establishing redistribution rights remain operator decisions.
func Resolve(ctx context.Context, options Options) (string, error) {
	if path := strings.TrimSpace(options.LibraryPath); path != "" {
		if !filepath.IsAbs(path) {
			return "", errors.New("Oodle library path must be absolute")
		}
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("inspect Oodle library: %w", err)
		}
		if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxLibraryBytes {
			return "", errors.New("Oodle library must be a non-empty regular file no larger than 64 MiB")
		}
		return path, nil
	}

	downloadURL := strings.TrimSpace(options.DownloadURL)
	expected, err := hex.DecodeString(strings.TrimSpace(options.SHA256))
	if err != nil || len(expected) != sha256.Size {
		return "", errors.New("Oodle download requires a valid SHA-256 digest")
	}
	parsed, err := url.Parse(downloadURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return "", errors.New("Oodle download URL must be absolute HTTPS without credentials")
	}
	cacheDir := strings.TrimSpace(options.CacheDir)
	if !filepath.IsAbs(cacheDir) {
		return "", errors.New("Oodle cache directory must be absolute")
	}
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return "", fmt.Errorf("create Oodle cache: %w", err)
	}
	cachePath := filepath.Join(cacheDir, "liboodle-"+hex.EncodeToString(expected[:8])+".so")
	if valid, err := fileMatches(cachePath, expected); err != nil {
		return "", err
	} else if valid {
		return cachePath, nil
	}

	client := options.HTTPClient
	if client == nil {
		client = secureHTTPClient()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("create Oodle download request: %w", err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("download Oodle library: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return "", fmt.Errorf("download Oodle library: upstream returned %s", response.Status)
	}
	if response.ContentLength > maxLibraryBytes {
		return "", errors.New("download Oodle library: response exceeds 64 MiB")
	}

	temporary, err := os.CreateTemp(cacheDir, ".oodle-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary Oodle library: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(response.Body, maxLibraryBytes+1))
	if copyErr != nil {
		return "", fmt.Errorf("write Oodle library: %w", copyErr)
	}
	if written == 0 || written > maxLibraryBytes {
		return "", errors.New("download Oodle library: invalid response size")
	}
	if subtle.ConstantTimeCompare(hash.Sum(nil), expected) != 1 {
		return "", errors.New("download Oodle library: SHA-256 mismatch")
	}
	if err := temporary.Sync(); err != nil {
		return "", fmt.Errorf("sync Oodle library: %w", err)
	}
	if err := temporary.Chmod(0o500); err != nil {
		return "", fmt.Errorf("set Oodle library permissions: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("close Oodle library: %w", err)
	}
	if err := os.Rename(temporaryPath, cachePath); err != nil {
		return "", fmt.Errorf("publish Oodle library: %w", err)
	}
	committed = true
	return cachePath, nil
}

func secureHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 45 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if request.URL.Scheme != "https" || request.URL.User != nil {
				return errors.New("Oodle download redirect must use HTTPS without credentials")
			}
			return nil
		},
	}
}

func fileMatches(path string, expected []byte) (bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("open cached Oodle library: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return false, fmt.Errorf("inspect cached Oodle library: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxLibraryBytes {
		return false, errors.New("cached Oodle library is not a valid regular file")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(file, maxLibraryBytes+1)); err != nil {
		return false, fmt.Errorf("hash cached Oodle library: %w", err)
	}
	return subtle.ConstantTimeCompare(hash.Sum(nil), expected) == 1, nil
}
