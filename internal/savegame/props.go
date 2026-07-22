// Portions derived from Palhelm and modified for Palworld Live Map.
// Copyright 2026 Palhelm contributors. Licensed under Apache-2.0.
package savegame

import (
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	maxPropertyDepth       = 64
	maxPropertyCount       = 4 << 20
	maxCollectionElements  = 4 << 20
	maxByteCollectionBytes = 64 << 20
	maxDecodedNodes        = 4 << 20
	maxDecodedBytes        = 256 << 20
)

func consumeDecoded(stats *Stats, kind string, nodes, bytes uint64) error {
	if stats == nil {
		return nil
	}
	if nodes > maxDecodedNodes-stats.decodedNodes {
		return &parseLimitError{Kind: kind + " decoded nodes", Value: stats.decodedNodes + nodes, Limit: maxDecodedNodes}
	}
	if bytes > maxDecodedBytes-stats.decodedBytes {
		return &parseLimitError{Kind: kind + " decoded bytes", Value: stats.decodedBytes + bytes, Limit: maxDecodedBytes}
	}
	stats.decodedNodes += nodes
	stats.decodedBytes += bytes
	return nil
}

type propertyMap map[string]*property
type property struct {
	Type  string
	Value any
}
type structData struct {
	Type  string
	Value any
}
type enumData struct{ Type, Value string }
type mapEntry struct{ Key, Value any }

var typeHints = map[string]string{
	".worldSaveData.CharacterSaveParameterMap.Key":          "StructProperty",
	".worldSaveData.CharacterSaveParameterMap.Value":        "StructProperty",
	".worldSaveData.GroupSaveDataMap.Key":                   "Guid",
	".worldSaveData.GroupSaveDataMap.Value":                 "StructProperty",
	".worldSaveData.BaseCampSaveData.Key":                   "Guid",
	".worldSaveData.BaseCampSaveData.Value":                 "StructProperty",
	".worldSaveData.BaseCampSaveData.Value.ModuleMap.Value": "StructProperty",
	// GUID-keyed maps introduced/reachable in Palworld 1.0 worlds. Without these
	// hints readMap defaults a struct-typed key to "StructProperty" and tries to
	// read a property list out of a bare 16-byte GUID, which aborts the whole
	// worldSaveData decode. Mirrors palworld-save-tools PALWORLD_TYPE_HINTS.
	".worldSaveData.MapObjectSpawnerInStageSaveData.Value.SpawnerDataMapByLevelObjectInstanceId.Key": "Guid",
	".worldSaveData.GuildExtraSaveDataMap.Key":                                                       "Guid",
	".worldSaveData.InvaderSaveData.Key":                                                             "Guid",
	// Dungeon reward maps serialize GUID keys under a StructProperty map tag.
	// A populated Palworld 1.0 map therefore needs the concrete Guid hint;
	// otherwise the bare 16-byte key is mistaken for a nested property list.
	".worldSaveData.DungeonSaveData.DungeonSaveData.RewardSaveDataMap.Key": "Guid",
	// Transient maps that exist only while their world object does (fishing spot,
	// lock gimmick, active supply drop). Each was observed flapping the drift flag
	// on 2026-07-10/11 as a one-property skip; keys are bare GUIDs like the rest.
	".worldSaveData.FishingSpotSaveData.Key":        "Guid",
	".worldSaveData.LockGimmickSaveData.Key":        "Guid",
	".worldSaveData.SupplySaveData.SupplyInfos.Key": "Guid",
}

