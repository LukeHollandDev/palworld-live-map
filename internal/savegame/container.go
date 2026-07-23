// Portions derived from Palhelm and modified for Palworld Live Map.
// Copyright 2026 Palhelm contributors. Licensed under Apache-2.0.
package savegame

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/LukeHollandDev/palworld-live-map/internal/palsav"
)

type containerHeader struct {
	RawLen        uint32
	CompressedLen uint32
	Magic         string
	SaveType      byte
	Offset        int
}

func readContainer(data []byte, maxBytes int64) ([]byte, containerHeader, error) {
	raw, decodedHeader, err := palsav.DecodeContainerWithLimits(data, palsav.Limits{
		MaxInputBytes:  maxBytes,
		MaxOutputBytes: maxBytes,
	})
	header := containerHeader{
		RawLen:        decodedHeader.RawSize,
		CompressedLen: decodedHeader.CompressedSize,
		Magic:         decodedHeader.Magic,
		SaveType:      decodedHeader.SaveType,
		Offset:        decodedHeader.Offset,
	}
	if err != nil {
		var limitErr *palsav.LimitError
		if errors.As(err, &limitErr) {
			return nil, header, &parseLimitError{
				Kind:  limitErr.Kind,
				Value: limitErr.Value,
				Limit: limitErr.Limit,
			}
		}
		return nil, header, fmt.Errorf("savegame: decode container: %w", err)
	}
	return raw, header, nil
}

// readSave performs only read operations and rejects symlinks/non-regular files.
// It checks size and modification time after reading to detect a torn snapshot.
func readSave(path string, maxBytes int64) ([]byte, fs.FileInfo, error) {
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
	raw, _, err := readContainer(b, maxBytes)
	return raw, before, err
}
