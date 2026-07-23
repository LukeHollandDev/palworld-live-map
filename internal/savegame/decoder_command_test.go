//go:build !windows

package savegame

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeDecoderScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "decoder")
	content := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCommandDecoderReturnsBoundedOutputWithoutEnvironment(t *testing.T) {
	t.Setenv("PALWORLD_ADMIN_PASSWORD", "must-not-reach-helper")
	path := writeDecoderScript(t, `
[ "$1" = "--raw-size" ] || exit 3
[ "$2" = "4" ] || exit 4
[ -z "${PALWORLD_ADMIN_PASSWORD+x}" ] || exit 5
printf 'GVAS'
`)
	decoder, err := loadCommandDecoder(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decoder.Decompress([]byte{1, 2, 3}, 4)
	if err != nil || string(got) != "GVAS" {
		t.Fatalf("Decompress() = %q, %v", got, err)
	}
}

func TestCommandDecoderRejectsExcessOutput(t *testing.T) {
	path := writeDecoderScript(t, "printf 'GVAS-extra'")
	decoder, err := loadCommandDecoder(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = decoder.Decompress([]byte{1}, 4)
	if err == nil || !strings.Contains(err.Error(), "exceeds declared") {
		t.Fatalf("Decompress() error = %v", err)
	}
}

func TestCommandDecoderBoundsStderr(t *testing.T) {
	path := writeDecoderScript(t, "printf 'decoder rejected input' >&2\nexit 2")
	decoder, err := loadCommandDecoder(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = decoder.Decompress([]byte{1}, 4)
	if err == nil || !strings.Contains(err.Error(), "decoder rejected input") {
		t.Fatalf("Decompress() error = %v", err)
	}
}

func TestCommandDecoderTimesOut(t *testing.T) {
	path := writeDecoderScript(t, "while :; do :; done")
	decoder, err := loadCommandDecoder(path, 25*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	_, err = decoder.Decompress([]byte{1}, 4)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("Decompress() error = %v", err)
	}
}

func TestCommandDecoderRejectsSymlink(t *testing.T) {
	target := writeDecoderScript(t, "printf 'GVAS'")
	link := filepath.Join(t.TempDir(), "decoder-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	_, err := loadCommandDecoder(link, time.Second)
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("loadCommandDecoder() error = %v", err)
	}
}