func readProperties(r *reader, path string, stats *Stats) (propertyMap, error) {
	r.propertyDepth++
	defer func() { r.propertyDepth-- }()
	if r.propertyDepth > maxPropertyDepth {
		return nil, &parseLimitError{Kind: "property depth", Value: uint64(r.propertyDepth), Limit: maxPropertyDepth}
	}
	out := make(propertyMap)
	previousName := "<start>"
	previousType := ""
	var previousSize uint64
	previousEnd := r.position()
	if err := consumeDecoded(stats, "property map", 1, 64); err != nil {
		return nil, err
	}
	for {
		name, err := r.fstring()
		if err != nil {
			return nil, fmt.Errorf("%s: property name after %s (%s size=%d end=%d): %w", path, previousName, previousType, previousSize, previousEnd, err)
		}
		if name == "None" {
			return out, nil
		}
		previousName = name
		stats.propertyCount++
		if stats.propertyCount > maxPropertyCount {
			return nil, &parseLimitError{Kind: "property count", Value: stats.propertyCount, Limit: maxPropertyCount}
		}
		if err := consumeDecoded(stats, "property", 1, 128); err != nil {
			return nil, err
		}
		typ, err := r.fstring()
		if err != nil {
			return nil, fmt.Errorf("%s.%s: property type: %w", path, name, err)
		}
		size, err := r.u64()
		if err != nil {
			return nil, fmt.Errorf("%s.%s: property size: %w", path, name, err)
		}
		previousType = typ
		previousSize = size
		payloadStart := r.position()
		retain := retainProperty(path, name)
		// This reader needs only two Level.sav maps and three per-player fields.
		// Skipping unrelated structured payloads by their declared size avoids
		// both large allocations and format drift in inventory/world-object data.
		if !retain {
			if bodyStart, ok := recoverBodyStart(r, payloadStart, typ); ok &&
				uint64(bodyStart)+size <= uint64(len(r.b)) {
				if seekErr := r.seek(bodyStart + int(size)); seekErr == nil {
					previousEnd = r.position()
					continue
				}
			}
			// Scalar tags are cheap and structurally stable. Rewind, decode them
			// for validation, then discard below instead of guessing their size.
			if err := r.seek(payloadStart); err != nil {
				return nil, err
			}
		}
		p, err := readProperty(r, typ, size, path+"."+name, stats)
		if err != nil {
			// Resilient per-property skip: an undecodable property (e.g. a
			// GUID-keyed map whose type hint is missing after a format change)
			// must not abort the whole property list. Skip exactly this property
			// using its declared size and continue, so unaffected siblings still
			// decode. Two guards keep the hardening invariants intact:
			//   * a *parseLimitError (hostile counts/depth/budget) must still fail
			//     fast — never swallow it.
			//   * only skip when the skip target is inside the buffer, so
			//     truncation still errors instead of silently succeeding.
			var limitErr *parseLimitError
			if errors.As(err, &limitErr) {
				return nil, fmt.Errorf("%s.%s (%s): %w", path, name, typ, err)
			}
			if path == "" && name == "worldSaveData" {
				return nil, fmt.Errorf(".worldSaveData (%s): %w", typ, err)
			}
			if bodyStart, ok := recoverBodyStart(r, payloadStart, typ); ok &&
				uint64(bodyStart)+size <= uint64(len(r.b)) {
				if seekErr := r.seek(bodyStart + int(size)); seekErr == nil {
					stats.recordSkip(path+"."+name, typ)
					previousEnd = r.position()
					continue
				}
			}
			return nil, fmt.Errorf("%s.%s (%s): %w", path, name, typ, err)
		}
		if retain {
			out[name] = p
		}
		previousEnd = r.position()
	}
}

// retainProperty is deliberately narrow only at paths whose shape is part of
// the roster contract. Every other property list remains generically decoded,
// preserving the parser's normal handling of required nested structures.
func retainProperty(path, name string) bool {
	switch path {
	case ".worldSaveData":
		return name == "CharacterSaveParameterMap" || name == "GroupSaveDataMap"
	case ".worldSaveData.CharacterSaveParameterMap.Key":
		return name == "PlayerUId" || name == "PlayerUID" || name == "PlayerUid"
	case ".worldSaveData.CharacterSaveParameterMap.Value",
		".worldSaveData.GroupSaveDataMap.Value":
		return name == "RawData" || name == "GroupType"
	case ".worldSaveData.CharacterSaveParameterMap.Value.RawData":
		return name == "SaveParameter"
	case ".worldSaveData.CharacterSaveParameterMap.Value.RawData.SaveParameter":
		return name == "IsPlayer" || name == "NickName" || name == "Nickname" || name == "Level"
	case ".SaveData":
		return name == "PlayerUId" || name == "PlayerUID" || name == "PlayerUid" ||
			name == "LastTransform" || name == "LastOnlineDateTime" || name == "RecordData"
	case ".SaveData.LastTransform":
		return name == "Translation"
	case ".SaveData.RecordData":
		return name == "TribeCaptureCount" || name == "PalCaptureCount" || name == "PaldeckUnlockFlag"
	default:
		return true
	}
}

