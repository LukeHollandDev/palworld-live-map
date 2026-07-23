// Copyright (C) 2016 Powzix
// Copyright (C) 2026 Luke Holland
//
// Ported to Go and substantially modified on 2026-07-23 from the ooz decoder
// in a GPL-labelled PalworldSaveTools package. See NOTICE for exact provenance
// and the unresolved original upstream grant.
// SPDX-License-Identifier: GPL-3.0-or-later

package palsav

import (
	"encoding/binary"
	"fmt"
)

const (
	oodleQuantumSize      = 0x40000
	mermaidChunkSize      = 0x20000
	mermaidSubchunkSize   = 0x10000
	oodleDecoderMermaid   = 10
	mermaidInitialHistory = 8
)

type oodleHeader struct {
	decoder   byte
	restart   bool
	raw       bool
	checksums bool
}

func decompressMermaid(src []byte, outputSize int) ([]byte, error) {
	if outputSize < 0 {
		return nil, fmt.Errorf("palsav: negative Mermaid output size")
	}
	dst := make([]byte, outputSize)
	srcAt, dstAt := 0, 0
	var header oodleHeader

	for dstAt < len(dst) {
		if dstAt%oodleQuantumSize == 0 {
			if len(src)-srcAt < 2 {
				return nil, fmt.Errorf("palsav: truncated Oodle block header at %#x", srcAt)
			}
			first, second := src[srcAt], src[srcAt+1]
			srcAt += 2
			if first&0x0f != 0x0c || (first>>4)&3 != 0 {
				return nil, fmt.Errorf("palsav: invalid Oodle block header %#x", first)
			}
			header = oodleHeader{
				decoder:   second & 0x7f,
				restart:   first&0x80 != 0,
				raw:       first&0x40 != 0,
				checksums: second&0x80 != 0,
			}
			if header.decoder != oodleDecoderMermaid {
				return nil, fmt.Errorf("palsav: unsupported Oodle decoder %d (want Mermaid/10)", header.decoder)
			}
		}

		rawCount := min(oodleQuantumSize, len(dst)-dstAt)
		if header.raw {
			if rawCount > len(src)-srcAt {
				return nil, fmt.Errorf("palsav: truncated raw Oodle quantum")
			}
			copy(dst[dstAt:dstAt+rawCount], src[srcAt:srcAt+rawCount])
			srcAt += rawCount
			dstAt += rawCount
			continue
		}
		if len(src)-srcAt < 3 {
			return nil, fmt.Errorf("palsav: truncated Oodle quantum header")
		}
		word := uint32(src[srcAt])<<16 | uint32(src[srcAt+1])<<8 | uint32(src[srcAt+2])
		srcAt += 3
		stored := word & 0x3ffff
		if stored == 0x3ffff {
			special := word >> 18
			if special != 1 || len(src)-srcAt < 1 {
				return nil, fmt.Errorf("palsav: unsupported Oodle special quantum %d", special)
			}
			value := src[srcAt]
			srcAt++
			for i := 0; i < rawCount; i++ {
				dst[dstAt+i] = value
			}
			dstAt += rawCount
			continue
		}
		compressed := int(stored) + 1
		if header.checksums {
			// The open reference decoder does not implement Oodle's proprietary
			// checksum. Reject instead of claiming integrity we cannot verify.
			return nil, fmt.Errorf("palsav: checksummed Oodle quanta are unsupported")
		}
		if compressed > len(src)-srcAt || compressed > rawCount {
			return nil, fmt.Errorf("palsav: invalid Oodle quantum size %d -> %d", compressed, rawCount)
		}
		quantum := src[srcAt : srcAt+compressed]
		if compressed == rawCount {
			copy(dst[dstAt:dstAt+rawCount], quantum)
		} else {
			used, err := decodeMermaidQuantum(dst, dstAt, rawCount, quantum)
			if err != nil {
				return nil, fmt.Errorf("palsav: Mermaid quantum at output %#x: %w", dstAt, err)
			}
			if used != len(quantum) {
				return nil, fmt.Errorf("palsav: Mermaid quantum consumed %d/%d bytes", used, len(quantum))
			}
		}
		srcAt += compressed
		dstAt += rawCount
	}
	if srcAt != len(src) {
		return nil, fmt.Errorf("palsav: Mermaid stream has %d trailing bytes", len(src)-srcAt)
	}
	return dst, nil
}

