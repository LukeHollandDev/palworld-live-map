// Copyright (C) 2026 Luke Holland
// SPDX-License-Identifier: GPL-3.0-or-later

package palsav

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"testing"
)

func TestZlibContainers(t *testing.T) {
	raw := []byte("GVAS synthetic payload")
	inner := testZlib(t, raw)
	outer := testZlib(t, inner)

	tests := []struct {
		name           string
		saveType       byte
		compressedSize uint32
		body           []byte
		prefix         []byte
	}{
		{
			name:           "single",
			saveType:       0x31,
			compressedSize: uint32(len(inner)),
			body:           inner,
		},
		{
			name:           "double",
			saveType:       0x32,
			compressedSize: uint32(len(inner)),
			body:           outer,
		},
		{
			name:           "CNK wrapped",
			saveType:       0x31,
			compressedSize: uint32(len(inner)),
			body:           inner,
			prefix:         []byte{0, 0, 0, 0, 0, 0, 0, 0, 'C', 'N', 'K', 0},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			container := testContainer(
				raw,
				"PlZ",
				test.saveType,
				test.compressedSize,
				test.body,
				test.prefix,
			)
			decoded, header, err := DecodeContainer(container)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(decoded, raw) {
				t.Fatalf("decoded = %q, want %q", decoded, raw)
			}
			if header.Offset != len(test.prefix) {
				t.Fatalf("header offset = %d, want %d", header.Offset, len(test.prefix))
			}
		})
	}
}

func TestZlibContainerRejectsTrailingBytes(t *testing.T) {
	raw := []byte("GVAS synthetic payload")
	body := append(testZlib(t, raw), 0xff)
	container := testContainer(raw, "PlZ", 0x31, uint32(len(body)), body, nil)
	if _, _, err := DecodeContainer(container); err == nil {
		t.Fatal("DecodeContainer accepted bytes after the zlib stream")
	}
}

func TestContainerInputLimitError(t *testing.T) {
	_, _, err := DecodeContainerWithLimits(make([]byte, 13), Limits{
		MaxInputBytes:  12,
		MaxOutputBytes: 12,
	})
	var limitErr *LimitError
	if !errors.As(err, &limitErr) || limitErr.Kind != "container input bytes" {
		t.Fatalf("error = %v, want container-input LimitError", err)
	}
}

func TestMermaidVerbatimQuantum(t *testing.T) {
	raw := []byte("GVAS synthetic Mermaid payload")
	stored := len(raw) - 1
	body := []byte{
		0x8c, 0x0a,
		byte(stored >> 16), byte(stored >> 8), byte(stored),
	}
	body = append(body, raw...)
	container := testContainer(raw, "PlM", 0x31, uint32(len(body)), body, nil)
	decoded, _, err := DecodeContainer(container)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, raw) {
		t.Fatalf("decoded = %q, want %q", decoded, raw)
	}
}

func TestMermaidMode1NearMatches(t *testing.T) {
	history := []byte("ABCDEFGH")
	commands := []byte{120, 248, 248, 248, 248, 248, 248, 248}
	table := append([]byte(nil), history...)
	table = append(table, 0x80, 0x00) // empty raw literal stream
	table = append(table, 0x80, byte(len(commands)))
	table = append(table, commands...)
	table = append(table, 1, 0, 8, 0) // one near offset, distance eight
	table = append(table, 0, 0, 0)    // no far offsets

	chunkWord := 0x800000 | 1<<19 | len(table)
	quantum := []byte{
		byte(chunkWord >> 16), byte(chunkWord >> 8), byte(chunkWord),
	}
	quantum = append(quantum, table...)
	stored := len(quantum) - 1
	body := []byte{
		0x8c, 0x0a,
		byte(stored >> 16), byte(stored >> 8), byte(stored),
	}
	body = append(body, quantum...)

	decoded, err := decompressMermaid(body, 128)
	if err != nil {
		t.Fatal(err)
	}
	for index, value := range decoded {
		if want := history[index%len(history)]; value != want {
			t.Fatalf("decoded[%d] = %q, want %q", index, value, want)
		}
	}
}

func TestMermaidMemsetAndMultipleRawQuanta(t *testing.T) {
	memset, err := decompressMermaid([]byte{
		0x8c, 0x0a,
		0x07, 0xff, 0xff,
		0x5a,
	}, 64)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(memset, bytes.Repeat([]byte{0x5a}, 64)) {
		t.Fatalf("memset output = %x", memset)
	}

	raw := make([]byte, oodleQuantumSize+17)
	copy(raw, "GVAS")
	for index := 4; index < len(raw); index++ {
		raw[index] = byte(index)
	}
	body := []byte{0xcc, 0x0a}
	body = append(body, raw[:oodleQuantumSize]...)
	body = append(body, 0x4c, 0x0a)
	body = append(body, raw[oodleQuantumSize:]...)
	decoded, err := decompressMermaid(body, len(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, raw) {
		t.Fatal("multiple raw Mermaid quanta did not round-trip")
	}
}

func TestRLEEntropyFill(t *testing.T) {
	// Short entropy header: type 3, compressed=1, decoded=64.
	result, err := decodeEntropy([]byte{0xb0, 0xf8, 0x01, 0x5a}, 64, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.used != 4 || !bytes.Equal(result.data, bytes.Repeat([]byte{0x5a}, 64)) {
		t.Fatalf("RLE result = used:%d data:%x", result.used, result.data)
	}
}

func TestNewHuffmanEntropy(t *testing.T) {
	var codebook testMSBBits
	codebook.write(1, 1) // new code lengths
	codebook.write(0, 1) // supported codebook form
	codebook.write(0, 2) // no forced low bits
	codebook.write(1, 8) // two symbols
	codebook.write(1, 2) // one range-description value
	codebook.unary(13)   // first canonical code length is one
	codebook.unary(9)    // second canonical code length is one
	codebook.unary(5)    // six-bit initial symbol offset
	codebook.write(2, 6) // first symbol is ASCII A

	payload := codebook.bytes()
	payload = append(payload,
		1, 0, // one byte in the first bitstream
		2,    // first stream: A, B
		2, 1, // forward stream A, B; backward stream B, A
	)
	decoded, err := decodeHuffman(payload, 6, 1)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != "ABABAB" {
		t.Fatalf("Huffman output = %q", decoded)
	}
}

type testMSBBits struct {
	bits []byte
}

func (writer *testMSBBits) write(value, count int) {
	for bit := count - 1; bit >= 0; bit-- {
		writer.bits = append(writer.bits, byte(value>>bit)&1)
	}
}

func (writer *testMSBBits) unary(zeros int) {
	writer.bits = append(writer.bits, make([]byte, zeros)...)
	writer.bits = append(writer.bits, 1)
}

func (writer *testMSBBits) bytes() []byte {
	value := make([]byte, (len(writer.bits)+7)/8)
	for index, bit := range writer.bits {
		value[index/8] |= bit << (7 - (index % 8))
	}
	return value
}

func testZlib(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zlib.NewWriter(&buffer)
	if _, err := writer.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func testContainer(
	raw []byte,
	magic string,
	saveType byte,
	compressedSize uint32,
	body []byte,
	prefix []byte,
) []byte {
	data := append([]byte(nil), prefix...)
	header := make([]byte, 12)
	binary.LittleEndian.PutUint32(header, uint32(len(raw)))
	binary.LittleEndian.PutUint32(header[4:], compressedSize)
	copy(header[8:11], magic)
	header[11] = saveType
	data = append(data, header...)
	data = append(data, body...)
	return data
}
