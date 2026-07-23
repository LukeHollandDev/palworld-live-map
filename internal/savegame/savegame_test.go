// Portions of the synthetic builders are derived from Palhelm's Apache-2.0
// fixture tests. All names and GUIDs are invented.
package savegame

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
)

func TestNewReaderRequiresDecoder(t *testing.T) {
	_, err := NewReader(Options{})
	if err == nil || !strings.Contains(err.Error(), "decoder path") {
		t.Fatalf("NewReader() error = %v", err)
	}
}

type testWriter struct{ b []byte }

func (w *testWriter) bytes(v []byte)  { w.b = append(w.b, v...) }
func (w *testWriter) u8(v byte)       { w.b = append(w.b, v) }
func (w *testWriter) u32(v uint32)    { w.b = binary.LittleEndian.AppendUint32(w.b, v) }
func (w *testWriter) i32(v int32)     { w.u32(uint32(v)) }
func (w *testWriter) u64(v uint64)    { w.b = binary.LittleEndian.AppendUint64(w.b, v) }
func (w *testWriter) i64(v int64)     { w.u64(uint64(v)) }
func (w *testWriter) guid(v [16]byte) { w.bytes(v[:]) }
func (w *testWriter) fstring(v string) {
	if v == "" {
		w.i32(0)
		return
	}
	w.i32(int32(len(v) + 1))
	w.bytes([]byte(v))
	w.u8(0)
}

func syntheticGUID(tag byte) [16]byte {
	var value [16]byte
	for i := range value {
		value[i] = tag
	}
	return value
}

func propertyHeader(name, typ string, size uint64) []byte {
	w := &testWriter{}
	w.fstring(name)
	w.fstring(typ)
	w.u64(size)
	return w.b
}

func testBoolProperty(name string, value bool) []byte {
	w := &testWriter{}
	w.bytes(propertyHeader(name, "BoolProperty", 0))
	if value {
		w.u8(1)
	} else {
		w.u8(0)
	}
	w.u8(0) // optional GUID absent
	return w.b
}

func testStringProperty(name, value string) []byte {
	body := &testWriter{}
	body.u8(0)
	body.fstring(value)
	w := &testWriter{}
	w.bytes(propertyHeader(name, "StrProperty", uint64(len(body.b))))
	w.bytes(body.b)
	return w.b
}

func testByteProperty(name string, value byte) []byte {
	body := &testWriter{}
	body.fstring("None")
	body.u8(0)
	body.u8(value)
	w := &testWriter{}
	w.bytes(propertyHeader(name, "ByteProperty", uint64(len(body.b))))
	w.bytes(body.b)
	return w.b
}

func testStructProperty(name, typ string, body []byte) []byte {
	preamble := &testWriter{}
	preamble.fstring(typ)
	preamble.guid([16]byte{})
	preamble.u8(0)
	w := &testWriter{}
	w.bytes(propertyHeader(name, "StructProperty", uint64(len(body))))
	w.bytes(preamble.b)
	w.bytes(body)
	return w.b
}

func testByteArrayProperty(name string, value []byte) []byte {
	body := &testWriter{}
	body.u32(uint32(len(value)))
	body.bytes(value)
	preamble := &testWriter{}
	preamble.fstring("ByteProperty")
	preamble.u8(0)
	w := &testWriter{}
	w.bytes(propertyHeader(name, "ArrayProperty", uint64(len(body.b))))
	w.bytes(preamble.b)
	w.bytes(body.b)
	return w.b
}

func syntheticCharacterRaw(name string, level byte, group [16]byte) []byte {
	params := &testWriter{}
	params.bytes(testBoolProperty("IsPlayer", true))
	params.bytes(testStringProperty("NickName", name))
	params.bytes(testByteProperty("Level", level))
	params.fstring("None")
	object := &testWriter{}
	object.bytes(testStructProperty("SaveParameter", "PalIndividualCharacterSaveParameter", params.b))
	object.fstring("None")
	object.u32(0)
	object.guid(group)
	object.u32(0)
	return object.b
}

func syntheticGuildV2Raw(id, playerID [16]byte, name string) []byte {
	w := &testWriter{}
	w.guid(id)
	w.fstring("")
	w.u32(0) // character handles
	w.u8(0)  // org type
	w.u32(0) // leading reserved bytes
	w.u32(0) // base ids
	w.i32(0)
	w.i32(3)
	w.u32(0) // base-camp point ids
	w.fstring(name)
	w.guid(playerID) // last name modifier
	w.u32(0)         // guild markers
	w.u32(2)         // chest roles
	w.bytes([]byte{1, 2})
	w.i32(0)
	w.guid(playerID) // admin
	w.u32(1)         // players
	w.guid(playerID)
	w.i64(638900000000000000)
	w.fstring("Synthetic Player")
	w.u8(1)  // role
	w.u32(1) // role permissions
	w.u8(1)
	w.u32(2)
	w.bytes([]byte{1, 2})
	w.u32(0) // trailing reserved bytes
	return w.b
}

