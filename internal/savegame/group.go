// Portions derived from Palhelm and modified for Palworld Live Map.
// Copyright 2026 Palhelm contributors. Licensed under Apache-2.0.
package savegame

import "fmt"

const (
	groupGuild        = "EPalGroupType::Guild"
	groupIndependent  = "EPalGroupType::IndependentGuild"
	groupOrganization = "EPalGroupType::Organization"
)

type guild struct {
	ID            string
	Name          string
	GroupType     string
	AdminUID      string
	MemberUIDs    []string
	Members       []guildMember
	BaseIDs       []string
	BaseCampLevel int32
}

type guildMember struct {
	UID        string
	Name       string
	LastOnline int64
}

// guildLeadingBytes is the reserved word retail Palworld writes between org_type
// and base_ids inside a guild group.
const guildLeadingBytes = 4

// guildTailVariants enumerate the opaque byte blocks that surround the guild
// admin/member section across Palworld patches. Retail 1.x (proven from live
// day-36 and day-38 saves) writes 14 bytes between last_guild_name_modifier and
// admin_player_uid and 31 bytes after the player list; the palworld-save-tools
// intermediate format used 4/4; older builds none. decodeGuildTail tries them in
// order and commits the first that lands exactly on EOF, so no magic width is
// assumed blindly. Most-specific (retail) first.
var guildTailVariants = []struct{ preAdmin, postPlayers int }{
	{14, 31},
	{4, 4},
	{0, 0},
}

func decodeGroup(raw []byte, groupType string, stats *Stats) (guild, error) {
	r := newReaderWithStats(raw, stats)
	id, err := readGUID(r)
	if err != nil {
		return guild{}, err
	}
	groupName, err := r.fstring()
	if err != nil {
		return guild{}, err
	}
	handles, err := r.u32()
	if err != nil {
		return guild{}, err
	}
	if err = validateCount("character-handle", handles, r.remaining(), 32); err != nil {
		return guild{}, err
	}
	if err = r.skip(int(handles) * 32); err != nil {
		return guild{}, err
	}
	g := guild{ID: id, Name: groupName, GroupType: groupType, MemberUIDs: []string{}, BaseIDs: []string{}}
	if groupType == groupGuild || groupType == groupIndependent || groupType == groupOrganization {
		if _, err = r.u8(); err != nil {
			return guild{}, err
		} // org_type
	}
	switch groupType {
	case groupOrganization:
		if err = r.skip(12); err != nil {
			return guild{}, err
		}
	case groupGuild:
		if err = decodeGuildBranch(r, &g, stats); err != nil {
			return guild{}, err
		}
		return g, nil
	case groupIndependent:
		if err = decodeIndependentBranch(r, &g, stats); err != nil {
			return guild{}, err
		}
		return g, nil
	}
	if r.remaining() != 0 {
		return guild{}, fmt.Errorf("%d trailing group bytes", r.remaining())
	}
	return g, nil
}

// decodeGuildBranch decodes an EPalGroupType::Guild raw blob in the proven retail
// Palworld 1.x layout. The prefix (base ids, base-camp level, guild name, last
// modifier) is structural and fully validated; the admin/member/tail section is
// resolved by decodeGuildTail. The reader must be positioned just after org_type.
//
// Layout (after org_type):
//
//	leading_bytes                          4 reserved bytes
//	base_ids                               tarray<guid>
//	unknown_1                              i32
//	base_camp_level                        i32
//	map_object_instance_ids_base_camp_points  tarray<guid>
//	guild_name                             fstring
//	last_guild_name_modifier_player_uid    guid
//	[ opaque pre-admin block ]             (decodeGuildTail)
//	admin_player_uid                       guid
//	players                                tarray<{guid, i64 last_online, fstring}>
//	[ opaque post-player block ]           (decodeGuildTail)
func decodeGuildBranch(r *reader, g *guild, stats *Stats) error {
	if err := r.skip(guildLeadingBytes); err != nil {
		return err
	}
	baseIDs, err := readGUIDArray(r, stats)
	if err != nil {
		return err
	}
	g.BaseIDs = baseIDs
	if _, err = r.i32(); err != nil { // unknown_1
		return err
	}
	if g.BaseCampLevel, err = r.i32(); err != nil {
		return err
	}
	if _, err = readGUIDArray(r, stats); err != nil { // map object instance ids
		return err
	}
	guildName, err := r.fstring()
	if err != nil {
		return err
	}
	if guildName != "" {
		g.Name = guildName
	}
	if _, err = readGUID(r); err != nil { // last_guild_name_modifier_player_uid
		return err
	}
	return decodeGuildTail(r, g, stats)
}

