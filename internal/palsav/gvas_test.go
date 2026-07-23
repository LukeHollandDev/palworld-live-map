// Copyright (C) 2026 Luke Holland
// Portions of the synthetic builders were adapted from Apache-2.0 Palhelm
// test code and substantially modified on 2026-07-23. See NOTICE.
// SPDX-License-Identifier: GPL-3.0-or-later

package palsav

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"
	"unicode/utf16"
)

func TestPropertyTagLayoutAndFallback(t *testing.T) {
	guid := GUID{
		A: 0x40d2fba7,
		B: 0x4b484ce5,
		C: 0xb0385a75,
		D: 0x884e499e,
	}
	var properties testArchive
	properties.property(
		"Duplicate",
		"IntProperty",
		testU32(41),
		7,
		nil,
		&guid,
	)
	properties.property(
		"Duplicate",
		"IntProperty",
		testU32(42),
		8,
		nil,
		nil,
	)
	properties.property(
		"Enabled",
		"BoolProperty",
		nil,
		0,
		func(tag *testArchive) { tag.u8(1) },
		nil,
	)
	properties.property(
		"BadInt",
		"IntProperty",
		[]byte{1, 2},
		0,
		nil,
		nil,
	)
	properties.property(
		"Mystery",
		"StructProperty",
		[]byte{1, 2, 3, 4},
		0,
		func(tag *testArchive) {
			tag.fstring("NewNativeStruct")
			tag.guid(GUID{})
		},
		nil,
	)
	properties.property(
		"Unicode",
		"StrProperty",
		testUTF16String("Pal 世界"),
		0,
		nil,
		nil,
	)
	properties.fstring("None")

	save, err := ParseGVAS(testGVAS(properties.bytes(), []byte{0, 0, 0, 0}))
	if err != nil {
		t.Fatal(err)
	}
	if len(save.Properties) != 6 {
		t.Fatalf("property count = %d", len(save.Properties))
	}
	first := save.Properties[0]
	if first.Name != "Duplicate" || first.ArrayIndex != 7 || first.Value != int32(41) {
		t.Fatalf("first property = %+v", first)
	}
	if first.PropertyGUID == nil || *first.PropertyGUID != guid {
		t.Fatalf("property GUID = %v", first.PropertyGUID)
	}
	if got := first.PropertyGUID.String(); got != "40d2fba7-4b48-4ce5-b038-5a75884e499e" {
		t.Fatalf("property GUID string = %q", got)
	}
	if save.Properties[1].ArrayIndex != 8 || save.Properties[1].Value != int32(42) {
		t.Fatalf("duplicate property = %+v", save.Properties[1])
	}
	if save.Properties[2].Value != true {
		t.Fatalf("bool = %#v", save.Properties[2].Value)
	}
	for _, index := range []int{3, 4} {
		if _, ok := save.Properties[index].Value.(RawValue); !ok {
			t.Fatalf("%s value = %T, want RawValue", save.Properties[index].Name, save.Properties[index].Value)
		}
	}
	if save.Properties[5].Value != "Pal 世界" {
		t.Fatalf("Unicode = %#v", save.Properties[5].Value)
	}
}