func guidString(tag byte) string {
	return stringGUID(syntheticGUID(tag))
}

func stringGUID(value [16]byte) string {
	r := newReader(value[:])
	id, err := readGUID(r)
	if err != nil {
		panic(err)
	}
	return id
}

func TestExtractLevelPlayersWithGuildV2(t *testing.T) {
	playerID := syntheticGUID(0x11)
	groupID := syntheticGUID(0x22)
	playerIDString := stringGUID(playerID)
	groupIDString := stringGUID(groupID)
	characterKey := propertyMap{
		"PlayerUId": {Type: "StructProperty", Value: structData{Type: "Guid", Value: playerIDString}},
	}
	characterValue := propertyMap{
		"RawData": {Type: "ArrayProperty", Value: syntheticCharacterRaw("Synthetic Player", 57, groupID)},
	}
	groupValue := propertyMap{
		"GroupType": {Type: "EnumProperty", Value: enumData{Type: "EPalGroupType", Value: groupGuild}},
		"RawData":   {Type: "ArrayProperty", Value: syntheticGuildV2Raw(groupID, playerID, "Synthetic Guild")},
	}
	world := propertyMap{
		"CharacterSaveParameterMap": {Type: "MapProperty", Value: []mapEntry{{Key: characterKey, Value: characterValue}}},
		"GroupSaveDataMap":          {Type: "MapProperty", Value: []mapEntry{{Key: groupIDString, Value: groupValue}}},
	}
	gvas := &gvasFile{Properties: propertyMap{
		"worldSaveData": {Type: "StructProperty", Value: structData{Type: "PalWorldSaveData", Value: world}},
	}}
	stats := newStats()
	players, err := extractLevelPlayers(gvas, 10, &stats)
	if err != nil {
		t.Fatal(err)
	}
	if len(players) != 1 {
		t.Fatalf("players=%d, want 1", len(players))
	}
	got := players[0]
	if got.PlayerID != playerIDString || got.DisplayName != "Synthetic Player" || got.Level != 57 {
		t.Fatalf("identity = %#v", got)
	}
	if got.GuildID != groupIDString || got.GuildName != "Synthetic Guild" {
		t.Fatalf("guild = %#v", got)
	}
	if stats.DecodeFailures["guilds"] != 0 {
		t.Fatalf("guild decode failures: %#v", stats.DecodeFailures)
	}
}

func TestPlayerStateUsesPersistedTransformAndDateTime(t *testing.T) {
	uid := guidString(0x33)
	ticks := unrealUnixEpochTicks + uint64((24*time.Hour+2*time.Second)/100)
	saveData := propertyMap{
		"PlayerUId": {Type: "StructProperty", Value: structData{Type: "Guid", Value: uid}},
		"LastTransform": {Type: "StructProperty", Value: structData{Type: "Transform", Value: propertyMap{
			"Translation": {Type: "StructProperty", Value: structData{Type: "Vector", Value: Vector{X: 123.5, Y: -456.25, Z: 7}}},
		}}},
		"LastOnlineDateTime": {Type: "StructProperty", Value: structData{Type: "DateTime", Value: ticks}},
	}
	gvas := &gvasFile{Properties: propertyMap{
		"SaveData": {Type: "StructProperty", Value: structData{Type: "PalPlayerSaveData", Value: saveData}},
	}}
	state, err := playerStateFromGVAS(gvas)
	if err != nil {
		t.Fatal(err)
	}
	if state.uid != uid || state.location == nil || state.location.X != 123.5 || state.location.Y != -456.25 {
		t.Fatalf("state=%#v", state)
	}
	want := time.Unix(86402, 0).UTC()
	if state.lastSeen == nil || !state.lastSeen.Equal(want) {
		t.Fatalf("lastSeen=%v, want %v", state.lastSeen, want)
	}
}

