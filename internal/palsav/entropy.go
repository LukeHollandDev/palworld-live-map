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
	"math/bits"
)

const maxEntropyDepth = 16

type entropyResult struct {
	data []byte
	used int
}

func decodeEntropy(src []byte, capacity, depth int) (entropyResult, error) {
	if depth > maxEntropyDepth {
		return entropyResult{}, fmt.Errorf("palsav: entropy nesting exceeds %d", maxEntropyDepth)
	}
	if len(src) < 2 {
		return entropyResult{}, fmt.Errorf("palsav: truncated entropy header")
	}
	chunkType := int(src[0]>>4) & 7
	if chunkType == 0 {
		var size, header int
		if src[0] >= 0x80 {
			size = int(binary.BigEndian.Uint16(src[:2]) & 0x0fff)
			header = 2
		} else {
			if len(src) < 3 {
				return entropyResult{}, fmt.Errorf("palsav: truncated long raw entropy header")
			}
			size = int(src[0])<<16 | int(src[1])<<8 | int(src[2])
			if size&^0x3ffff != 0 {
				return entropyResult{}, fmt.Errorf("palsav: reserved raw entropy size bits are set")
			}
			header = 3
		}
		if size > capacity || size > len(src)-header {
			return entropyResult{}, fmt.Errorf("palsav: raw entropy size %d exceeds its bounds", size)
		}
		out := append([]byte(nil), src[header:header+size]...)
		return entropyResult{data: out, used: header + size}, nil
	}
	if chunkType >= 6 {
		return entropyResult{}, fmt.Errorf("palsav: unsupported entropy type %d", chunkType)
	}

	var compressed, decoded, header int
	if src[0] >= 0x80 {
		if len(src) < 3 {
			return entropyResult{}, fmt.Errorf("palsav: truncated short entropy header")
		}
		word := int(src[0])<<16 | int(src[1])<<8 | int(src[2])
		compressed = word & 0x3ff
		decoded = compressed + ((word >> 10) & 0x3ff) + 1
		header = 3
	} else {
		if len(src) < 5 {
			return entropyResult{}, fmt.Errorf("palsav: truncated long entropy header")
		}
		word := uint32(src[1])<<24 | uint32(src[2])<<16 | uint32(src[3])<<8 | uint32(src[4])
		compressed = int(word & 0x3ffff)
		decoded = int(((word>>18)|uint32(src[0])<<14)&0x3ffff) + 1
		if compressed >= decoded {
			return entropyResult{}, fmt.Errorf("palsav: invalid entropy sizes %d >= %d", compressed, decoded)
		}
		header = 5
	}
	if compressed > len(src)-header || decoded > capacity {
		return entropyResult{}, fmt.Errorf("palsav: entropy sizes %d -> %d exceed their bounds", compressed, decoded)
	}
	payload := src[header : header+compressed]
	var out []byte
	var err error
	switch chunkType {
	case 2, 4:
		out, err = decodeHuffman(payload, decoded, chunkType>>1)
	case 3:
		out, err = decodeRLE(payload, decoded, depth)
	case 1:
		err = fmt.Errorf("palsav: entropy type 1 (tANS) is not present in the supported Palworld saves")
	case 5:
		err = fmt.Errorf("palsav: entropy type 5 is not present in the supported Palworld saves")
	}
	if err != nil {
		return entropyResult{}, err
	}
	if len(out) != decoded {
		return entropyResult{}, fmt.Errorf("palsav: entropy decoded %d bytes, expected %d", len(out), decoded)
	}
	return entropyResult{data: out, used: header + compressed}, nil
}