func TestLazyCollectionsWithRemovedValues(t *testing.T) {
	var mapPayload testArchive
	mapPayload.u32(1)
	mapPayload.i32(9)
	mapPayload.u32(2)
	mapPayload.i32(1)
	mapPayload.fstring("one")
	mapPayload.i32(2)
	mapPayload.fstring("two")

	var setPayload testArchive
	setPayload.u32(1)
	setPayload.u32(7)
	setPayload.u32(2)
	setPayload.u32(8)
	setPayload.u32(9)

	var structDescriptor testArchive
	structDescriptor.property(
		"Items",
		"StructProperty",
		nil,
		0,
		func(tag *testArchive) {
			tag.fstring("PalEmptyStruct")
			tag.guid(GUID{})
		},
		nil,
	)
	var structArrayPayload testArchive
	structArrayPayload.u32(0)
	structArrayPayload.data = append(structArrayPayload.data, structDescriptor.bytes()...)

	var properties testArchive
	properties.property(
		"Numbers",
		"MapProperty",
		mapPayload.bytes(),
		0,
		func(tag *testArchive) {
			tag.fstring("IntProperty")
			tag.fstring("StrProperty")
		},
		nil,
	)
	properties.property(
		"Set",
		"SetProperty",
		setPayload.bytes(),
		0,
		func(tag *testArchive) { tag.fstring("UInt32Property") },
		nil,
	)
	properties.property(
		"Items",
		"ArrayProperty",
		structArrayPayload.bytes(),
		0,
		func(tag *testArchive) { tag.fstring("StructProperty") },
		nil,
	)
	properties.fstring("None")

	save, err := ParseGVAS(testGVAS(properties.bytes(), nil))
	if err != nil {
		t.Fatal(err)
	}
	mapValue := propertyValue[*MapValue](t, save.Properties, "Numbers")
	if len(mapValue.Removed) != 1 || mapValue.Removed[0] != int32(9) {
		t.Fatalf("removed map keys = %#v", mapValue.Removed)
	}
	entries, err := mapValue.Entries()
	if err != nil {
		t.Fatal(err)
	}
	wantEntries := []MapEntry{{Key: int32(1), Value: "one"}, {Key: int32(2), Value: "two"}}
	if len(entries) != len(wantEntries) {
		t.Fatalf("entries = %#v", entries)
	}
	for i := range entries {
		if entries[i] != wantEntries[i] {
			t.Fatalf("entry %d = %#v, want %#v", i, entries[i], wantEntries[i])
		}
	}

	setValue := propertyValue[*SetValue](t, save.Properties, "Set")
	if len(setValue.Removed) != 1 || setValue.Removed[0] != uint32(7) {
		t.Fatalf("removed set values = %#v", setValue.Removed)
	}
	values, err := setValue.Values()
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || values[0] != uint32(8) || values[1] != uint32(9) {
		t.Fatalf("set values = %#v", values)
	}

	array := propertyValue[ArrayValue](t, save.Properties, "Items").Structs
	if array == nil || array.Count != 0 || array.StructType != "PalEmptyStruct" {
		t.Fatalf("struct array = %+v", array)
	}
	iterator := array.Iterator()
	if iterator.Next() || iterator.Err() != nil {
		t.Fatalf("zero struct array iteration: next=%v err=%v", iterator.Next(), iterator.Err())
	}
}

func TestLimitsPropagateInsteadOfBecomingRaw(t *testing.T) {
	var inner testArchive
	inner.property("Value", "IntProperty", testU32(1), 0, nil, nil)
	inner.fstring("None")

	var outer testArchive
	outer.property(
		"Nested",
		"StructProperty",
		inner.bytes(),
		0,
		func(tag *testArchive) {
			tag.fstring("NestedStruct")
			tag.guid(GUID{})
		},
		nil,
	)
	outer.fstring("None")

	_, err := ParseGVASWithOptions(testGVAS(outer.bytes(), nil), Options{MaxDepth: 1})
	var limitErr *LimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("error = %v, want LimitError", err)
	}

	_, err = ParseGVASWithOptions(testGVAS(outer.bytes(), nil), Options{MaxPathBytes: 4})
	if !errors.As(err, &limitErr) || limitErr.Kind != "property path bytes" {
		t.Fatalf("path error = %v, want property-path LimitError", err)
	}
}

func TestMalformedFStringAndPropertySize(t *testing.T) {
	var malformed testArchive
	malformed.i32(math.MinInt32)
	if _, err := ParseGVAS(testGVAS(malformed.bytes(), nil)); err == nil {
		t.Fatal("accepted MinInt32 FString length")
	}

	var property testArchive
	property.fstring("Value")
	property.fstring("IntProperty")
	property.u32(100)
	property.u32(0)
	property.u8(0)
	if _, err := ParseGVAS(testGVAS(property.bytes(), nil)); err == nil {
		t.Fatal("accepted property payload past EOF")
	}
}