// recoverBodyStart re-reads the type-specific FPropertyTag preamble starting at
// payloadStart and returns the offset where the size-counted value body begins.
// A property's declared size counts only that value body (it excludes the tag
// preamble), so skipping requires locating the body start first. Only the
// structured, drift-prone container types are recoverable; anything else returns
// false so the caller propagates the original error rather than guessing.
func recoverBodyStart(r *reader, payloadStart int, typ string) (int, bool) {
	if err := r.seek(payloadStart); err != nil {
		return 0, false
	}
	switch typ {
	case "MapProperty":
		if _, err := r.fstring(); err != nil { // key type
			return 0, false
		}
		if _, err := r.fstring(); err != nil { // value type
			return 0, false
		}
		if _, err := readOptionalGUID(r); err != nil {
			return 0, false
		}
		return r.position(), true
	case "ArrayProperty", "SetProperty":
		if _, err := r.fstring(); err != nil { // element type
			return 0, false
		}
		if _, err := readOptionalGUID(r); err != nil {
			return 0, false
		}
		return r.position(), true
	case "StructProperty":
		if _, err := r.fstring(); err != nil { // struct type
			return 0, false
		}
		if _, err := readGUID(r); err != nil {
			return 0, false
		}
		if _, err := readOptionalGUID(r); err != nil {
			return 0, false
		}
		return r.position(), true
	default:
		return 0, false
	}
}

func readProperty(r *reader, typ string, size uint64, path string, stats *Stats) (*property, error) {
	p := &property{Type: typ}
	var err error
	switch typ {
	case "IntProperty":
		_, err = readOptionalGUID(r)
		if err == nil {
			var v int32
			v, err = r.i32()
			p.Value = v
		}
	case "Int64Property":
		_, err = readOptionalGUID(r)
		if err == nil {
			var v int64
			v, err = r.i64()
			p.Value = v
		}
	case "UInt16Property":
		_, err = readOptionalGUID(r)
		if err == nil {
			var v uint16
			v, err = r.u16()
			p.Value = v
		}
	case "UInt32Property":
		_, err = readOptionalGUID(r)
		if err == nil {
			var v uint32
			v, err = r.u32()
			p.Value = v
		}
	case "UInt64Property":
		_, err = readOptionalGUID(r)
		if err == nil {
			var v uint64
			v, err = r.u64()
			p.Value = v
		}
	case "FixedPoint64Property":
		_, err = readOptionalGUID(r)
		if err == nil {
			var v int32
			v, err = r.i32()
			p.Value = v
		}
	case "FloatProperty":
		_, err = readOptionalGUID(r)
		if err == nil {
			p.Value, err = r.f32()
		}
	case "DoubleProperty":
		_, err = readOptionalGUID(r)
		if err == nil {
			p.Value, err = r.f64()
		}
	case "StrProperty", "NameProperty":
		_, err = readOptionalGUID(r)
		if err == nil {
			p.Value, err = r.fstring()
		}
	case "BoolProperty":
		var v uint8
		v, err = r.u8()
		p.Value = v != 0
		if err == nil {
			_, err = readOptionalGUID(r)
		}
	case "EnumProperty":
		p.Value, err = readEnum(r)
	case "ByteProperty":
		p.Value, err = readByteProperty(r)
	case "StructProperty":
		p.Value, err = readStruct(r, size, path, stats)
	case "ArrayProperty":
		p.Value, err = readArray(r, size, path, stats)
	case "MapProperty":
		p.Value, err = readMap(r, path, stats)
	default:
		// Unreal property types may add type-specific tag metadata. For an
		// unknown tag the only safe recovery available is its declared payload.
		stats.SkippedProperties++
		if _, err = readOptionalGUID(r); err != nil {
			return nil, err
		}
		if size > uint64(r.remaining()) {
			return nil, fmt.Errorf("unknown type %s declares %d bytes with %d remaining", typ, size, r.remaining())
		}
		p.Value, err = r.read(int(size))
	}
	return p, err
}

