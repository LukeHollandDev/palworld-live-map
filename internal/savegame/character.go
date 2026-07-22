// Portions derived from Palhelm and modified for Palworld Live Map.
// Copyright 2026 Palhelm contributors. Licensed under Apache-2.0.
package savegame

import "fmt"

const zeroGUID = "00000000-0000-0000-0000-000000000000"

type levelPlayer struct {
	UID   string
	Name  string
	Level int32
}

func playerFromCharacterEntry(e mapEntry, stats *Stats) (*levelPlayer, error) {
	key, ok := asProperties(e.Key)
	if !ok {
		return nil, fmt.Errorf("character key is not a struct")
	}
	uid := firstString(key, "PlayerUId", "PlayerUID", "PlayerUid")
	// Every non-player character in a 1.0 world uses the zero player GUID. This
	// lets us avoid custom-decoding thousands of Pal/NPC RawData blobs.
	if uid == "" || uid == zeroGUID {
		return nil, nil
	}
	value, ok := asProperties(e.Value)
	if !ok {
		return nil, fmt.Errorf("character value is not a struct")
	}
	rawProp := value["RawData"]
	if rawProp == nil {
		return nil, fmt.Errorf("character has no RawData")
	}
	raw, ok := rawProp.Value.([]byte)
	if !ok {
		return nil, fmt.Errorf("character RawData is %T", rawProp.Value)
	}
	sp, err := decodeCharacterSaveParameter(raw, stats)
	if err != nil {
		return nil, err
	}
	if !firstBool(sp, "IsPlayer") {
		return nil, fmt.Errorf("non-zero player GUID is not marked IsPlayer")
	}
	level := firstInt(sp, "Level")
	if level < 0 || level > 1_000 {
		return nil, fmt.Errorf("invalid player level")
	}
	return &levelPlayer{
		UID:   uid,
		Name:  firstString(sp, "NickName", "Nickname"),
		Level: int32(level),
	}, nil
}

func decodeCharacterSaveParameter(raw []byte, stats *Stats) (propertyMap, error) {
	r := newReaderWithStats(raw, stats)
	obj, err := readProperties(r, ".worldSaveData.CharacterSaveParameterMap.Value.RawData", stats)
	if err != nil {
		return nil, err
	}
	if err = r.skip(4); err != nil {
		return nil, err
	}
	if _, err = readGUID(r); err != nil { // character group id
		return nil, err
	}
	if err = r.skip(4); err != nil {
		return nil, err
	}
	if r.remaining() != 0 {
		stats.recordSkip("worldSaveData.CharacterSaveParameterMap.Value.RawData.trailing", "tolerated")
	}
	if nested, ok := propertyProperties(obj, "SaveParameter"); ok {
		return nested, nil
	}
	return obj, nil
}