// decodeGuildTail resolves the admin_player_uid, player list and the two opaque
// blocks that bracket it. It trials each known variant on a throwaway reader and
// commits the first that consumes the tail exactly to EOF, guaranteeing a
// zero-tolerance decode on well-formed saves. If no variant fits, it falls back
// to a best-effort member extraction that tolerates an unrecognized trailing
// layout — counted in stats so the drift is observable — rather than discarding
// an otherwise-valid named guild.
func decodeGuildTail(r *reader, g *guild, stats *Stats) error {
	tailStart := r.position()
	tail := r.b[tailStart:]
	// Palworld's July 2026 guild-role update replaced the opaque tail with
	// markers, chest roles, role-bearing members, and permission arrays. Trial
	// this fully structural layout first and accept it only when it reaches EOF.
	if guildV2TailFitsEOF(tail) {
		admin, members, err := readGuildV2Tail(r, stats)
		if err != nil {
			return err
		}
		applyGuildMembers(g, admin, members)
		return nil
	}
	for _, v := range guildTailVariants {
		if !guildTailFitsEOF(tail, v.preAdmin, v.postPlayers) {
			continue
		}
		admin, members, err := readGuildTail(r, v.preAdmin, v.postPlayers, stats)
		if err != nil {
			return err
		}
		applyGuildMembers(g, admin, members)
		if r.remaining() != 0 {
			return fmt.Errorf("%d trailing guild bytes", r.remaining())
		}
		return nil
	}
	// Final fallback: an unrecognized tail layout must not drop the guild. Extract
	// admin + members best-effort with the most common opaque prefix and tolerate
	// the remainder. Recorded so exercised tolerance is visible in Stats.
	if err := r.seek(tailStart); err != nil {
		return err
	}
	admin, members := readGuildTailTolerant(r, guildTailVariants[0].preAdmin, stats)
	applyGuildMembers(g, admin, members)
	stats.recordSkip("worldSaveData.GroupSaveDataMap.Value.RawData.guild-tail", "tolerated")
	return nil
}

func guildV2TailFitsEOF(tail []byte) bool {
	tr := newReader(tail)
	if _, _, err := readGuildV2Tail(tr, tr.stats); err != nil {
		return false
	}
	return tr.remaining() == 0
}

// readGuildV2Tail decodes the observed 2026-07 guild-role layout:
// marker array, chest-role bytes, unknown i32, admin, role-bearing members,
// role permissions, and four reserved trailing bytes.
func readGuildV2Tail(r *reader, stats *Stats) (string, []guildMember, error) {
	markers, err := r.u32()
	if err != nil {
		return "", nil, err
	}
	const markerBytes = 16 + 24 + 4 + 16
	if err = validateCount("guild marker", markers, r.remaining(), markerBytes); err != nil {
		return "", nil, err
	}
	if err = consumeDecoded(stats, "guild marker array", uint64(markers), uint64(markers)*markerBytes); err != nil {
		return "", nil, err
	}
	if err = r.skip(int(markers) * markerBytes); err != nil {
		return "", nil, err
	}
	chestRoles, err := r.u32()
	if err != nil {
		return "", nil, err
	}
	if err = validateCount("guild chest role", chestRoles, r.remaining(), 1); err != nil {
		return "", nil, err
	}
	if err = r.skip(int(chestRoles)); err != nil {
		return "", nil, err
	}
	if _, err = r.i32(); err != nil { // unknown_i32
		return "", nil, err
	}
	admin, err := readGUID(r)
	if err != nil {
		return "", nil, err
	}
	members, err := readGuildMembersV2(r, stats)
	if err != nil {
		return "", nil, err
	}
	permissions, err := r.u32()
	if err != nil {
		return "", nil, err
	}
	if err = validateCount("guild role permission", permissions, r.remaining(), 5); err != nil {
		return "", nil, err
	}
	for range permissions {
		if _, err = r.u8(); err != nil { // role
			return "", nil, err
		}
		count, countErr := r.u32()
		if countErr != nil {
			return "", nil, countErr
		}
		if err = validateCount("guild permission", count, r.remaining(), 1); err != nil {
			return "", nil, err
		}
		if err = r.skip(int(count)); err != nil {
			return "", nil, err
		}
	}
	if err = r.skip(4); err != nil {
		return "", nil, err
	}
	return admin, members, nil
}

func readGuildMembersV2(r *reader, stats *Stats) ([]guildMember, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if err = validateCount("guild member", n, r.remaining(), 29); err != nil {
		return nil, err
	}
	if err = consumeDecoded(stats, "guild member array", uint64(n), uint64(n)*40); err != nil {
		return nil, err
	}
	members := make([]guildMember, 0, n)
	for range n {
		uid, readErr := readGUID(r)
		if readErr != nil {
			return nil, readErr
		}
		last, readErr := r.i64()
		if readErr != nil {
			return nil, readErr
		}
		name, readErr := r.fstring()
		if readErr != nil {
			return nil, readErr
		}
		if _, readErr = r.u8(); readErr != nil { // EPalGuildRole
			return nil, readErr
		}
		members = append(members, guildMember{UID: uid, Name: name, LastOnline: last})
	}
	return members, nil
}