func decodeRLE(src []byte, outputSize, depth int) ([]byte, error) {
	if len(src) == 0 {
		return nil, fmt.Errorf("palsav: empty RLE stream")
	}
	if len(src) == 1 {
		out := make([]byte, outputSize)
		for i := range out {
			out[i] = src[0]
		}
		return out, nil
	}

	commands := src[1:]
	if src[0] != 0 {
		prefix, err := decodeEntropy(src, outputSize, depth+1)
		if err != nil {
			return nil, fmt.Errorf("palsav: RLE command prefix: %w", err)
		}
		commands = make([]byte, 0, len(prefix.data)+len(src)-prefix.used)
		commands = append(commands, prefix.data...)
		commands = append(commands, src[prefix.used:]...)
	}

	out := make([]byte, outputSize)
	outAt, front, back := 0, 0, len(commands)
	rleByte := byte(0)
	copyLiteral := func(count int) error {
		if count < 0 || count > back-front || count > len(out)-outAt {
			return fmt.Errorf("palsav: invalid RLE literal length %d", count)
		}
		copy(out[outAt:outAt+count], commands[front:front+count])
		front += count
		outAt += count
		return nil
	}
	fill := func(count int) error {
		if count < 0 || count > len(out)-outAt {
			return fmt.Errorf("palsav: invalid RLE run length %d", count)
		}
		for i := 0; i < count; i++ {
			out[outAt+i] = rleByte
		}
		outAt += count
		return nil
	}

	for front < back {
		command := uint32(commands[back-1])
		switch {
		case command == 0 || command >= 0x30:
			back--
			literals := int((^command) & 0xf)
			run := int(command >> 4)
			if err := copyLiteral(literals); err != nil {
				return nil, err
			}
			if err := fill(run); err != nil {
				return nil, err
			}
		case command >= 0x10:
			if back-front < 2 {
				return nil, fmt.Errorf("palsav: truncated RLE command")
			}
			value := uint32(binary.LittleEndian.Uint16(commands[back-2:])) - 4096
			back -= 2
			if err := copyLiteral(int(value & 0x3f)); err != nil {
				return nil, err
			}
			if err := fill(int(value >> 6)); err != nil {
				return nil, err
			}
		case command == 1:
			if back-front < 2 {
				return nil, fmt.Errorf("palsav: truncated RLE byte command")
			}
			rleByte = commands[front]
			front++
			back--
		case command >= 9:
			if back-front < 2 {
				return nil, fmt.Errorf("palsav: truncated long RLE command")
			}
			count := (int(binary.LittleEndian.Uint16(commands[back-2:])) - 0x8ff) * 128
			back -= 2
			if err := fill(count); err != nil {
				return nil, err
			}
		default:
			if back-front < 2 {
				return nil, fmt.Errorf("palsav: truncated long literal command")
			}
			count := (int(binary.LittleEndian.Uint16(commands[back-2:])) - 511) * 64
			back -= 2
			if err := copyLiteral(count); err != nil {
				return nil, err
			}
		}
	}
	if front != back || outAt != len(out) {
		return nil, fmt.Errorf("palsav: RLE ended at command %d/%d and output %d/%d", front, back, outAt, len(out))
	}
	return out, nil
}

type msbReader struct {
	data []byte
	bit  int
}

func (r *msbReader) read(n int) (uint32, error) {
	if n < 0 || n > 24 || r.bit+n > len(r.data)*8 {
		return 0, fmt.Errorf("palsav: truncated Huffman bitstream")
	}
	var value uint32
	for i := 0; i < n; i++ {
		at := r.bit + i
		value = value<<1 | uint32((r.data[at>>3]>>(7-(at&7)))&1)
	}
	r.bit += n
	return value, nil
}

func (r *msbReader) peek(n int) (uint32, error) {
	at := r.bit
	value, err := r.read(n)
	r.bit = at
	return value, err
}

func (r *msbReader) byteOffset() int { return (r.bit + 7) >> 3 }

type huffRange struct {
	symbol int
	count  int
}

var huffPrefixOriginal = [...]int{0, 0, 2, 6, 14, 30, 62, 126, 254, 510, 766, 1022}

func readHuffmanCodebook(r *msbReader) ([2048]huffEntry, int, error) {
	var lut [2048]huffEntry
	first, err := r.read(1)
	if err != nil {
		return lut, 0, err
	}
	if first == 0 {
		return lut, 0, fmt.Errorf("palsav: legacy Huffman code lengths are not used by the supported saves")
	}
	second, err := r.read(1)
	if err != nil {
		return lut, 0, err
	}
	if second != 0 {
		return lut, 0, fmt.Errorf("palsav: reserved Huffman codebook form")
	}

	forcedValue, err := r.read(2)
	if err != nil {
		return lut, 0, err
	}
	forced := int(forcedValue)
	nValue, err := r.read(8)
	if err != nil {
		return lut, 0, err
	}
	numSymbols := int(nValue) + 1
	fluff, err := readFluff(r, numSymbols)
	if err != nil {
		return lut, 0, err
	}

	codeLength := make([]int, numSymbols+fluff)
	for i := range codeLength {
		zeros := 0
		for {
			bit, readErr := r.read(1)
			if readErr != nil {
				return lut, 0, readErr
			}
			if bit != 0 {
				break
			}
			zeros++
			if zeros > 255 {
				return lut, 0, fmt.Errorf("palsav: invalid Golomb-Rice length")
			}
		}
		codeLength[i] = zeros
	}
	for i := 0; i < numSymbols && forced != 0; i++ {
		extra, readErr := r.read(forced)
		if readErr != nil {
			return lut, 0, readErr
		}
		codeLength[i] = codeLength[i]<<forced | int(extra)
	}

	runningSum := 0x1e
	for i := 0; i < numSymbols; i++ {
		value := codeLength[i]
		delta := -(value & 1) ^ (value >> 1)
		codeLength[i] = delta + (runningSum >> 2) + 1
		if codeLength[i] < 1 || codeLength[i] > 11 {
			return lut, 0, fmt.Errorf("palsav: invalid Huffman code length %d", codeLength[i])
		}
		runningSum += delta
	}

	ranges, err := convertHuffmanRanges(r, numSymbols, fluff, codeLength[numSymbols:])
	if err != nil {
		return lut, 0, err
	}
	if numSymbols == 1 {
		if len(ranges) != 1 || ranges[0].count != 1 {
			return lut, 0, fmt.Errorf("palsav: invalid single-symbol Huffman ranges")
		}
		entry := huffEntry{symbol: byte(ranges[0].symbol), length: 1}
		for i := range lut {
			lut[i] = entry
		}
		return lut, numSymbols, nil
	}
	prefix := huffPrefixOriginal
	symbols := make([]byte, 1280)
	codeAt := 0
	for _, item := range ranges {
		symbol := item.symbol
		for i := 0; i < item.count; i++ {
			length := codeLength[codeAt]
			codeAt++
			at := prefix[length]
			if at < 0 || at >= len(symbols) {
				return lut, 0, fmt.Errorf("palsav: Huffman symbol table overflow")
			}
			symbols[at] = byte(symbol)
			prefix[length]++
			symbol++
		}
	}
	if codeAt != numSymbols {
		return lut, 0, fmt.Errorf("palsav: incomplete Huffman ranges")
	}
	if err := makeHuffmanLUT(&lut, prefix, symbols); err != nil {
		return lut, 0, err
	}
	return lut, numSymbols, nil
}

