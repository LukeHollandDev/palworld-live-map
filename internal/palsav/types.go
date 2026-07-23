// Copyright (C) 2026 Luke Holland
// Portions copyright 2026 Palhelm contributors and licensed under Apache-2.0.
// Adapted and substantially modified on 2026-07-23. See NOTICE.
// SPDX-License-Identifier: GPL-3.0-or-later

package palsav

import (
	"fmt"
	"strings"
)

const (
	defaultMaxStringBytes        = 16 << 20
	defaultMaxPathBytes          = 64 << 10
	defaultMaxCollectionElements = 10_000_000
	defaultMaxDepth              = 128
	defaultMaxProperties         = 10_000_000
)

// Options controls container and GVAS decoding.
type Options struct {
	Limits Limits

	// MaxStringBytes bounds one serialized FString, including its terminator.
	MaxStringBytes int
	// MaxPathBytes bounds one constructed nested property path.
	MaxPathBytes int
	// MaxCollectionElements bounds one array, map, or set.
	MaxCollectionElements uint32
	// MaxDepth bounds nested property lists and collection values.
	MaxDepth int
	// MaxProperties bounds the number of eagerly decoded tagged properties.
	MaxProperties uint64

	// TypeHints overrides or extends Palworld's built-in path-to-struct hints.
	// Keys use paths such as ".worldSaveData.BaseCampSaveData.Key".
	TypeHints map[string]string
}

type decodeConfig struct {
	maxStringBytes        int
	maxPathBytes          int
	maxCollectionElements uint32
	maxDepth              int
	maxProperties         uint64
	typeHints             map[string]string
}

func (o Options) normalized() (Options, *decodeConfig, error) {
	var err error
	o.Limits, err = o.Limits.normalized()
	if err != nil {
		return Options{}, nil, err
	}
	if o.MaxStringBytes == 0 {
		o.MaxStringBytes = defaultMaxStringBytes
	}
	if o.MaxPathBytes == 0 {
		o.MaxPathBytes = defaultMaxPathBytes
	}
	if o.MaxCollectionElements == 0 {
		o.MaxCollectionElements = defaultMaxCollectionElements
	}
	if o.MaxDepth == 0 {
		o.MaxDepth = defaultMaxDepth
	}
	if o.MaxProperties == 0 {
		o.MaxProperties = defaultMaxProperties
	}
	if o.MaxStringBytes < 1 || o.MaxPathBytes < 1 || o.MaxCollectionElements < 1 || o.MaxDepth < 1 || o.MaxProperties < 1 {
		return Options{}, nil, fmt.Errorf("palsav: GVAS limits must be positive")
	}
	hints := make(map[string]string, len(palworldTypeHints)+len(o.TypeHints))
	for path, hint := range palworldTypeHints {
		hints[path] = hint
	}
	for path, hint := range o.TypeHints {
		if path == "" || hint == "" {
			return Options{}, nil, fmt.Errorf("palsav: empty type-hint path or value")
		}
		hints[path] = hint
	}
	return o, &decodeConfig{
		maxStringBytes:        o.MaxStringBytes,
		maxPathBytes:          o.MaxPathBytes,
		maxCollectionElements: o.MaxCollectionElements,
		maxDepth:              o.MaxDepth,
		maxProperties:         o.MaxProperties,
		typeHints:             hints,
	}, nil
}

// Save is a decoded .sav container and GVAS object.
type Save struct {
	// Container is zero when the Save came from ParseGVAS rather than Decode
	// or Load.
	Container  ContainerHeader
	Header     GVASHeader
	Properties Properties
	Trailer    []byte
	// Raw is the complete decompressed GVAS archive. Property payloads and
	// Trailer alias this buffer, so callers should treat it as immutable.
	Raw []byte
}

// GVASHeader is Unreal's SaveGame archive header.
type GVASHeader struct {
	SaveGameVersion int32
	PackageUE4      int32
	PackageUE5      int32
	Engine          EngineVersion
	CustomFormat    int32
	CustomVersions  []CustomVersion
	ClassName       string
}

type EngineVersion struct {
	Major      uint16
	Minor      uint16
	Patch      uint16
	ChangeList uint32
	Branch     string
}