// guildTailFitsEOF reports whether reading [preAdmin opaque][admin guid][players]
// [postPlayers opaque] from tail consumes it exactly. It runs on a fresh reader
// with a throwaway stats budget so trials never mutate the caller's accounting.
func guildTailFitsEOF(tail []byte, preAdmin, postPlayers int) bool {
	tr := newReader(tail)
	if _, _, err := readGuildTail(tr, preAdmin, postPlayers, tr.stats); err != nil {
		return false
	}
	return tr.remaining() == 0
}

// readGuildTail reads the admin/member section for one variant. Counts are bounded
// by readGuildMembers/validateCount so a misaligned trial fails fast rather than
// allocating.
func readGuildTail(r *reader, preAdmin, postPlayers int, stats *Stats) (string, []guildMember, error) {
	if err := r.skip(preAdmin); err != nil {
		return "", nil, err
	}
	admin, err := readGUID(r)
	if err != nil {
		return "", nil, err
	}
	members, err := readGuildMembers(r, stats)
	if err != nil {
		return "", nil, err
	}
	if err := r.skip(postPlayers); err != nil {
		return "", nil, err
	}
	return admin, members, nil
}

// readGuildTailTolerant extracts the admin and, when they decode, the members,
// ignoring any trailing bytes. Used only as the final fallback.
func readGuildTailTolerant(r *reader, preAdmin int, stats *Stats) (string, []guildMember) {
	if err := r.skip(preAdmin); err != nil {
		return "", nil
	}
	admin, err := readGUID(r)
	if err != nil {
		return "", nil
	}
	members, err := readGuildMembers(r, stats)
	if err != nil {
		return admin, nil
	}
	return admin, members
}

// decodeIndependentBranch decodes an EPalGroupType::IndependentGuild blob. Retail
// evidence for this variant is unavailable, so it decodes the known field prefix
// and tolerates any trailing bytes (the same opaque drift proven for guilds is
// likely present) rather than discarding the group. The reader must be positioned
// just after org_type.
func decodeIndependentBranch(r *reader, g *guild, stats *Stats) error {
	var err error
	if g.BaseCampLevel, err = r.i32(); err != nil {
		return err
	}
	if _, err = readGUIDArray(r, stats); err != nil { // map object instance ids
		return err
	}
	guildName, err := r.fstring()
	if err != nil {
		return err
	}
	if guildName != "" {
		g.Name = guildName
	}
	uid, err := readGUID(r)
	if err != nil {
		return err
	}
	if _, err = r.fstring(); err != nil { // guild_name_2
		return err
	}
	last, err := r.i64()
	if err != nil {
		return err
	}
	name, err := r.fstring()
	if err != nil {
		return err
	}
	g.Members = []guildMember{{UID: uid, Name: name, LastOnline: last}}
	g.MemberUIDs = []string{uid}
	g.AdminUID = uid
	if r.remaining() != 0 {
		// Tolerate an unmodeled trailing block instead of dropping the guild.
		stats.recordSkip("worldSaveData.GroupSaveDataMap.Value.RawData.independent-tail", "tolerated")
	}
	return nil
}

func applyGuildMembers(g *guild, admin string, members []guildMember) {
	g.AdminUID = admin
	g.Members = members
	for _, m := range members {
		g.MemberUIDs = append(g.MemberUIDs, m.UID)
	}
}

func readGUIDArray(r *reader, stats *Stats) ([]string, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if err := validateCount("GUID", n, r.remaining(), 16); err != nil {
		return nil, err
	}
	if err := consumeDecoded(stats, "GUID array", uint64(n), uint64(n)*16); err != nil {
		return nil, err
	}
	v := make([]string, 0)
	for range n {
		id, e := readGUID(r)
		if e != nil {
			return nil, e
		}
		v = append(v, id)
	}
	return v, nil
}

func readGuildMembers(r *reader, stats *Stats) ([]guildMember, error) {
	n, err := r.u32()
	if err != nil {
		return nil, err
	}
	if err := validateCount("guild member", n, r.remaining(), 28); err != nil {
		return nil, err
	}
	if err := consumeDecoded(stats, "guild member array", uint64(n), uint64(n)*40); err != nil {
		return nil, err
	}
	v := make([]guildMember, 0)
	for range n {
		id, e := readGUID(r)
		if e != nil {
			return nil, e
		}
		last, e := r.i64()
		if e != nil {
			return nil, e
		}
		name, e := r.fstring()
		if e != nil {
			return nil, e
		}
		v = append(v, guildMember{UID: id, Name: name, LastOnline: last})
	}
	return v, nil
}
