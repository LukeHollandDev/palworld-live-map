// Copyright (C) 2026 Luke Holland
// Portions copyright 2026 Palhelm contributors and licensed under Apache-2.0.
// Adapted and substantially modified on 2026-07-23. See NOTICE.
// SPDX-License-Identifier: GPL-3.0-or-later

package palsav

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"os"
)

// Decode decompresses and parses a complete Palworld .sav file.
func Decode(data []byte) (*Save, error) {
	return DecodeWithOptions(data, Options{})
}

// DecodeWithOptions is Decode with explicit resource limits and type hints.
func DecodeWithOptions(data []byte, options Options) (*Save, error) {
	options, cfg, err := options.normalized()
	if err != nil {
		return nil, err
	}
	raw, container, err := DecodeContainerWithLimits(data, options.Limits)
	if err != nil {
		return nil, err
	}
	save, err := parseGVAS(raw, cfg)
	if err != nil {
		return nil, err
	}
	save.Container = container
	return save, nil
}

// ParseGVAS parses an already decompressed Unreal SaveGame archive. It retains
// data without copying; callers must not mutate it while the Save is in use.
func ParseGVAS(data []byte) (*Save, error) {
	return ParseGVASWithOptions(data, Options{})
}

// ParseGVASWithOptions is ParseGVAS with explicit resource limits and type
// hints. MaxInputBytes and MaxOutputBytes both bound the supplied GVAS buffer.
func ParseGVASWithOptions(data []byte, options Options) (*Save, error) {
	options, cfg, err := options.normalized()
	if err != nil {
		return nil, err
	}
	limit := options.Limits.MaxOutputBytes
	if options.Limits.MaxInputBytes < limit {
		limit = options.Limits.MaxInputBytes
	}
	if int64(len(data)) > limit {
		return nil, &LimitError{
			Kind:  "GVAS input bytes",
			Value: uint64(len(data)),
			Limit: uint64(limit),
		}
	}
	return parseGVAS(data, cfg)
}

// Load reads, decompresses, and parses a .sav file.
func Load(path string) (*Save, error) {
	return LoadWithOptions(path, Options{})
}

// LoadWithOptions is Load with explicit resource limits and type hints.
func LoadWithOptions(path string, options Options) (*Save, error) {
	normalized, _, err := options.normalized()
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("palsav: open %q: %w", path, err)
	}
	defer file.Close()

	readLimit := normalized.Limits.MaxInputBytes
	if readLimit < math.MaxInt64 {
		readLimit++
	}
	data, err := io.ReadAll(io.LimitReader(file, readLimit))
	if err != nil {
		return nil, fmt.Errorf("palsav: read %q: %w", path, err)
	}
	if int64(len(data)) > normalized.Limits.MaxInputBytes {
		return nil, &LimitError{
			Kind:  "file input bytes",
			Value: uint64(len(data)),
			Limit: uint64(normalized.Limits.MaxInputBytes),
		}
	}
	return DecodeWithOptions(data, normalized)
}

func parseGVAS(data []byte, cfg *decodeConfig) (*Save, error) {
	reader := newArchiveReader(data, cfg)
	header, err := readGVASHeader(reader)
	if err != nil {
		return nil, err
	}
	properties, err := readPropertyList(reader, "")
	if err != nil {
		return nil, err
	}
	trailer, err := reader.take(reader.remaining())
	if err != nil {
		return nil, err
	}
	return &Save{
		Header:     header,
		Properties: properties,
		Trailer:    trailer,
		Raw:        data,
	}, nil
}

func readGVASHeader(reader *archiveReader) (GVASHeader, error) {
	var header GVASHeader
	magic, err := reader.take(4)
	if err != nil {
		return header, err
	}
	if !bytes.Equal(magic, []byte("GVAS")) {
		return header, fmt.Errorf("palsav: invalid GVAS magic %q", magic)
	}
	if header.SaveGameVersion, err = reader.i32(); err != nil {
		return header, err
	}
	if header.SaveGameVersion != 3 {
		return header, fmt.Errorf(
			"palsav: unsupported save-game version %d (want 3)",
			header.SaveGameVersion,
		)
	}
	if header.PackageUE4, err = reader.i32(); err != nil {
		return header, err
	}
	if header.PackageUE5, err = reader.i32(); err != nil {
		return header, err
	}
	if header.Engine.Major, err = reader.u16(); err != nil {
		return header, err
	}
	if header.Engine.Minor, err = reader.u16(); err != nil {
		return header, err
	}
	if header.Engine.Patch, err = reader.u16(); err != nil {
		return header, err
	}
	if header.Engine.ChangeList, err = reader.u32(); err != nil {
		return header, err
	}
	if header.Engine.Branch, err = reader.fstring(); err != nil {
		return header, err
	}
	if header.CustomFormat, err = reader.i32(); err != nil {
		return header, err
	}
	if header.CustomFormat != 3 {
		return header, fmt.Errorf(
			"palsav: unsupported custom-version format %d (want 3)",
			header.CustomFormat,
		)
	}
	count, err := reader.u32()
	if err != nil {
		return header, err
	}
	if err := validateCount(reader.state.cfg, "custom-version", count, reader.remaining(), 20); err != nil {
		return header, err
	}
	header.CustomVersions = make([]CustomVersion, count)
	for i := range header.CustomVersions {
		if header.CustomVersions[i].ID, err = reader.guid(); err != nil {
			return header, err
		}
		if header.CustomVersions[i].Version, err = reader.i32(); err != nil {
			return header, err
		}
	}
	if header.ClassName, err = reader.fstring(); err != nil {
		return header, err
	}
	return header, nil
}
