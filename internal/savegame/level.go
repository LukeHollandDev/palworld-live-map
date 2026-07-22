// Portions derived from Palhelm and modified for Palworld Live Map.
// Copyright 2026 Palhelm contributors. Licensed under Apache-2.0.
package savegame

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const maxDisplayTextBytes = 1 << 10

type guildMembership struct {
	ID   string
	Name string
}

func extractLevelPlayers(gvas *gvasFile, maxPlayers int, stats *Stats) ([]Player, error) {
	root, ok := propertyProperties(gvas.Properties, "worldSaveData")
	if !ok {
		return nil, fmt.Errorf("savegame: Level.sav has no worldSaveData")
	}
	memberships := make(map[string]guildMembership)
	if prop := root["GroupSaveDataMap"]; prop != nil {
		entries, ok := prop.Value.([]mapEntry)
		if !ok {
			return nil, fmt.Errorf("savegame: GroupSaveDataMap has unexpected type")
		}
		for _, entry := range entries {
			group, err := guildFromEntry(entry, stats)
			if err != nil {
				stats.DecodeFailures["guilds"]++
				continue
			}
			if group.GroupType != groupGuild && group.GroupType != groupIndependent {
				continue
			}
			if !validDisplayText(group.Name, true) {
				stats.DecodeFailures["guilds"]++
				continue
			}
			for _, uid := range group.MemberUIDs {
				if !validGUID(uid) || uid == zeroGUID {
					stats.DecodeFailures["guildMembers"]++
					continue
				}
				key := strings.ToLower(uid)
				candidate := guildMembership{ID: group.ID, Name: group.Name}
				if previous, exists := memberships[key]; exists && previous.ID != candidate.ID {
					stats.GuildConflicts++
					// Deterministically retain the lexicographically smaller guild id.
					if previous.ID < candidate.ID {
						continue
					}
				}
				memberships[key] = candidate
			}
		}
	}

	characters := root["CharacterSaveParameterMap"]
	if characters == nil {
		return nil, fmt.Errorf("savegame: Level.sav has no CharacterSaveParameterMap")
	}
	entries, ok := characters.Value.([]mapEntry)
	if !ok {
		return nil, fmt.Errorf("savegame: CharacterSaveParameterMap has unexpected type")
	}
	players := make(map[string]Player)
	for _, entry := range entries {
		levelPlayer, err := playerFromCharacterEntry(entry, stats)
		if err != nil {
			stats.DecodeFailures["characters"]++
			continue
		}
		if levelPlayer == nil {
			continue
		}
		if !validGUID(levelPlayer.UID) || levelPlayer.UID == zeroGUID || !validDisplayText(levelPlayer.Name, false) {
			stats.DecodeFailures["characters"]++
			continue
		}
		key := strings.ToLower(levelPlayer.UID)
		p := Player{PlayerID: key, DisplayName: levelPlayer.Name, Level: levelPlayer.Level}
		if membership, exists := memberships[key]; exists {
			p.GuildID = strings.ToLower(membership.ID)
			p.GuildName = membership.Name
		}
		if previous, exists := players[key]; exists {
			stats.DuplicatePlayers++
			if previous.Level > p.Level || (previous.Level == p.Level && previous.DisplayName <= p.DisplayName) {
				continue
			}
		}
		players[key] = p
		if len(players) > maxPlayers {
			return nil, &parseLimitError{Kind: "players", Value: uint64(len(players)), Limit: uint64(maxPlayers)}
		}
	}
	if len(players) == 0 {
		return nil, fmt.Errorf("savegame: no valid players in CharacterSaveParameterMap")
	}
	out := make([]Player, 0, len(players))
	for _, player := range players {
		out = append(out, player)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PlayerID < out[j].PlayerID })
	return out, nil
}

func guildFromEntry(entry mapEntry, stats *Stats) (guild, error) {
	value, ok := asProperties(entry.Value)
	if !ok {
		return guild{}, fmt.Errorf("group value is not a struct")
	}
	groupType := firstString(value, "GroupType")
	rawProp := value["RawData"]
	if rawProp == nil {
		return guild{}, fmt.Errorf("group has no RawData")
	}
	raw, ok := rawProp.Value.([]byte)
	if !ok {
		return guild{}, fmt.Errorf("group RawData is %T", rawProp.Value)
	}
	group, err := decodeGroup(raw, groupType, stats)
	if err != nil {
		return guild{}, err
	}
	if mapID, ok := entry.Key.(string); ok && mapID != "" {
		if !strings.EqualFold(mapID, group.ID) {
			return guild{}, fmt.Errorf("group map key does not match embedded id")
		}
		group.ID = strings.ToLower(mapID)
	}
	if !validGUID(group.ID) || group.ID == zeroGUID {
		return guild{}, fmt.Errorf("invalid group id")
	}
	return group, nil
}

func validDisplayText(value string, allowEmpty bool) bool {
	if value == "" {
		return allowEmpty
	}
	if len(value) > maxDisplayTextBytes || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if r == 0 || r == '\r' || r == '\n' {
			return false
		}
	}
	return true
}

func validGUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	for i := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			continue
		}
		c := value[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