func FuzzParseGVAS(f *testing.F) {
	f.Add(testGVAS(testFStringBytes("None"), nil))
	f.Add([]byte("GVAS"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = ParseGVASWithOptions(data, Options{
			Limits:                Limits{MaxInputBytes: 1 << 20, MaxOutputBytes: 1 << 20},
			MaxStringBytes:        1 << 16,
			MaxCollectionElements: 1 << 16,
			MaxProperties:         1 << 16,
			MaxDepth:              32,
		})
	})
}

func FuzzDecodeContainer(f *testing.F) {
	f.Add([]byte("not a save"))
	raw := []byte("GVAS fuzz seed")
	stored := len(raw) - 1
	body := []byte{
		0x8c, 0x0a,
		byte(stored >> 16), byte(stored >> 8), byte(stored),
	}
	body = append(body, raw...)
	f.Add(testContainer(raw, "PlM", 0x31, uint32(len(body)), body, nil))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = DecodeContainerWithLimits(data, Limits{
			MaxInputBytes:  1 << 20,
			MaxOutputBytes: 1 << 20,
		})
	})
}

type testArchive struct {
	data []byte
}

func (archive *testArchive) bytes() []byte { return archive.data }

func (archive *testArchive) u8(value byte) {
	archive.data = append(archive.data, value)
}

func (archive *testArchive) u16(value uint16) {
	var raw [2]byte
	binary.LittleEndian.PutUint16(raw[:], value)
	archive.data = append(archive.data, raw[:]...)
}

func (archive *testArchive) u32(value uint32) {
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], value)
	archive.data = append(archive.data, raw[:]...)
}

func (archive *testArchive) i32(value int32) { archive.u32(uint32(value)) }

func (archive *testArchive) fstring(value string) {
	archive.data = append(archive.data, testFStringBytes(value)...)
}

func (archive *testArchive) guid(value GUID) {
	archive.u32(value.A)
	archive.u32(value.B)
	archive.u32(value.C)
	archive.u32(value.D)
}

func (archive *testArchive) property(
	name string,
	propertyType string,
	payload []byte,
	arrayIndex uint32,
	metadata func(*testArchive),
	propertyGUID *GUID,
) {
	archive.fstring(name)
	archive.fstring(propertyType)
	archive.u32(uint32(len(payload)))
	archive.u32(arrayIndex)
	if metadata != nil {
		metadata(archive)
	}
	if propertyGUID == nil {
		archive.u8(0)
	} else {
		archive.u8(1)
		archive.guid(*propertyGUID)
	}
	archive.data = append(archive.data, payload...)
}

func testGVAS(properties, trailer []byte) []byte {
	var archive testArchive
	archive.data = append(archive.data, "GVAS"...)
	archive.i32(3)
	archive.i32(522)
	archive.i32(1008)
	archive.u16(5)
	archive.u16(1)
	archive.u16(1)
	archive.u32(0)
	archive.fstring("++UE5+Release-5.1")
	archive.i32(3)
	archive.u32(0)
	archive.fstring("/Script/Pal.SyntheticSaveGame")
	archive.data = append(archive.data, properties...)
	archive.data = append(archive.data, trailer...)
	return archive.data
}

func testFStringBytes(value string) []byte {
	var archive testArchive
	archive.i32(int32(len(value) + 1))
	archive.data = append(archive.data, value...)
	archive.u8(0)
	return archive.data
}

func testUTF16String(value string) []byte {
	units := utf16.Encode([]rune(value))
	var archive testArchive
	archive.i32(-int32(len(units) + 1))
	for _, unit := range units {
		archive.u16(unit)
	}
	archive.u16(0)
	return archive.data
}

func testU32(value uint32) []byte {
	var raw [4]byte
	binary.LittleEndian.PutUint32(raw[:], value)
	return raw[:]
}

func TestTestGVASBuilder(t *testing.T) {
	data := testGVAS(testFStringBytes("None"), []byte{0})
	if !bytes.HasPrefix(data, []byte("GVAS")) {
		t.Fatal("bad synthetic GVAS")
	}
}