type CustomVersion struct {
	ID      GUID
	Version int32
}

// GUID is Unreal's four-uint32 FGuid representation.
type GUID struct {
	A uint32
	B uint32
	C uint32
	D uint32
}

func (g GUID) String() string {
	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%04x%08x",
		g.A,
		g.B>>16,
		g.B&0xffff,
		g.C>>16,
		g.C&0xffff,
		g.D,
	)
}

func (g GUID) IsZero() bool { return g.A|g.B|g.C|g.D == 0 }

func (g GUID) MarshalText() ([]byte, error) { return []byte(g.String()), nil }

// Properties preserves Unreal's serialized property order and array indices.
type Properties []Property

// Find returns the first property with name, or nil.
func (p Properties) Find(name string) *Property {
	for i := range p {
		if p[i].Name == name {
			return &p[i]
		}
	}
	return nil
}

// FindFold is Find with ASCII/Unicode case-insensitive matching.
func (p Properties) FindFold(name string) *Property {
	for i := range p {
		if strings.EqualFold(p[i].Name, name) {
			return &p[i]
		}
	}
	return nil
}

// Property is one legacy Unreal FPropertyTag and its value.
type Property struct {
	Name       string
	Type       string
	Size       uint32
	ArrayIndex uint32
	Meta       PropertyMeta

	PropertyGUID *GUID
	Value        any
	// Raw is the exact size-counted value payload. It aliases the decoded GVAS
	// buffer and remains available even when Value is a RawValue.
	Raw []byte
	// Offset is the absolute byte offset of the property name in Save.Raw.
	Offset int
}

// PropertyMeta is the type-specific part of an Unreal property tag.
type PropertyMeta struct {
	StructType string
	StructGUID GUID
	BoolValue  bool
	EnumType   string
	InnerType  string
	KeyType    string
	ValueType  string
}

// RawValue preserves a payload that is unknown or could not be interpreted.
type RawValue struct {
	Data   []byte
	Reason string
}

// StructValue associates a concrete Unreal struct identity with its value.
// Value is usually Properties, GUID, Vector, Quat, LinearColor, Color, or int64.
type StructValue struct {
	Type  string
	Value any
}

type EnumValue struct {
	Type  string
	Value string
}

type Vector struct {
	X float64
	Y float64
	Z float64
}

type Quat struct {
	X float64
	Y float64
	Z float64
	W float64
}

type LinearColor struct {
	R float32
	G float32
	B float32
	A float32
}

type Color struct {
	B byte
	G byte
	R byte
	A byte
}

type IntPoint struct {
	X int32
	Y int32
}

type IntVector struct {
	X int32
	Y int32
	Z int32
}

type Vector2D struct {
	X float64
	Y float64
}

type Rotator struct {
	Pitch float64
	Yaw   float64
	Roll  float64
}

// ArrayValue represents an ArrayProperty. Primitive arrays use compact typed
// slices in Values. Struct arrays are exposed through Structs.
type ArrayValue struct {
	InnerType string
	Values    any
	Structs   *StructArray
}

type MapEntry struct {
	Key   any
	Value any
}

// MapValue is a lazy, ordered MapProperty. Use Iterator or Entries to decode.
type MapValue struct {
	KeyType         string
	ValueType       string
	KeyStructType   string
	ValueStructType string
	Removed         []any
	Count           uint32

	raw       []byte
	base      int
	path      string
	keyPath   string
	valuePath string
	cfg       *decodeConfig
}

// SetValue is a lazy, ordered SetProperty.
type SetValue struct {
	ElementType       string
	ElementStructType string
	Removed           []any
	Count             uint32

	raw  []byte
	base int
	path string
	cfg  *decodeConfig
}

// StructArray lazily exposes the concatenated values in an
// ArrayProperty<StructProperty>.
type StructArray struct {
	Count uint32
	Name  string
	Type  string
	// Size is the descriptor's aggregate byte size for every array element,
	// not the size of one element.
	Size         uint32
	ArrayIndex   uint32
	StructType   string
	StructGUID   GUID
	PropertyGUID *GUID

	raw  []byte
	base int
	path string
	cfg  *decodeConfig
}