func readEnum(r *reader) (enumData, error) {
	t, err := r.fstring()
	if err != nil {
		return enumData{}, err
	}
	if _, err = readOptionalGUID(r); err != nil {
		return enumData{}, err
	}
	v, err := r.fstring()
	return enumData{Type: t, Value: v}, err
}

func readByteProperty(r *reader) (enumData, error) {
	t, err := r.fstring()
	if err != nil {
		return enumData{}, err
	}
	if _, err = readOptionalGUID(r); err != nil {
		return enumData{}, err
	}
	if t == "None" {
		v, e := r.u8()
		return enumData{Type: t, Value: fmt.Sprint(v)}, e
	}
	v, err := r.fstring()
	return enumData{Type: t, Value: v}, err
}

func readStruct(r *reader, size uint64, path string, stats *Stats) (structData, error) {
	t, err := r.fstring()
	if err != nil {
		return structData{}, err
	}
	if _, err = readGUID(r); err != nil {
		return structData{}, err
	}
	if _, err = readOptionalGUID(r); err != nil {
		return structData{}, err
	}
	// worldSaveData is the decoder's required root contract. Falling it back to
	// opaque bytes hides the exact nested schema error and can only lead to a
	// generic "missing worldSaveData" result later.
	allowFallback := path != ".worldSaveData"
	v, err := readStructValue(r, t, size, path, stats, allowFallback)
	return structData{Type: t, Value: v}, err
}

func readStructValue(r *reader, typ string, size uint64, path string, stats *Stats, allowFallback bool) (any, error) {
	switch typ {
	case "Vector":
		x, e := r.f64()
		if e != nil {
			return nil, e
		}
		y, e := r.f64()
		if e != nil {
			return nil, e
		}
		z, e := r.f64()
		return Vector{X: x, Y: y, Z: z}, e
	case "Quat":
		vals := make([]float64, 4)
		for i := range vals {
			v, e := r.f64()
			if e != nil {
				return nil, e
			}
			vals[i] = v
		}
		return vals, nil
	case "LinearColor":
		vals := make([]float32, 4)
		for i := range vals {
			v, e := r.f32()
			if e != nil {
				return nil, e
			}
			vals[i] = v
		}
		return vals, nil
	case "Color":
		return r.read(4)
	case "DateTime":
		return r.u64()
	case "Guid":
		return readGUID(r)
	default:
		start := r.position()
		startPropertyCount := stats.propertyCount
		v, err := readProperties(r, path, stats)
		if err == nil {
			return v, nil
		}
		if !allowFallback {
			return nil, err
		}
		stats.propertyCount = startPropertyCount
		_ = r.seek(start)
		if size > uint64(r.remaining()) {
			return nil, err
		}
		stats.SkippedStructs++
		return r.read(int(size))
	}
}

