// Copyright (C) 2026 Luke Holland
// Portions copyright 2026 Palhelm contributors and licensed under Apache-2.0.
// Adapted and substantially modified on 2026-07-23. See NOTICE.
// SPDX-License-Identifier: GPL-3.0-or-later

package palsav

import (
	"encoding/binary"
	"fmt"
	"math"
	"unicode/utf16"
)

type parseState struct {
	cfg        *decodeConfig
	depth      int
	properties uint64
}

// LimitError reports that a caller-configurable decoding limit was exceeded.
type LimitError struct {
	Kind  string
	Value uint64
	Limit uint64
}

func (err *LimitError) Error() string {
	return fmt.Sprintf("palsav: %s %d exceeds limit %d", err.Kind, err.Value, err.Limit)
}

func (s *parseState) enter(kind string) error {
	s.depth++
	if s.depth > s.cfg.maxDepth {
		s.depth--
		return &LimitError{
			Kind:  kind + " nesting depth",
			Value: uint64(s.depth + 1),
			Limit: uint64(s.cfg.maxDepth),
		}
	}
	return nil
}

func (s *parseState) leave() { s.depth-- }

type archiveReader struct {
	data  []byte
	at    int
	base  int
	state *parseState
}

func newArchiveReader(data []byte, cfg *decodeConfig) *archiveReader {
	return &archiveReader{data: data, state: &parseState{cfg: cfg}}
}

func newArchiveReaderAt(data []byte, base int, cfg *decodeConfig) *archiveReader {
	return &archiveReader{data: data, base: base, state: &parseState{cfg: cfg}}
}

func (r *archiveReader) sub(data []byte, base int) *archiveReader {
	return &archiveReader{data: data, base: base, state: r.state}
}

func (r *archiveReader) offset() int    { return r.base + r.at }
func (r *archiveReader) remaining() int { return len(r.data) - r.at }

func (r *archiveReader) take(n int) ([]byte, error) {
	if n < 0 || n > r.remaining() {
		return nil, fmt.Errorf("palsav: unexpected EOF at offset %d reading %d bytes (%d remain)", r.offset(), n, r.remaining())
	}
	value := r.data[r.at : r.at+n]
	r.at += n
	return value, nil
}

func (r *archiveReader) skip(n int) error {
	_, err := r.take(n)
	return err
}

func (r *archiveReader) u8() (uint8, error) {
	value, err := r.take(1)
	if err != nil {
		return 0, err
	}
	return value[0], nil
}

func (r *archiveReader) i8() (int8, error) {
	value, err := r.u8()
	return int8(value), err
}

func (r *archiveReader) u16() (uint16, error) {
	value, err := r.take(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(value), nil
}

func (r *archiveReader) i16() (int16, error) {
	value, err := r.u16()
	return int16(value), err
}

func (r *archiveReader) u32() (uint32, error) {
	value, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(value), nil
}

func (r *archiveReader) i32() (int32, error) {
	value, err := r.u32()
	return int32(value), err
}

func (r *archiveReader) u64() (uint64, error) {
	value, err := r.take(8)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint64(value), nil
}

func (r *archiveReader) i64() (int64, error) {
	value, err := r.u64()
	return int64(value), err
}

func (r *archiveReader) f32() (float32, error) {
	value, err := r.u32()
	return math.Float32frombits(value), err
}

func (r *archiveReader) f64() (float64, error) {
	value, err := r.u64()
	return math.Float64frombits(value), err
}

func (r *archiveReader) guid() (GUID, error) {
	var value GUID
	var err error
	if value.A, err = r.u32(); err != nil {
		return GUID{}, err
	}
	if value.B, err = r.u32(); err != nil {
		return GUID{}, err
	}
	if value.C, err = r.u32(); err != nil {
		return GUID{}, err
	}
	if value.D, err = r.u32(); err != nil {
		return GUID{}, err
	}
	return value, nil
}

func (r *archiveReader) optionalGUID() (*GUID, error) {
	present, err := r.u8()
	if err != nil {
		return nil, err
	}
	if present == 0 {
		return nil, nil
	}
	if present != 1 {
		return nil, fmt.Errorf("palsav: invalid optional GUID marker %d at offset %d", present, r.offset()-1)
	}
	value, err := r.guid()
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (r *archiveReader) fstring() (string, error) {
	start := r.offset()
	count, err := r.i32()
	if err != nil {
		return "", err
	}
	if count == 0 {
		return "", nil
	}
	if count > 0 {
		size := int64(count)
		if size > int64(r.remaining()) {
			return "", fmt.Errorf("palsav: invalid FString length %d at offset %d", count, start)
		}
		if size > int64(r.state.cfg.maxStringBytes) {
			return "", &LimitError{
				Kind:  "FString bytes",
				Value: uint64(size),
				Limit: uint64(r.state.cfg.maxStringBytes),
			}
		}
		value, err := r.take(int(size))
		if err != nil {
			return "", err
		}
		if len(value) == 0 || value[len(value)-1] != 0 {
			return "", fmt.Errorf("palsav: unterminated FString at offset %d", start)
		}
		return string(value[:len(value)-1]), nil
	}
	if count == math.MinInt32 {
		return "", fmt.Errorf("palsav: invalid UTF-16 FString length at offset %d", start)
	}
	units := int64(-count)
	if units*2 > int64(r.remaining()) {
		return "", fmt.Errorf("palsav: invalid UTF-16 FString length %d at offset %d", count, start)
	}
	if units > int64(r.state.cfg.maxStringBytes/2) {
		return "", &LimitError{
			Kind:  "UTF-16 FString bytes",
			Value: uint64(units * 2),
			Limit: uint64(r.state.cfg.maxStringBytes),
		}
	}
	value, err := r.take(int(units * 2))
	if err != nil {
		return "", err
	}
	if len(value) < 2 || value[len(value)-2] != 0 || value[len(value)-1] != 0 {
		return "", fmt.Errorf("palsav: unterminated UTF-16 FString at offset %d", start)
	}
	decoded := make([]uint16, units-1)
	for i := range decoded {
		decoded[i] = binary.LittleEndian.Uint16(value[i*2:])
	}
	return string(utf16.Decode(decoded)), nil
}

func validateCount(cfg *decodeConfig, kind string, count uint32, remaining, minimum int) error {
	if count > cfg.maxCollectionElements {
		return &LimitError{
			Kind:  kind + " count",
			Value: uint64(count),
			Limit: uint64(cfg.maxCollectionElements),
		}
	}
	if uint64(count) > uint64(maxInt()) {
		return &LimitError{
			Kind:  kind + " count for this platform",
			Value: uint64(count),
			Limit: uint64(maxInt()),
		}
	}
	if minimum < 1 {
		minimum = 1
	}
	if uint64(count)*uint64(minimum) > uint64(remaining) {
		return fmt.Errorf("palsav: %s count %d cannot fit in %d bytes", kind, count, remaining)
	}
	return nil
}
