// Copyright (C) 2026 Luke Holland
// Portions copyright 2026 Palhelm contributors and licensed under Apache-2.0.
// Adapted and substantially modified on 2026-07-23. See NOTICE.
// SPDX-License-Identifier: GPL-3.0-or-later
//

// Package palsav decodes Palworld .sav containers and their Unreal GVAS data.
//
// The package uses only the Go standard library. It is a read-only decoder:
// it never modifies a save and does not implement Oodle compression.
package palsav

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	defaultMaxInputBytes  = 1 << 30
	defaultMaxOutputBytes = 1 << 30
)

// Limits bounds allocations and work performed while decoding untrusted saves.
// A zero field selects the package default.
type Limits struct {
	MaxInputBytes  int64
	MaxOutputBytes int64
}

func (l Limits) normalized() (Limits, error) {
	if l.MaxInputBytes == 0 {
		l.MaxInputBytes = defaultMaxInputBytes
	}
	if l.MaxOutputBytes == 0 {
		l.MaxOutputBytes = defaultMaxOutputBytes
	}
	if l.MaxInputBytes < 12 {
		return Limits{}, fmt.Errorf("palsav: MaxInputBytes must be at least 12")
	}
	if l.MaxOutputBytes < 1 {
		return Limits{}, fmt.Errorf("palsav: MaxOutputBytes must be positive")
	}
	return l, nil
}

// ContainerHeader describes Palworld's wrapper around a GVAS payload.
type ContainerHeader struct {
	RawSize uint32
	// CompressedSize is the on-disk body size for single-layer containers.
	// For PlZ/0x32 it is the inner zlib stream size after the outer inflate.
	CompressedSize uint32
	Magic          string
	SaveType       byte
	Offset         int
}

// DecodeContainer unwraps and decompresses a complete Palworld .sav file.
// It supports PlM/0x31 (Oodle Mermaid) and PlZ/0x31 or 0x32 (zlib).
func DecodeContainer(data []byte) ([]byte, ContainerHeader, error) {
	return DecodeContainerWithLimits(data, Limits{})
}

// DecodeContainerWithLimits is DecodeContainer with explicit resource limits.
func DecodeContainerWithLimits(data []byte, limits Limits) ([]byte, ContainerHeader, error) {
	limits, err := limits.normalized()
	if err != nil {
		return nil, ContainerHeader{}, err
	}
	if int64(len(data)) > limits.MaxInputBytes {
		return nil, ContainerHeader{}, &LimitError{
			Kind:  "container input bytes",
			Value: uint64(len(data)),
			Limit: uint64(limits.MaxInputBytes),
		}
	}
	header, err := parseContainerHeader(data)
	if err != nil {
		return nil, header, err
	}
	if int64(header.RawSize) > limits.MaxOutputBytes {
		return nil, header, &LimitError{
			Kind:  "declared output bytes",
			Value: uint64(header.RawSize),
			Limit: uint64(limits.MaxOutputBytes),
		}
	}
	if uint64(header.RawSize) > uint64(maxInt()) {
		return nil, header, &LimitError{
			Kind:  "declared output bytes for this platform",
			Value: uint64(header.RawSize),
			Limit: uint64(maxInt()),
		}
	}
	bodyAt := header.Offset + 12
	if bodyAt < 0 || bodyAt > len(data) {
		return nil, header, fmt.Errorf("palsav: invalid container body offset %d", bodyAt)
	}

	var raw []byte
	switch header.Magic {
	case "PlM":
		if header.SaveType != 0x31 {
			return nil, header, fmt.Errorf("palsav: unsupported PlM save type %#x", header.SaveType)
		}
		body, sizeErr := singleLayerBody(data, bodyAt, header.CompressedSize)
		if sizeErr != nil {
			return nil, header, sizeErr
		}
		raw, err = decompressMermaid(body, int(header.RawSize))
	case "PlZ":
		switch header.SaveType {
		case 0x31:
			body, sizeErr := singleLayerBody(data, bodyAt, header.CompressedSize)
			if sizeErr != nil {
				return nil, header, sizeErr
			}
			raw, err = inflateExact(body, int64(header.RawSize))
		case 0x32:
			intermediateLimit := uint64(limits.MaxOutputBytes)
			if uint64(maxInt()) < intermediateLimit {
				intermediateLimit = uint64(maxInt())
			}
			if uint64(header.CompressedSize) > intermediateLimit {
				return nil, header, &LimitError{
					Kind:  "intermediate zlib bytes",
					Value: uint64(header.CompressedSize),
					Limit: intermediateLimit,
				}
			}
			var inner []byte
			// For double-zlib saves CompressedSize describes the inner stream
			// produced by inflating the complete on-disk body.
			inner, err = inflateExact(data[bodyAt:], int64(header.CompressedSize))
			if err == nil {
				raw, err = inflateExact(inner, int64(header.RawSize))
			}
		default:
			err = fmt.Errorf("palsav: unsupported PlZ save type %#x", header.SaveType)
		}
	default:
		err = fmt.Errorf("palsav: unsupported container magic %q", header.Magic)
	}
	if err != nil {
		return nil, header, err
	}
	if len(raw) != int(header.RawSize) {
		return nil, header, fmt.Errorf("palsav: decoded %d bytes, expected %d", len(raw), header.RawSize)
	}
	if !bytes.HasPrefix(raw, []byte("GVAS")) {
		return nil, header, errors.New("palsav: decoded payload does not begin with GVAS")
	}
	return raw, header, nil
}