func readArray(r *reader, size uint64, path string, stats *Stats) (any, error) {
	t, err := r.fstring()
	if err != nil {
		return nil, err
	}
	if _, err = readOptionalGUID(r); err != nil {
		return nil, err
	}
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if t == "ByteProperty" {
		if err := validateCountLimit("byte array", n, r.remaining(), 1, maxByteCollectionBytes); err != nil {
			return nil, err
		}
		if err := consumeDecoded(stats, "byte array", 1, uint64(n)); err != nil {
			return nil, err
		}
		b, e := r.read(int(n))
		return b, e
	}
	if t == "StructProperty" {
		name, e := r.fstring()
		if e != nil {
			return nil, e
		}
		propType, e := r.fstring()
		if e != nil {
			return nil, e
		}
		_ = name
		_ = propType
		innerSize, e := r.u64()
		if e != nil {
			return nil, e
		}
		structType, e := r.fstring()
		if e != nil {
			return nil, e
		}
		if _, e = readGUID(r); e != nil {
			return nil, e
		}
		if e = r.skip(1); e != nil {
			return nil, e
		}
		if err := validateCount("struct array", n, r.remaining(), minStructValueSize(structType)); err != nil {
			return nil, err
		}
		if err := consumeDecoded(stats, "struct array", uint64(n), uint64(n)*16); err != nil {
			return nil, err
		}
		vals := make([]any, 0)
		for range n {
			v, e := readStructValue(r, structType, innerSize, path+"."+name, stats, false)
			if e != nil {
				return nil, e
			}
			vals = append(vals, v)
		}
		return vals, nil
	}
	if err := validateCount("array", n, r.remaining(), minValueSize(t)); err != nil {
		return nil, err
	}
	if err := consumeDecoded(stats, "array", uint64(n), uint64(n)*decodedValueSize(t)); err != nil {
		return nil, err
	}
	if vals, ok, err := readTypedArray(r, t, n, path, stats); ok {
		return vals, err
	}
	vals := make([]any, 0)
	for range n {
		v, e := readValue(r, t, "", path, stats)
		if e != nil {
			return nil, e
		}
		vals = append(vals, v)
	}
	_ = size
	return vals, nil
}

func readMap(r *reader, path string, stats *Stats) ([]mapEntry, error) {
	kt, err := r.fstring()
	if err != nil {
		return nil, err
	}
	vt, err := r.fstring()
	if err != nil {
		return nil, err
	}
	if _, err = readOptionalGUID(r); err != nil {
		return nil, err
	}
	if _, err = r.u32(); err != nil {
		return nil, err
	}
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	khint, vhint := typeHints[path+".Key"], typeHints[path+".Value"]
	minEntry := minValueSize(kt) + minValueSize(vt)
	if err := validateCount("map", n, r.remaining(), minEntry); err != nil {
		return nil, err
	}
	if err := consumeDecoded(stats, "map", uint64(n)*3, uint64(n)*32); err != nil {
		return nil, err
	}
	entries := make([]mapEntry, 0)
	for range n {
		k, e := readValue(r, kt, khint, path+".Key", stats)
		if e != nil {
			return nil, e
		}
		v, e := readValue(r, vt, vhint, path+".Value", stats)
		if e != nil {
			return nil, e
		}
		entries = append(entries, mapEntry{Key: k, Value: v})
	}
	return entries, nil
}

func readValue(r *reader, typ, structType, path string, stats *Stats) (any, error) {
	switch typ {
	case "StructProperty":
		if structType == "" {
			structType = "StructProperty"
		}
		return readStructValue(r, structType, 0, path, stats, false)
	case "EnumProperty", "NameProperty", "StrProperty":
		return r.fstring()
	case "IntProperty":
		return r.i32()
	case "Int64Property":
		return r.i64()
	case "UInt32Property":
		return r.u32()
	case "UInt64Property":
		return r.u64()
	case "FloatProperty":
		return r.f32()
	case "DoubleProperty":
		return r.f64()
	case "BoolProperty":
		v, e := r.u8()
		return v != 0, e
	case "Guid":
		return readGUID(r)
	default:
		return nil, fmt.Errorf("unsupported nested value type %s at %s", typ, path)
	}
}

func validateCount(kind string, n uint32, remaining, minElementSize int) error {
	return validateCountLimit(kind, n, remaining, minElementSize, maxCollectionElements)
}

