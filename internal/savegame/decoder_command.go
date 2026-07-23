package savegame

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultDecoderTimeout = 20 * time.Second
	maxDecoderBytes       = int64(64 << 20)
	maxDecoderStderr      = int64(4096)
)

var errDecoderOutputLimit = errors.New("decoder output exceeds declared raw size")

type commandDecoder struct {
	path    string
	timeout time.Duration
}

func loadCommandDecoder(path string, timeout time.Duration) (saveDecompressor, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, fmt.Errorf("savegame: decoder path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("savegame: stat decoder: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxDecoderBytes {
		return nil, fmt.Errorf("savegame: decoder must be a non-empty regular file no larger than 64 MiB")
	}
	if info.Mode().Perm()&0o111 == 0 {
		return nil, fmt.Errorf("savegame: decoder is not executable")
	}
	if timeout == 0 {
		timeout = defaultDecoderTimeout
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("savegame: decoder timeout must be positive")
	}
	return &commandDecoder{path: path, timeout: timeout}, nil
}

func (d *commandDecoder) Decompress(src []byte, rawLen int) ([]byte, error) {
	if d == nil || len(src) == 0 || rawLen <= 0 {
		return nil, fmt.Errorf("savegame: invalid decoder input/output size")
	}
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, d.path, "--raw-size", strconv.Itoa(rawLen))
	cmd.Dir = "/"
	cmd.Env = []string{}
	cmd.Stdin = bytes.NewReader(src)
	stdout := &limitedBuffer{remaining: int64(rawLen)}
	stderr := &truncatingBuffer{remaining: maxDecoderStderr}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if stdout.exceeded || errors.Is(err, errDecoderOutputLimit) {
		return nil, fmt.Errorf("savegame: open decoder output exceeds declared length %d", rawLen)
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("savegame: open decoder timed out after %s", d.timeout)
	}
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, fmt.Errorf("savegame: open decoder failed: %w: %s", err, detail)
		}
		return nil, fmt.Errorf("savegame: open decoder failed: %w", err)
	}
	if stdout.Len() != rawLen {
		return nil, fmt.Errorf("savegame: open decoder returned %d bytes, expected %d", stdout.Len(), rawLen)
	}
	return stdout.Bytes(), nil
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	remaining int64
	exceeded  bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if int64(len(p)) > b.remaining {
		allowed := max(int64(0), b.remaining)
		if allowed > 0 {
			_, _ = b.buffer.Write(p[:allowed])
		}
		b.remaining = 0
		b.exceeded = true
		return int(allowed), errDecoderOutputLimit
	}
	n, err := b.buffer.Write(p)
	b.remaining -= int64(n)
	return n, err
}

func (b *limitedBuffer) Len() int      { return b.buffer.Len() }
func (b *limitedBuffer) Bytes() []byte { return b.buffer.Bytes() }

type truncatingBuffer struct {
	buffer    bytes.Buffer
	remaining int64
}

func (b *truncatingBuffer) Write(p []byte) (int, error) {
	original := len(p)
	if int64(len(p)) > b.remaining {
		p = p[:max(int64(0), b.remaining)]
	}
	if len(p) > 0 {
		_, _ = b.buffer.Write(p)
		b.remaining -= int64(len(p))
	}
	return original, nil
}

func (b *truncatingBuffer) String() string { return b.buffer.String() }
