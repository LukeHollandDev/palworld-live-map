// Portions derived from Palhelm and modified for Palworld Live Map.
// Copyright 2026 Palhelm contributors. Licensed under Apache-2.0.
package savegame

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"os"
)

type saveDecompressor interface {
	Decompress(src []byte, rawLen int) ([]byte, error)
}

type containerHeader struct {
	RawLen        uint32
	CompressedLen uint32
	Magic         string
	SaveType      byte
	Offset        int
}

func readContainer(data []byte, maxBytes int64, decoder saveDecompressor) ([]byte, containerHeader, error) {
	h, err := parseContainerHeader(data)
	if err != nil {
		return nil, h, err
	}
	if int64(h.RawLen) > maxBytes {
		return nil, h, &parseLimitError{Kind: "decompressed save bytes", Value: uint64(h.RawLen), Limit: uint64(maxBytes)}
	}
	bodyStart := h.Offset + 12
	if int(h.CompressedLen) > len(data)-bodyStart {
		return nil, h, fmt.Errorf("savegame: compressed length %d exceeds %d-byte body", h.CompressedLen, len(data)-bodyStart)
	}
	src := data[bodyStart : bodyStart+int(h.CompressedLen)]
	var raw []byte
	switch h.Magic {
	case "PlZ":
		if h.SaveType != 0x31 && h.SaveType != 0x32 {
			return nil, h, fmt.Errorf("savegame: unsupported PlZ save type %#x", h.SaveType)
		}
		raw, err = zlibBytes(src, int64(h.RawLen))
		if err == nil && h.SaveType == 0x32 {
			raw, err = zlibBytes(raw, int64(h.RawLen))
		}
	case "PlM":
		if h.SaveType != 0x31 {
			return nil, h, fmt.Errorf("savegame: unsupported PlM save type %#x", h.SaveType)
		}
		if decoder == nil {
			return nil, h, fmt.Errorf("savegame: PlM requires a decoder")
		}
		raw, err = decoder.Decompress(src, int(h.RawLen))
	default:
		err = fmt.Errorf("savegame: unsupported container magic %q", h.Magic)
	}
	if err != nil {
		return nil, h, err
	}
	if len(raw) != int(h.RawLen) {
		return nil, h, fmt.Errorf("savegame: decompressed length %d, expected %d", len(raw), h.RawLen)
	}
	if !bytes.HasPrefix(raw, []byte("GVAS")) {
		return nil, h, fmt.Errorf("savegame: decompressed body does not begin with GVAS")
	}
	return raw, h, nil
}

func parseContainerHeader(data []byte) (containerHeader, error) {
	for _, base := range []int{0, 12} {
		if len(data) < base+12 {
			continue
		}
		magic := string(data[base+8 : base+11])
		if magic != "PlZ" && magic != "PlM" {
			continue
		}
		return containerHeader{
			RawLen:        binary.LittleEndian.Uint32(data[base:]),
			CompressedLen: binary.LittleEndian.Uint32(data[base+4:]),
			Magic:         magic,
			SaveType:      data[base+11],
			Offset:        base,
		}, nil
	}
	return containerHeader{}, fmt.Errorf("savegame: PlZ/PlM header not found at normal or CNK offset")
}

func zlibBytes(src []byte, rawLen int64) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("savegame: zlib: %w", err)
	}
	defer zr.Close()
	b, err := io.ReadAll(io.LimitReader(zr, rawLen+1))
	if err != nil {
		return nil, fmt.Errorf("savegame: zlib: %w", err)
	}
	if int64(len(b)) > rawLen {
		return nil, fmt.Errorf("savegame: zlib output exceeds declared raw length %d", rawLen)
	}
	return b, nil
}

// readSave performs only read operations and rejects symlinks/non-regular files.
// It checks size and modification time after reading to detect a torn snapshot.
func readSave(path string, maxBytes int64, decoder saveDecompressor) ([]byte, fs.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("savegame: stat save: %w", err)
	}
	if !before.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("savegame: save is not a regular file")
	}
	if before.Size() <= 0 || before.Size() > maxBytes {
		return nil, nil, &parseLimitError{Kind: "save file bytes", Value: uint64(max(0, before.Size())), Limit: uint64(maxBytes)}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("savegame: open save: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, nil, fmt.Errorf("savegame: read save: %w", err)
	}
	if int64(len(b)) > maxBytes {
		return nil, nil, &parseLimitError{Kind: "save file bytes", Value: uint64(len(b)), Limit: uint64(maxBytes)}
	}
	after, err := f.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("savegame: restat save: %w", err)
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, nil, fmt.Errorf("savegame: save changed while being read")
	}
	raw, _, err := readContainer(b, maxBytes, decoder)
	return raw, before, err
}