func readFluff(r *msbReader, numSymbols int) (int, error) {
	if numSymbols == 256 {
		return 0, nil
	}
	x := 257 - numSymbols
	if x > numSymbols {
		x = numSymbols
	}
	x *= 2
	width := bits.Len(uint(x - 1))
	value, err := r.peek(width)
	if err != nil {
		return 0, err
	}
	threshold := (1 << width) - x
	if int(value>>1) >= threshold {
		value, err = r.read(width)
		if err != nil {
			return 0, err
		}
		return int(value) - threshold, nil
	}
	value, err = r.read(width - 1)
	return int(value), err
}

func convertHuffmanRanges(r *msbReader, numSymbols, fluff int, encoded []int) ([]huffRange, error) {
	rangeCount := fluff >> 1
	encodedAt := 0
	symbolAt := 0
	if fluff&1 != 0 {
		if encodedAt >= len(encoded) || encoded[encodedAt] >= 8 {
			return nil, fmt.Errorf("palsav: invalid initial Huffman range")
		}
		width := encoded[encodedAt] + 1
		encodedAt++
		value, err := r.read(width)
		if err != nil {
			return nil, err
		}
		symbolAt = int(value) + (1 << width) - 1
	}
	ranges := make([]huffRange, 0, rangeCount+1)
	used := 0
	for i := 0; i < rangeCount; i++ {
		if encodedAt+1 >= len(encoded) || encoded[encodedAt] >= 9 || encoded[encodedAt+1] >= 8 {
			return nil, fmt.Errorf("palsav: invalid Huffman range")
		}
		width := encoded[encodedAt]
		encodedAt++
		value, err := r.read(width)
		if err != nil {
			return nil, err
		}
		count := int(value) + (1 << width)
		width = encoded[encodedAt] + 1
		encodedAt++
		value, err = r.read(width)
		if err != nil {
			return nil, err
		}
		space := int(value) + (1 << width) - 1
		ranges = append(ranges, huffRange{symbol: symbolAt, count: count})
		used += count
		symbolAt += count + space
	}
	if symbolAt >= 256 || used >= numSymbols || symbolAt+numSymbols-used > 256 {
		return nil, fmt.Errorf("palsav: invalid Huffman symbol ranges")
	}
	ranges = append(ranges, huffRange{symbol: symbolAt, count: numSymbols - used})
	return ranges, nil
}

type huffEntry struct {
	symbol byte
	length uint8
}

func makeHuffmanLUT(lut *[2048]huffEntry, prefix [12]int, symbols []byte) error {
	slot := 0
	for length := 1; length <= 11; length++ {
		start := huffPrefixOriginal[length]
		count := prefix[length] - start
		step := 1 << (11 - length)
		for i := 0; i < count; i++ {
			if start+i >= len(symbols) || slot+step > len(lut) {
				return fmt.Errorf("palsav: invalid Huffman lookup table")
			}
			entry := huffEntry{symbol: symbols[start+i], length: uint8(length)}
			for j := 0; j < step; j++ {
				reversed := bits.Reverse16(uint16(slot+j)) >> 5
				lut[reversed] = entry
			}
			slot += step
		}
	}
	if slot != len(lut) {
		return fmt.Errorf("palsav: incomplete Huffman lookup table (%d/2048)", slot)
	}
	return nil
}

