// Copyright (C) 2026 Luke Holland
// SPDX-License-Identifier: GPL-3.0-or-later

package palsav

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeSuppliedSaves(t *testing.T) {
	root := suppliedSaveRoot(t)
	paths, err := filepath.Glob(filepath.Join(root, "Players", "*.sav"))
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, filepath.Join(root, "Level.sav"), filepath.Join(root, "LevelMeta.sav"))
	golden := suppliedGoldenHashes(t, len(paths))
	for index, path := range paths {
		t.Run(fmt.Sprintf("save-%02d", index+1), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			raw, header, err := DecodeContainer(data)
			if err != nil {
				t.Fatal(err)
			}
			if header.Magic != "PlM" || header.SaveType != 0x31 {
				t.Fatalf("container = %q/%#x", header.Magic, header.SaveType)
			}
			if len(raw) != int(header.RawSize) || !bytes.HasPrefix(raw, []byte("GVAS")) {
				t.Fatalf("decoded size/prefix = %d/%q", len(raw), raw[:min(4, len(raw))])
			}
			if golden != nil {
				sum := sha256.Sum256(raw)
				if hex.EncodeToString(sum[:]) != golden[index] {
					t.Fatal("decoded payload does not match its private golden hash")
				}
			}
		})
	}
}

func suppliedGoldenHashes(t *testing.T, count int) []string {
	t.Helper()
	inline := os.Getenv("PALWORLD_SAVE_GOLDEN_SHA256")
	path := os.Getenv("PALWORLD_SAVE_GOLDEN_SHA256_FILE")
	if inline == "" && path == "" {
		return nil
	}
	var data []byte
	if inline != "" {
		data = []byte(inline)
	} else {
		var err error
		data, err = os.ReadFile(path)
		if err != nil {
			t.Fatalf("read private golden hash file: %v", err)
		}
	}
	hashes := strings.Fields(string(data))
	if len(hashes) != count {
		t.Fatalf("private golden hash file has %d entries, want %d", len(hashes), count)
	}
	for _, hash := range hashes {
		decoded, err := hex.DecodeString(hash)
		if err != nil || len(decoded) != sha256.Size {
			t.Fatal("private golden hash file contains an invalid SHA-256 value")
		}
	}
	return hashes
}

func suppliedSaveRoot(t *testing.T) string {
	t.Helper()
	root := os.Getenv("PALWORLD_SAVE_FIXTURES")
	configured := root != ""
	if !configured {
		root = "."
	}
	if _, err := os.Stat(filepath.Join(root, "Level.sav")); err != nil {
		if configured {
			t.Fatalf("PALWORLD_SAVE_FIXTURES does not contain Level.sav")
		}
		t.Skip("set PALWORLD_SAVE_FIXTURES to a directory containing external saves")
	}
	if _, err := os.Stat(filepath.Join(root, "LevelMeta.sav")); err != nil {
		t.Fatal("fixture directory does not contain LevelMeta.sav")
	}
	if info, err := os.Stat(filepath.Join(root, "Players")); err != nil || !info.IsDir() {
		t.Fatal("fixture directory does not contain Players/")
	}
	players, err := filepath.Glob(filepath.Join(root, "Players", "*.sav"))
	if err != nil {
		t.Fatal(err)
	}
	normal, dps := 0, 0
	for _, path := range players {
		if strings.HasSuffix(path, "_dps.sav") {
			dps++
		} else {
			normal++
		}
	}
	if normal == 0 || dps == 0 {
		t.Fatal("fixture directory must contain normal and DPS player saves")
	}
	return root
}
