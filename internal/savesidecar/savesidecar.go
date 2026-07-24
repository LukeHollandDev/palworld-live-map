package savesidecar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// defaultBinaryName is the decoder binary the server looks for next to its
	// own executable when no explicit path is configured.
	defaultBinaryName = "savedecode"
	// maxOutputBytes bounds the decoder's stdout. The roster document for a
	// real world is a few kilobytes; this cap is defense in depth against a
	// misbehaving or misinvoked decoder, not an expected size.
	maxOutputBytes = 16 << 20
	// maxStderrBytes bounds how much of the decoder's stderr is retained for
	// error messages.
	maxStderrBytes = 8 << 10
)

// Options configures a Reader.
type Options struct {
	// BinaryPath is the absolute path to the "savedecode" binary. When empty,
	// the Reader looks for a binary named "savedecode" alongside the running
	// server executable.
	BinaryPath string
	// MaxOutputBytes overrides the stdout cap. Zero uses the default.
	MaxOutputBytes int64
}

// Reader decodes a save-snapshot directory by executing the external decoder
// binary and parsing its roster JSON. It implements the snapshot-reader
// contract consumed by internal/saveroster.
type Reader struct {
	binary    string
	maxOutput int64
}

// NewReader resolves and validates the decoder binary so misconfiguration
// fails fast at startup rather than on the first poll.
func NewReader(options Options) (*Reader, error) {
	binary := strings.TrimSpace(options.BinaryPath)
	if binary == "" {
		resolved, err := binaryNextToExecutable()
		if err != nil {
			return nil, err
		}
		binary = resolved
	}
	if !filepath.IsAbs(binary) {
		return nil, errors.New("save decoder path must be absolute")
	}
	info, err := os.Stat(binary)
	if err != nil {
		return nil, fmt.Errorf("locate save decoder: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("save decoder is not a regular file: %s", binary)
	}
	maxOutput := options.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = maxOutputBytes
	}
	return &Reader{binary: binary, maxOutput: maxOutput}, nil
}

func binaryNextToExecutable() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate server executable: %w", err)
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return "", fmt.Errorf("resolve server executable: %w", err)
	}
	return filepath.Join(filepath.Dir(self), defaultBinaryName), nil
}

// ReadSnapshot execs the decoder against dir (a directory holding Level.sav
// and a Players/ directory) and decodes the roster JSON it prints to stdout.
// The provided context bounds the subprocess lifetime.
func (r *Reader) ReadSnapshot(ctx context.Context, dir string) (*Snapshot, error) {
	if ctx == nil {
		return nil, errors.New("save decoder requires a context")
	}
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("save decoder requires a snapshot directory")
	}

	cmd := exec.CommandContext(ctx, r.binary, dir)
	stderr := &cappedBuffer{limit: maxStderrBytes}
	cmd.Stderr = stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("prepare save decoder: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start save decoder: %w", err)
	}

	// Read at most maxOutput+1 bytes so an overrun is detectable.
	data, readErr := io.ReadAll(io.LimitReader(stdout, r.maxOutput+1))
	waitErr := cmd.Wait()

	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if waitErr != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return nil, fmt.Errorf("save decoder failed: %w: %s", waitErr, message)
		}
		return nil, fmt.Errorf("save decoder failed: %w", waitErr)
	}
	if readErr != nil {
		return nil, fmt.Errorf("read save decoder output: %w", readErr)
	}
	if int64(len(data)) > r.maxOutput {
		return nil, fmt.Errorf("save decoder output exceeds %d bytes", r.maxOutput)
	}

	return Decode(data)
}

// Decode parses a roster JSON document. It is separated from the exec path so
// the contract can be tested against fixtures without a decoder binary.
func Decode(data []byte) (*Snapshot, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("save decoder produced no output")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var snapshot Snapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return nil, fmt.Errorf("decode save roster JSON: %w", err)
	}
	if snapshot.Players == nil {
		snapshot.Players = []Player{}
	}
	return &snapshot, nil
}

// cappedBuffer accumulates up to limit bytes and silently discards the rest.
type cappedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if remaining := c.limit - c.buf.Len(); remaining > 0 {
		if len(p) > remaining {
			c.buf.Write(p[:remaining])
		} else {
			c.buf.Write(p)
		}
	}
	// Report the full length so the writer does not treat this as a short write.
	return len(p), nil
}

func (c *cappedBuffer) String() string { return c.buf.String() }
