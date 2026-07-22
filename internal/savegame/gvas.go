// Portions derived from Palhelm and modified for Palworld Live Map.
// Copyright 2026 Palhelm contributors. Licensed under Apache-2.0.
package savegame

import "fmt"

type gvasHeader struct {
	SaveGameVersion int32
	PackageUE4      int32
	PackageUE5      int32
	EngineMajor     uint16
	EngineMinor     uint16
	EnginePatch     uint16
	EngineChange    uint32
	EngineBranch    string
	CustomFormat    int32
	CustomVersions  []customVersion
	ClassName       string
}

type customVersion struct {
	ID      string
	Version int32
}

type gvasFile struct {
	Header     gvasHeader
	Properties propertyMap
	Trailer    []byte
}

func parseGVAS(data []byte, stats *Stats) (*gvasFile, error) {
	r := newReaderWithStats(data, stats)
	h, err := readGVASHeader(r)
	if err != nil {
		return nil, err
	}
	props, err := readProperties(r, "", stats)
	if err != nil {
		return nil, err
	}
	trailer, err := r.read(r.remaining())
	if err != nil {
		return nil, err
	}
	// Trailer aliases the already-owned decompressed save buffer. Copying it can
	// double memory for a valid early-None file without adding parser value.
	return &gvasFile{Header: h, Properties: props, Trailer: trailer}, nil
}

func readGVASHeader(r *reader) (gvasHeader, error) {
	var h gvasHeader
	magic, err := r.u32()
	if err != nil {
		return h, err
	}
	if magic != 0x53415647 {
		return h, fmt.Errorf("sav: invalid GVAS magic %#x", magic)
	}
	if h.SaveGameVersion, err = r.i32(); err != nil {
		return h, err
	}
	if h.SaveGameVersion != 3 {
		return h, fmt.Errorf("sav: unsupported save-game version %d", h.SaveGameVersion)
	}
	if h.PackageUE4, err = r.i32(); err != nil {
		return h, err
	}
	if h.PackageUE5, err = r.i32(); err != nil {
		return h, err
	}
	if h.EngineMajor, err = r.u16(); err != nil {
		return h, err
	}
	if h.EngineMinor, err = r.u16(); err != nil {
		return h, err
	}
	if h.EnginePatch, err = r.u16(); err != nil {
		return h, err
	}
	if h.EngineChange, err = r.u32(); err != nil {
		return h, err
	}
	if h.EngineBranch, err = r.fstring(); err != nil {
		return h, err
	}
	if h.CustomFormat, err = r.i32(); err != nil {
		return h, err
	}
	if h.CustomFormat != 3 {
		return h, fmt.Errorf("sav: unsupported custom-version format %d", h.CustomFormat)
	}
	n, err := r.u32()
	if err != nil {
		return h, err
	}
	if err := validateCount("custom-version", n, r.remaining(), 20); err != nil {
		return h, err
	}
	if err := consumeDecoded(r.stats, "custom-version array", uint64(n), uint64(n)*32); err != nil {
		return h, err
	}
	h.CustomVersions = make([]customVersion, 0)
	for range n {
		id, e := readGUID(r)
		if e != nil {
			return h, e
		}
		v, e := r.i32()
		if e != nil {
			return h, e
		}
		h.CustomVersions = append(h.CustomVersions, customVersion{ID: id, Version: v})
	}
	if h.ClassName, err = r.fstring(); err != nil {
		return h, err
	}
	return h, nil
}
