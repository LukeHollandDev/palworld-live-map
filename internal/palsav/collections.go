// Copyright (C) 2026 Luke Holland
// SPDX-License-Identifier: GPL-3.0-or-later

package palsav

import "fmt"

// MapIterator decodes one ordered map entry at a time.
type MapIterator struct {
	value   *MapValue
	reader  *archiveReader
	index   uint32
	current MapEntry
	err     error
	valid   bool
}

// Iterator returns a new independent iterator over the map.
func (value *MapValue) Iterator() *MapIterator {
	if value == nil {
		return &MapIterator{err: fmt.Errorf("palsav: nil MapValue")}
	}
	return &MapIterator{
		value:  value,
		reader: newArchiveReaderAt(value.raw, value.base, value.cfg),
	}
}

// Next advances to the next map entry.
func (iterator *MapIterator) Next() bool {
	iterator.valid = false
	if iterator.err != nil || iterator.reader == nil {
		return false
	}
	if iterator.index == iterator.value.Count {
		if iterator.reader.remaining() != 0 {
			iterator.err = fmt.Errorf(
				"palsav: map %s has %d trailing bytes",
				displayPath(iterator.value.path),
				iterator.reader.remaining(),
			)
		}
		return false
	}

	key, err := readBareValue(
		iterator.reader,
		iterator.value.KeyType,
		iterator.value.KeyStructType,
		iterator.value.keyPath,
	)
	if err != nil {
		iterator.err = fmt.Errorf(
			"palsav: map %s entry %d key: %w",
			displayPath(iterator.value.path),
			iterator.index,
			err,
		)
		return false
	}
	value, err := readBareValue(
		iterator.reader,
		iterator.value.ValueType,
		iterator.value.ValueStructType,
		iterator.value.valuePath,
	)
	if err != nil {
		iterator.err = fmt.Errorf(
			"palsav: map %s entry %d value: %w",
			displayPath(iterator.value.path),
			iterator.index,
			err,
		)
		return false
	}
	iterator.current = MapEntry{Key: key, Value: value}
	iterator.index++
	iterator.valid = true
	return true
}

// Entry returns the entry produced by the most recent successful Next call.
func (iterator *MapIterator) Entry() MapEntry {
	if !iterator.valid {
		return MapEntry{}
	}
	return iterator.current
}

// Err reports the first iteration error.
func (iterator *MapIterator) Err() error { return iterator.err }

// Entries eagerly decodes every map entry while preserving serialized order.
// Prefer Iterator for large Level.sav maps.
func (value *MapValue) Entries() ([]MapEntry, error) {
	if value == nil {
		return nil, fmt.Errorf("palsav: nil MapValue")
	}
	entries := make([]MapEntry, 0, value.Count)
	iterator := value.Iterator()
	for iterator.Next() {
		entries = append(entries, iterator.Entry())
	}
	if err := iterator.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// SetIterator decodes one ordered set element at a time.
type SetIterator struct {
	value   *SetValue
	reader  *archiveReader
	index   uint32
	current any
	err     error
	valid   bool
}

// Iterator returns a new independent iterator over the set.
func (value *SetValue) Iterator() *SetIterator {
	if value == nil {
		return &SetIterator{err: fmt.Errorf("palsav: nil SetValue")}
	}
	return &SetIterator{
		value:  value,
		reader: newArchiveReaderAt(value.raw, value.base, value.cfg),
	}
}

// Next advances to the next set element.
func (iterator *SetIterator) Next() bool {
	iterator.valid = false
	if iterator.err != nil || iterator.reader == nil {
		return false
	}
	if iterator.index == iterator.value.Count {
		if iterator.reader.remaining() != 0 {
			iterator.err = fmt.Errorf(
				"palsav: set %s has %d trailing bytes",
				displayPath(iterator.value.path),
				iterator.reader.remaining(),
			)
		}
		return false
	}
	value, err := readBareValue(
		iterator.reader,
		iterator.value.ElementType,
		iterator.value.ElementStructType,
		iterator.value.path,
	)
	if err != nil {
		iterator.err = fmt.Errorf(
			"palsav: set %s element %d: %w",
			displayPath(iterator.value.path),
			iterator.index,
			err,
		)
		return false
	}
	iterator.current = value
	iterator.index++
	iterator.valid = true
	return true
}

// Value returns the element produced by the most recent successful Next call.
func (iterator *SetIterator) Value() any {
	if !iterator.valid {
		return nil
	}
	return iterator.current
}

// Err reports the first iteration error.
func (iterator *SetIterator) Err() error { return iterator.err }

// Values eagerly decodes every set element in serialized order.
func (value *SetValue) Values() ([]any, error) {
	if value == nil {
		return nil, fmt.Errorf("palsav: nil SetValue")
	}
	values := make([]any, 0, value.Count)
	iterator := value.Iterator()
	for iterator.Next() {
		values = append(values, iterator.Value())
	}
	if err := iterator.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

// StructIterator decodes one ArrayProperty<StructProperty> value at a time.
type StructIterator struct {
	array   *StructArray
	reader  *archiveReader
	index   uint32
	current any
	err     error
	valid   bool
}

// Iterator returns a new independent iterator over the struct array.
func (array *StructArray) Iterator() *StructIterator {
	if array == nil {
		return &StructIterator{err: fmt.Errorf("palsav: nil StructArray")}
	}
	return &StructIterator{
		array:  array,
		reader: newArchiveReaderAt(array.raw, array.base, array.cfg),
	}
}

// Next advances to the next struct value.
func (iterator *StructIterator) Next() bool {
	iterator.valid = false
	if iterator.err != nil || iterator.reader == nil {
		return false
	}
	if iterator.index == iterator.array.Count {
		if iterator.reader.remaining() != 0 {
			iterator.err = fmt.Errorf(
				"palsav: struct array %s has %d trailing bytes",
				displayPath(iterator.array.path),
				iterator.reader.remaining(),
			)
		}
		return false
	}
	value, err := readStructBody(iterator.reader, iterator.array.StructType, iterator.array.path)
	if err != nil {
		iterator.err = fmt.Errorf(
			"palsav: struct array %s element %d: %w",
			displayPath(iterator.array.path),
			iterator.index,
			err,
		)
		return false
	}
	iterator.current = value
	iterator.index++
	iterator.valid = true
	return true
}

// Value returns the value produced by the most recent successful Next call.
func (iterator *StructIterator) Value() any {
	if !iterator.valid {
		return nil
	}
	return iterator.current
}

// Err reports the first iteration error.
func (iterator *StructIterator) Err() error { return iterator.err }

// Values eagerly decodes every struct in the array. Prefer Iterator for large
// arrays such as DPS SaveParameterArray.
func (array *StructArray) Values() ([]any, error) {
	if array == nil {
		return nil, fmt.Errorf("palsav: nil StructArray")
	}
	values := make([]any, 0, array.Count)
	iterator := array.Iterator()
	for iterator.Next() {
		values = append(values, iterator.Value())
	}
	if err := iterator.Err(); err != nil {
		return nil, err
	}
	return values, nil
}