type lsbReader struct {
	data     []byte
	bit      int
	backward bool
}

func (r *lsbReader) lookup(lut *[2048]huffEntry) (byte, error) {
	var index uint16
	for i := 0; i < 11; i++ {
		at := r.bit + i
		byteAt := at >> 3
		if byteAt >= len(r.data) {
			continue // ooz permits zero padding at the end of a stream.
		}
		if r.backward {
			byteAt = len(r.data) - 1 - byteAt
		}
		index |= uint16((r.data[byteAt]>>(at&7))&1) << i
	}
	entry := lut[index]
	if entry.length == 0 {
		return 0, fmt.Errorf("palsav: invalid Huffman code")
	}
	r.bit += int(entry.length)
	if r.bit > len(r.data)*8+10 {
		return 0, fmt.Errorf("palsav: Huffman stream overrun")
	}
	return entry.symbol, nil
}

func (r *lsbReader) bytesUsed() int { return (r.bit + 7) >> 3 }

func decodeHuffman(src []byte, outputSize, variant int) ([]byte, error) {
	reader := &msbReader{data: src}
	lut, symbols, err := readHuffmanCodebook(reader)
	if err != nil {
		return nil, err
	}
	if symbols == 1 {
		if reader.byteOffset() != len(src) {
			return nil, fmt.Errorf(
				"palsav: single-symbol Huffman codebook consumed %d/%d bytes",
				reader.byteOffset(),
				len(src),
			)
		}
		only := lut[0].symbol
		out := make([]byte, outputSize)
		for i := range out {
			out[i] = only
		}
		return out, nil
	}
	at := reader.byteOffset()
	if at > len(src) {
		return nil, fmt.Errorf("palsav: Huffman codebook overrun")
	}
	src = src[at:]
	out := make([]byte, outputSize)

	switch variant {
	case 1:
		if len(src) < 3 {
			return nil, fmt.Errorf("palsav: truncated Huffman split")
		}
		split := int(binary.LittleEndian.Uint16(src))
		src = src[2:]
		if split > len(src) {
			return nil, fmt.Errorf("palsav: invalid Huffman split %d", split)
		}
		if err := decodeHuffmanStreams(out, src[:split], src[split:], &lut); err != nil {
			return nil, err
		}
	case 2:
		if len(src) < 6 {
			return nil, fmt.Errorf("palsav: truncated six-stream Huffman split")
		}
		middle := int(src[0]) | int(src[1])<<8 | int(src[2])<<16
		src = src[3:]
		if middle > len(src) || middle < 2 {
			return nil, fmt.Errorf("palsav: invalid Huffman middle split %d", middle)
		}
		rightHalf := src[middle:]
		leftHalf := src[:middle]
		leftSplit := int(binary.LittleEndian.Uint16(leftHalf))
		leftHalf = leftHalf[2:]
		if leftSplit > len(leftHalf) || len(rightHalf) < 2 {
			return nil, fmt.Errorf("palsav: invalid Huffman left split")
		}
		rightSplit := int(binary.LittleEndian.Uint16(rightHalf))
		rightHalf = rightHalf[2:]
		if rightSplit > len(rightHalf) {
			return nil, fmt.Errorf("palsav: invalid Huffman right split")
		}
		half := (len(out) + 1) >> 1
		if err := decodeHuffmanStreams(out[:half], leftHalf[:leftSplit], leftHalf[leftSplit:], &lut); err != nil {
			return nil, err
		}
		if err := decodeHuffmanStreams(out[half:], rightHalf[:rightSplit], rightHalf[rightSplit:], &lut); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("palsav: unsupported Huffman variant %d", variant)
	}
	return out, nil
}

func decodeHuffmanStreams(dst, first, shared []byte, lut *[2048]huffEntry) error {
	a := lsbReader{data: first}
	b := lsbReader{data: shared, backward: true}
	c := lsbReader{data: shared}
	streams := [...]*lsbReader{&a, &b, &c}
	for i := range dst {
		value, err := streams[i%3].lookup(lut)
		if err != nil {
			return err
		}
		dst[i] = value
	}
	if a.bytesUsed() != len(first) {
		return fmt.Errorf("palsav: Huffman first stream consumed %d/%d bytes", a.bytesUsed(), len(first))
	}
	if b.bytesUsed()+c.bytesUsed() != len(shared) {
		return fmt.Errorf("palsav: Huffman shared streams consumed %d+%d/%d bytes", b.bytesUsed(), c.bytesUsed(), len(shared))
	}
	return nil
}