func decodeMermaidQuantum(dst []byte, dstAt, outputSize int, src []byte) (int, error) {
	start := 0
	for produced := 0; produced < outputSize; {
		chunkSize := min(mermaidChunkSize, outputSize-produced)
		if len(src)-start < 4 {
			return 0, fmt.Errorf("truncated Mermaid chunk")
		}
		word := int(src[start])<<16 | int(src[start+1])<<8 | int(src[start+2])
		if word&0x800000 == 0 {
			result, err := decodeEntropy(src[start:], chunkSize, 0)
			if err != nil {
				return 0, err
			}
			if len(result.data) != chunkSize {
				return 0, fmt.Errorf("entropy-only Mermaid chunk decoded %d/%d bytes", len(result.data), chunkSize)
			}
			copy(dst[dstAt+produced:dstAt+produced+chunkSize], result.data)
			start += result.used
		} else {
			start += 3
			stored := word & 0x7ffff
			mode := (word >> 19) & 0xf
			if stored > len(src)-start {
				return 0, fmt.Errorf("truncated Mermaid LZ chunk")
			}
			if stored > chunkSize || mode > 1 {
				return 0, fmt.Errorf("invalid Mermaid chunk mode/size %d/%d", mode, stored)
			}
			if stored == chunkSize {
				if mode != 0 {
					return 0, fmt.Errorf("verbatim Mermaid chunk has mode %d", mode)
				}
				copy(dst[dstAt+produced:dstAt+produced+chunkSize], src[start:start+stored])
			} else {
				table, err := readMermaidTable(src[start:start+stored], dst, dstAt+produced, chunkSize)
				if err != nil {
					return 0, err
				}
				if err := processMermaidLZ(mode, dst, dstAt+produced, chunkSize, table); err != nil {
					return 0, err
				}
			}
			start += stored
		}
		produced += chunkSize
	}
	return start, nil
}

type mermaidTable struct {
	literals []byte
	commands []byte
	cmdSplit int
	near     []uint16
	far      [2][]uint32
	lengths  []byte

	literalAt int
	nearAt    int
	lengthAt  int
}

