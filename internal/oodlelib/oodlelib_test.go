package oodlelib

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestResolveAcceptsExplicitRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "liboodle.so")
	if err := os.WriteFile(path, []byte("operator supplied"), 0o400); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(context.Background(), Options{LibraryPath: path})
	if err != nil || got != path {
		t.Fatalf("Resolve() = %q, %v", got, err)
	}
}

func TestResolveDownloadsPinnedLibraryOnce(t *testing.T) {
	content := []byte("test Oodle library bytes")
	digest := sha256.Sum256(content)
	var requests atomic.Int32
	server := httptest.NewTLSServer(httpHandler(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write(content)
	}))
	defer server.Close()
	options := Options{
		DownloadURL: server.URL,
		SHA256:      hex.EncodeToString(digest[:]),
		CacheDir:    t.TempDir(),
		HTTPClient:  server.Client(),
	}
	first, err := Resolve(context.Background(), options)
	if err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}
	second, err := Resolve(context.Background(), options)
	if err != nil || second != first || requests.Load() != 1 {
		t.Fatalf("second Resolve() = %q, %v; requests = %d", second, err, requests.Load())
	}
	got, err := os.ReadFile(first)
	if err != nil || string(got) != string(content) {
		t.Fatalf("cached content = %q, %v", got, err)
	}
}

func TestResolveRejectsHashMismatchWithoutPublishing(t *testing.T) {
	server := httptest.NewTLSServer(httpHandler(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("unexpected"))
	}))
	defer server.Close()
	cache := t.TempDir()
	_, err := Resolve(context.Background(), Options{
		DownloadURL: server.URL,
		SHA256:      strings.Repeat("00", sha256.Size),
		CacheDir:    cache,
		HTTPClient:  server.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("Resolve() error = %v", err)
	}
	entries, readErr := os.ReadDir(cache)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("cache entries = %v, error = %v", entries, readErr)
	}
}

type httpHandler func(http.ResponseWriter, *http.Request)

func (handler httpHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	handler(w, request)
}