func validateCountLimit(kind string, n uint32, remaining, minElementSize int, absolute uint64) error {
	if minElementSize < 1 {
		minElementSize = 1
	}
	limit := uint64(remaining / minElementSize)
	if limit > absolute {
		limit = absolute
	}
	if uint64(n) > limit {
		return &parseLimitError{Kind: kind + " count", Value: uint64(n), Limit: limit}
	}
	return nil
}

func decodedValueSize(typ string) uint64 {
	switch typ {
	case "BoolProperty":
		return 1
	case "IntProperty", "UInt32Property", "FloatProperty":
		return 4
	case "Int64Property", "UInt64Property", "DoubleProperty":
		return 8
	default:
		return 16
	}
}

func readTypedArray(r *reader, typ string, n uint32, path string, stats *Stats) (any, bool, error) {
	switch typ {
	case "BoolProperty":
		v := make([]bool, n)
		for i := range v {
			x, err := r.u8()
			if err != nil {
				return nil, true, err
			}
			v[i] = x != 0
		}
		return v, true, nil
	case "IntProperty":
		v := make([]int32, n)
		for i := range v {
			var err error
			v[i], err = r.i32()
			if err != nil {
				return nil, true, err
			}
		}
		return v, true, nil
	case "UInt32Property":
		v := make([]uint32, n)
		for i := range v {
			var err error
			v[i], err = r.u32()
			if err != nil {
				return nil, true, err
			}
		}
		return v, true, nil
	case "Int64Property":
		v := make([]int64, n)
		for i := range v {
			var err error
			v[i], err = r.i64()
			if err != nil {
				return nil, true, err
			}
		}
		return v, true, nil
	case "UInt64Property":
		v := make([]uint64, n)
		for i := range v {
			var err error
			v[i], err = r.u64()
			if err != nil {
				return nil, true, err
			}
		}
		return v, true, nil
	case "FloatProperty":
		v := make([]float32, n)
		for i := range v {
			var err error
			v[i], err = r.f32()
			if err != nil {
				return nil, true, err
			}
		}
		return v, true, nil
	case "DoubleProperty":
		v := make([]float64, n)
		for i := range v {
			var err error
			v[i], err = r.f64()
			if err != nil {
				return nil, true, err
			}
		}
		return v, true, nil
	default:
		_ = path
		_ = stats
		return nil, false, nil
	}
}

func minValueSize(typ string) int {
	switch typ {
	case "BoolProperty":
		return 1
	case "IntProperty", "UInt32Property", "FloatProperty", "EnumProperty", "NameProperty", "StrProperty":
		return 4
	case "Int64Property", "UInt64Property", "DoubleProperty":
		return 8
	case "Guid":
		return 16
	case "StructProperty":
		return 9 // A nested property list needs the length and bytes of its "None" terminator.
	default:
		return 1
	}
}

func minStructValueSize(typ string) int {
	switch typ {
	case "Vector":
		return 24
	case "Quat":
		return 32
	case "LinearColor", "Guid":
		return 16
	case "Color":
		return 4
	case "DateTime":
		return 8
	default:
		return 9
	}
}

func readOptionalGUID(r *reader) (string, error) {
	yes, err := r.u8()
	if err != nil {
		return "", err
	}
	if yes == 0 {
		return "", nil
	}
	return readGUID(r)
}

func readGUID(r *reader) (string, error) {
	b, err := r.read(16)
	if err != nil {
		return "", err
	}
	// Unreal serializes each GUID uint32 little-endian. The Python reference's
	// display order is 4-2-2-2-2+4 with byte reversal inside those groups.
	return fmt.Sprintf("%s-%s-%s-%s-%s%s", hex.EncodeToString([]byte{b[3], b[2], b[1], b[0]}), hex.EncodeToString([]byte{b[7], b[6]}), hex.EncodeToString([]byte{b[5], b[4]}), hex.EncodeToString([]byte{b[11], b[10]}), hex.EncodeToString([]byte{b[9], b[8]}), hex.EncodeToString([]byte{b[15], b[14], b[13], b[12]})), nil
}
