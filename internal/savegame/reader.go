// Portions derived from Palhelm and modified for Palworld Live Map.
// Copyright 2026 Palhelm contributors. Licensed under Apache-2.0.
package savegame

import (
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf16"
)

const maxFStringBytes int64 = 4 << 20

// parseLimitError reports a structural limit violation in hostile input.
// Keeping this distinct from ordinary truncation errors lets callers and tests
// recognize deliberate parser limits with errors.As.
type parseLimitError struct {
	Kind  string
	Value uint64
	Limit uint64
}

func (e *parseLimitError) Error() string {
	return fmt.Sprintf("sav: %s %d exceeds limit %d", e.Kind, e.Value, e.Limit)
}

type reader struct {
	b             []byte
	off           int
	propertyDepth uint32
	stats         *Stats
}

func newReader(b []byte) *reader {
	stats := newStats()
	return newReaderWithStats(b, &stats)
}
func newReaderWithStats(b []byte, stats *Stats) *reader { return &reader{b: b, stats: stats} }
func (r *reader) remaining() int                        { return len(r.b) - r.off }
func (r *reader) position() int                         { return r.off }

func (r *reader) seek(pos int) error {
	if pos < 0 || pos > len(r.b) {
		return fmt.Errorf("sav: seek to %d outside 0..%d", pos, len(r.b))
	}
	r.off = pos
	return nil
}

func (r *reader) read(n int) ([]byte, error) {
	if n < 0 || n > r.remaining() {
		return nil, fmt.Errorf("sav: unexpected EOF at offset %d reading %d bytes (%d remain)", r.off, n, r.remaining())
	}
	b := r.b[r.off : r.off+n]
	r.off += n
	return b, nil
}

func (r *reader) skip(n int) error { _, err := r.read(n); return err }

func (r *reader) u8() (uint8, error) {
	b, err := r.read(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func (r *reader) u16() (uint16, error) {
	b, err := r.read(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(b), nil
}

func (r *reader) i32() (int32, error) {
	v, err := r.u32()
	return int32(v), err
}

func (r *reader) u32() (uint32, error) {
	b, err := r.read(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

func (r *reader) i64() (int64, error) {
	v, err := r.u64()
	return int64(v), err
}

func (r *reader) u64() (uint64, error) {
	b, err := r.read(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(b), nil
}

func (r *reader) f32() (float32, error) {
	v, err := r.u32()
	return math.Float32frombits(v), err
}

func (r *reader) f64() (float64, error) {
	v, err := r.u64()
	return math.Float64frombits(v), err
}

func (r *reader) fstring() (string, error) {
	n, err := r.i32()
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	if n > 0 {
		length := int64(n)
		if length > int64(r.remaining()) || length > maxFStringBytes {
			return "", fmt.Errorf("sav: invalid FString length %d at offset %d", n, r.off-4)
		}
		b, err := r.read(int(length))
		if err != nil {
			return "", err
		}
		if len(b) == 0 || b[len(b)-1] != 0 {
			return "", fmt.Errorf("sav: unterminated ANSI FString at offset %d", r.off-len(b))
		}
		if err := consumeDecoded(r.stats, "FString", 1, uint64(len(b)-1)); err != nil {
			return "", err
		}
		return string(b[:len(b)-1]), nil
	}
	units := -int64(n)
	byteLen := units * 2
	if byteLen > int64(r.remaining()) || byteLen > maxFStringBytes {
		return "", fmt.Errorf("sav: invalid UTF-16 FString length %d at offset %d", n, r.off-4)
	}
	b, err := r.read(int(byteLen))
	if err != nil {
		return "", err
	}
	if len(b) < 2 || b[len(b)-2] != 0 || b[len(b)-1] != 0 {
		return "", fmt.Errorf("sav: unterminated UTF-16 FString at offset %d", r.off-len(b))
	}
	if err := consumeDecoded(r.stats, "FString", 1, uint64(byteLen)); err != nil {
		return "", err
	}
	u := make([]uint16, units-1)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(b[i*2:])
	}
	return string(utf16.Decode(u)), nil
}