func readMermaidTable(src []byte, dst []byte, outputAt, outputSize int) (*mermaidTable, error) {
	if len(src) < 10 {
		return nil, fmt.Errorf("truncated Mermaid LZ table")
	}
	at := 0
	if outputAt == 0 {
		if len(src) < mermaidInitialHistory || len(dst) < mermaidInitialHistory {
			return nil, fmt.Errorf("missing Mermaid initial history")
		}
		copy(dst[:mermaidInitialHistory], src[:mermaidInitialHistory])
		at += mermaidInitialHistory
	}
	table := &mermaidTable{}

	literals, err := decodeEntropy(src[at:], outputSize, 0)
	if err != nil {
		return nil, fmt.Errorf("literal stream: %w", err)
	}
	table.literals = literals.data
	at += literals.used

	commands, err := decodeEntropy(src[at:], outputSize, 0)
	if err != nil {
		return nil, fmt.Errorf("command stream: %w", err)
	}
	table.commands = commands.data
	at += commands.used

	table.cmdSplit = len(table.commands)
	if outputSize > mermaidSubchunkSize {
		if len(src)-at < 2 {
			return nil, fmt.Errorf("truncated Mermaid command split")
		}
		table.cmdSplit = int(binary.LittleEndian.Uint16(src[at:]))
		at += 2
		if table.cmdSplit > len(table.commands) {
			return nil, fmt.Errorf("invalid Mermaid command split %d/%d", table.cmdSplit, len(table.commands))
		}
	}
	if len(src)-at < 2 {
		return nil, fmt.Errorf("truncated Mermaid near-offset count")
	}
	nearCount := int(binary.LittleEndian.Uint16(src[at:]))
	if nearCount == 0xffff {
		at += 2
		high, decodeErr := decodeEntropy(src[at:], outputSize>>1, 0)
		if decodeErr != nil {
			return nil, fmt.Errorf("near-offset high bytes: %w", decodeErr)
		}
		at += high.used
		low, decodeErr := decodeEntropy(src[at:], outputSize>>1, 0)
		if decodeErr != nil {
			return nil, fmt.Errorf("near-offset low bytes: %w", decodeErr)
		}
		at += low.used
		if len(low.data) != len(high.data) {
			return nil, fmt.Errorf("mismatched Mermaid near-offset streams")
		}
		table.near = make([]uint16, len(low.data))
		for i := range table.near {
			table.near[i] = uint16(low.data[i]) | uint16(high.data[i])<<8
		}
	} else {
		at += 2
		bytesNeeded := nearCount * 2
		if bytesNeeded > len(src)-at {
			return nil, fmt.Errorf("truncated Mermaid near offsets")
		}
		table.near = make([]uint16, nearCount)
		for i := range table.near {
			table.near[i] = binary.LittleEndian.Uint16(src[at+i*2:])
		}
		at += bytesNeeded
	}

	if len(src)-at < 3 {
		return nil, fmt.Errorf("truncated Mermaid far-offset counts")
	}
	countWord := int(src[at]) | int(src[at+1])<<8 | int(src[at+2])<<16
	at += 3
	if countWord != 0 {
		counts := [2]int{countWord >> 12, countWord & 0xfff}
		for i := range counts {
			if counts[i] == 0xfff {
				if len(src)-at < 2 {
					return nil, fmt.Errorf("truncated extended Mermaid far-offset count")
				}
				counts[i] = int(binary.LittleEndian.Uint16(src[at:]))
				at += 2
			}
		}
		for stream, count := range counts {
			limit := int64(outputAt)
			if stream == 1 {
				limit += mermaidSubchunkSize
			}
			values, used, decodeErr := decodeMermaidFarOffsets(src[at:], count, limit)
			if decodeErr != nil {
				return nil, decodeErr
			}
			table.far[stream] = values
			at += used
		}
	}
	table.lengths = src[at:]
	return table, nil
}

func decodeMermaidFarOffsets(src []byte, count int, outputOffset int64) ([]uint32, int, error) {
	values := make([]uint32, count)
	at := 0
	for i := range values {
		if len(src)-at < 3 {
			return nil, 0, fmt.Errorf("truncated Mermaid far offsets")
		}
		value := uint32(src[at]) | uint32(src[at+1])<<8 | uint32(src[at+2])<<16
		at += 3
		if outputOffset >= 0xc00000-1 && value >= 0xc00000 {
			if at >= len(src) {
				return nil, 0, fmt.Errorf("truncated extended Mermaid far offset")
			}
			value += uint32(src[at]) << 22
			at++
		}
		if int64(value) > outputOffset {
			return nil, 0, fmt.Errorf("Mermaid far offset %d exceeds history %d", value, outputOffset)
		}
		values[i] = value
	}
	return values, at, nil
}

func processMermaidLZ(mode int, dst []byte, outputAt, outputSize int, table *mermaidTable) error {
	if mode != 1 {
		return fmt.Errorf("palsav: Mermaid mode %d is not present in the supported saves", mode)
	}
	savedDistance := -mermaidInitialHistory
	for iteration, produced := 0, 0; produced < outputSize; iteration++ {
		size := min(mermaidSubchunkSize, outputSize-produced)
		commandStart, commandEnd := 0, table.cmdSplit
		if iteration != 0 {
			commandStart, commandEnd = table.cmdSplit, len(table.commands)
		}
		if iteration > 1 {
			return fmt.Errorf("Mermaid chunk has more than two subchunks")
		}
		if err := processMermaidMode1(
			dst,
			outputAt+produced,
			size,
			table.commands[commandStart:commandEnd],
			table.far[iteration],
			table,
			&savedDistance,
			outputAt == 0 && iteration == 0,
		); err != nil {
			return err
		}
		produced += size
	}
	if table.nearAt != len(table.near) {
		return fmt.Errorf("Mermaid near-offset stream consumed %d/%d values", table.nearAt, len(table.near))
	}
	if table.lengthAt != len(table.lengths) {
		return fmt.Errorf("Mermaid length stream consumed %d/%d bytes", table.lengthAt, len(table.lengths))
	}
	if table.literalAt != len(table.literals) {
		return fmt.Errorf("Mermaid literal stream consumed %d/%d bytes", table.literalAt, len(table.literals))
	}
	return nil
}