func TestPlayerStateUsesAuthoritativeRecordDataProgress(t *testing.T) {
	uid := guidString(0x34)
	saveData := propertyMap{
		"PlayerUId": {Type: "StructProperty", Value: structData{Type: "Guid", Value: uid}},
		"RecordData": {Type: "StructProperty", Value: structData{Value: propertyMap{
			"TribeCaptureCount": {Type: "IntProperty", Value: int32(321)},
			"PalCaptureCount": {Type: "MapProperty", Value: []mapEntry{
				{Key: "SheepBall", Value: int32(4)},
				{Key: "Anubis", Value: int32(1)},
				{Key: "Human", Value: int32(7)},
				{Key: "NeverCaught", Value: int32(0)},
			}},
			"PaldeckUnlockFlag": {Type: "MapProperty", Value: []mapEntry{
				{Key: "SheepBall", Value: true},
				{Key: "Anubis", Value: false},
			}},
		}}},
	}
	gvas := &gvasFile{Properties: propertyMap{
		"SaveData": {Type: "StructProperty", Value: structData{Value: saveData}},
	}}
	state, err := playerStateFromGVAS(gvas)
	if err != nil {
		t.Fatal(err)
	}
	if state.captureTotal == nil || *state.captureTotal != 5 ||
		state.uniquePalsCaptured == nil || *state.uniquePalsCaptured != 321 ||
		state.paldeckUnlocked == nil || *state.paldeckUnlocked != 1 {
		t.Fatalf("progress = capture %v, unique %v, Paldeck %v", state.captureTotal, state.uniquePalsCaptured, state.paldeckUnlocked)
	}

	missing := &gvasFile{Properties: propertyMap{
		"SaveData": {Type: "StructProperty", Value: structData{Value: propertyMap{
			"PlayerUId": {Type: "StructProperty", Value: structData{Type: "Guid", Value: uid}},
		}}},
	}}
	state, err = playerStateFromGVAS(missing)
	if err != nil {
		t.Fatal(err)
	}
	if state.captureTotal != nil || state.uniquePalsCaptured != nil || state.paldeckUnlocked != nil {
		t.Fatalf("missing progress should remain unavailable: %#v", state)
	}
}

func TestPlayerProgressRejectsPartialOrMalformedAggregates(t *testing.T) {
	state := playerState{}
	decodePlayerProgress(propertyMap{
		"RecordData": {Type: "StructProperty", Value: structData{Value: propertyMap{
			"TribeCaptureCount": {Type: "IntProperty", Value: int32(-1)},
			"PalCaptureCount": {Type: "MapProperty", Value: []mapEntry{
				{Key: "SheepBall", Value: int32(4)},
				{Key: "Broken", Value: int32(-1)},
			}},
			"PaldeckUnlockFlag": {Type: "MapProperty", Value: []mapEntry{
				{Key: "SheepBall", Value: true},
				{Key: "Broken", Value: int32(1)},
			}},
		}}},
	}, &state)
	if state.captureTotal != nil || state.uniquePalsCaptured != nil || state.paldeckUnlocked != nil {
		t.Fatalf("malformed progress should remain unavailable, got %#v", state)
	}
}

type fakeDecoder struct{ raw []byte }

func (f fakeDecoder) Decompress([]byte, int) ([]byte, error) {
	return append([]byte(nil), f.raw...), nil
}

func TestReadPlMContainerUsesInjectedDecoder(t *testing.T) {
	raw := []byte("GVAS synthetic")
	w := &testWriter{}
	w.u32(uint32(len(raw)))
	w.u32(3)
	w.bytes([]byte("PlM"))
	w.u8(0x31)
	w.bytes([]byte{1, 2, 3})
	got, header, err := readContainer(w.b, 1024, fakeDecoder{raw: raw})
	if err != nil {
		t.Fatal(err)
	}
	if header.Magic != "PlM" || string(got) != string(raw) {
		t.Fatalf("header=%+v raw=%q", header, got)
	}
}

func TestUnrealDateTimeRejectsOutOfRange(t *testing.T) {
	if _, ok := unrealDateTime(unrealUnixEpochTicks - 1); ok {
		t.Fatal("pre-Unix timestamp accepted")
	}
	if got, ok := unrealDateTime(unrealUnixEpochTicks + 12_345_678); !ok || !got.Equal(time.Unix(1, 234_567_800).UTC()) {
		t.Fatalf("got %v, ok=%v", got, ok)
	}
}

func TestSelectiveSetPropertySkipPreservesAlignment(t *testing.T) {
	body := &testWriter{}
	body.u32(0) // empty set
	w := &testWriter{}
	w.fstring("InLockerCharacterInstanceIDArray")
	w.fstring("SetProperty")
	w.u64(uint64(len(body.b)))
	w.fstring("StructProperty")
	w.u8(0) // optional GUID absent
	w.bytes(body.b)
	w.fstring("None")
	stats := newStats()
	properties, err := readProperties(newReaderWithStats(w.b, &stats), ".worldSaveData", &stats)
	if err != nil {
		t.Fatal(err)
	}
	if len(properties) != 0 {
		t.Fatalf("selectively skipped properties = %#v, want empty", properties)
	}
}