func parseContainerHeader(data []byte) (ContainerHeader, error) {
	offsets := []int{0}
	if len(data) >= 12 && string(data[8:11]) == "CNK" {
		offsets = append(offsets, 12)
	}
	for _, offset := range offsets {
		if len(data) < offset+12 {
			continue
		}
		magic := string(data[offset+8 : offset+11])
		if magic != "PlM" && magic != "PlZ" {
			continue
		}
		return ContainerHeader{
			RawSize:        binary.LittleEndian.Uint32(data[offset:]),
			CompressedSize: binary.LittleEndian.Uint32(data[offset+4:]),
			Magic:          magic,
			SaveType:       data[offset+11],
			Offset:         offset,
		}, nil
	}
	return ContainerHeader{}, errors.New("palsav: PlM/PlZ header not found")
}

func singleLayerBody(data []byte, bodyAt int, compressedSize uint32) ([]byte, error) {
	if uint64(compressedSize) > uint64(len(data)-bodyAt) {
		return nil, fmt.Errorf("palsav: compressed size %d exceeds the container", compressedSize)
	}
	bodyEnd := bodyAt + int(compressedSize)
	if bodyEnd != len(data) {
		return nil, fmt.Errorf("palsav: %d trailing container bytes", len(data)-bodyEnd)
	}
	return data[bodyAt:bodyEnd], nil
}

func inflateExact(src []byte, expected int64) ([]byte, error) {
	source := bytes.NewReader(src)
	reader, err := zlib.NewReader(source)
	if err != nil {
		return nil, fmt.Errorf("palsav: zlib header: %w", err)
	}
	out, err := io.ReadAll(io.LimitReader(reader, expected+1))
	if err != nil {
		reader.Close()
		return nil, fmt.Errorf("palsav: zlib data: %w", err)
	}
	if err := reader.Close(); err != nil {
		return nil, fmt.Errorf("palsav: zlib close: %w", err)
	}
	if int64(len(out)) != expected {
		return nil, fmt.Errorf("palsav: zlib decoded %d bytes, expected %d", len(out), expected)
	}
	if source.Len() != 0 {
		return nil, fmt.Errorf("palsav: zlib stream has %d trailing bytes", source.Len())
	}
	return out, nil
}

func maxInt() int { return int(^uint(0) >> 1) }