func processMermaidMode1(
	dst []byte,
	subchunkAt, subchunkSize int,
	commands []byte,
	far []uint32,
	table *mermaidTable,
	savedDistance *int,
	skipInitial bool,
) error {
	cursor := subchunkAt
	end := subchunkAt + subchunkSize
	if skipInitial {
		cursor += mermaidInitialHistory
	}
	farAt := 0
	copyLiterals := func(count int) error {
		if count < 0 || count > end-cursor || count > len(table.literals)-table.literalAt {
			return fmt.Errorf("invalid Mermaid literal length %d", count)
		}
		copy(dst[cursor:cursor+count], table.literals[table.literalAt:table.literalAt+count])
		cursor += count
		table.literalAt += count
		return nil
	}
	copyMatch := func(matchAt, count int) error {
		if count < 0 || count > end-cursor || matchAt < 0 || matchAt >= cursor {
			return fmt.Errorf("invalid Mermaid match at %d, output %d, length %d", matchAt, cursor, count)
		}
		for i := 0; i < count; i++ {
			source := matchAt + i
			if source < 0 || source >= cursor+i {
				return fmt.Errorf("invalid overlapping Mermaid match")
			}
			dst[cursor+i] = dst[source]
		}
		cursor += count
		return nil
	}
	readLength := func(base int) (int, error) {
		if table.lengthAt >= len(table.lengths) {
			return 0, fmt.Errorf("truncated Mermaid length stream")
		}
		value := int(table.lengths[table.lengthAt])
		table.lengthAt++
		if value > 251 {
			if len(table.lengths)-table.lengthAt < 2 {
				return 0, fmt.Errorf("truncated extended Mermaid length")
			}
			value += int(binary.LittleEndian.Uint16(table.lengths[table.lengthAt:])) * 4
			table.lengthAt += 2
		}
		return value + base, nil
	}
	readFar := func() (int, error) {
		if farAt >= len(far) {
			return 0, fmt.Errorf("truncated Mermaid far-offset stream")
		}
		matchAt := subchunkAt - int(far[farAt])
		farAt++
		return matchAt, nil
	}

	for _, commandByte := range commands {
		command := int(commandByte)
		switch {
		case command >= 24:
			literalCount := command & 7
			if err := copyLiterals(literalCount); err != nil {
				return err
			}
			if command < 128 {
				if table.nearAt >= len(table.near) {
					return fmt.Errorf("truncated Mermaid near-offset stream")
				}
				*savedDistance = -int(table.near[table.nearAt])
				table.nearAt++
			}
			matchCount := (command >> 3) & 0xf
			if err := copyMatch(cursor+*savedDistance, matchCount); err != nil {
				return err
			}
		case command > 2:
			count := command + 5
			matchAt, err := readFar()
			if err != nil {
				return err
			}
			*savedDistance = matchAt - cursor
			if err := copyMatch(matchAt, count); err != nil {
				return err
			}
		case command == 0:
			count, err := readLength(64)
			if err != nil {
				return err
			}
			if err := copyLiterals(count); err != nil {
				return err
			}
		case command == 1:
			count, err := readLength(91)
			if err != nil {
				return err
			}
			if table.nearAt >= len(table.near) {
				return fmt.Errorf("truncated Mermaid near-offset stream")
			}
			*savedDistance = -int(table.near[table.nearAt])
			table.nearAt++
			if err := copyMatch(cursor+*savedDistance, count); err != nil {
				return err
			}
		case command == 2:
			count, err := readLength(29)
			if err != nil {
				return err
			}
			matchAt, err := readFar()
			if err != nil {
				return err
			}
			*savedDistance = matchAt - cursor
			if err := copyMatch(matchAt, count); err != nil {
				return err
			}
		}
	}
	if err := copyLiterals(end - cursor); err != nil {
		return err
	}
	if farAt != len(far) {
		return fmt.Errorf("Mermaid far-offset stream consumed %d/%d values", farAt, len(far))
	}
	return nil
}
