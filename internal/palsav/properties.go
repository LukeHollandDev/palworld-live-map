// Copyright (C) 2026 Luke Holland
// Portions copyright 2026 Palhelm contributors and licensed under Apache-2.0.
// Adapted and substantially modified on 2026-07-23. See NOTICE.
// SPDX-License-Identifier: GPL-3.0-or-later

package palsav

import (
	"errors"
	"fmt"
)

func readPropertyList(reader *archiveReader, path string) (Properties, error) {
	if err := reader.state.enter("property"); err != nil {
		return nil, err
	}
	defer reader.state.leave()

	var properties Properties
	for {
		offset := reader.offset()
		name, err := reader.fstring()
		if err != nil {
			return nil, fmt.Errorf("palsav: property name at %s: %w", displayPath(path), err)
		}
		if name == "None" {
			return properties, nil
		}
		propertyPath, err := joinPath(reader.state.cfg, path, name)
		if err != nil {
			return nil, err
		}
		if reader.state.properties >= reader.state.cfg.maxProperties {
			return nil, &LimitError{
				Kind:  "property count",
				Value: reader.state.properties + 1,
				Limit: reader.state.cfg.maxProperties,
			}
		}
		reader.state.properties++

		property, err := readPropertyTagAfterName(reader, name, offset)
		if err != nil {
			return nil, fmt.Errorf("palsav: property %s: %w", propertyPath, err)
		}
		if uint64(property.Size) > uint64(reader.remaining()) {
			return nil, fmt.Errorf(
				"palsav: property %s declares %d bytes with %d remaining",
				propertyPath,
				property.Size,
				reader.remaining(),
			)
		}
		payloadOffset := reader.offset()
		payload, err := reader.take(int(property.Size))
		if err != nil {
			return nil, err
		}
		property.Raw = payload

		// A known tag is still useful when a new Palworld value encoding is
		// encountered. Decode through a bounded sub-reader, and preserve the
		// exact payload if semantic decoding fails.
		propertyCount := reader.state.properties
		valueReader := reader.sub(payload, payloadOffset)
		value, decodeErr := readPropertyPayload(valueReader, &property, propertyPath)
		if decodeErr == nil && valueReader.remaining() != 0 {
			decodeErr = fmt.Errorf("%d unconsumed payload bytes", valueReader.remaining())
		}
		if decodeErr != nil {
			var limitErr *LimitError
			if errors.As(decodeErr, &limitErr) {
				return nil, decodeErr
			}
			reader.state.properties = propertyCount
			property.Value = RawValue{Data: payload, Reason: decodeErr.Error()}
		} else {
			property.Value = value
		}
		properties = append(properties, property)
	}
}

func readPropertyTag(reader *archiveReader) (Property, error) {
	offset := reader.offset()
	name, err := reader.fstring()
	if err != nil {
		return Property{}, err
	}
	if name == "None" {
		return Property{}, fmt.Errorf("unexpected None property tag")
	}
	return readPropertyTagAfterName(reader, name, offset)
}

func readPropertyTagAfterName(reader *archiveReader, name string, offset int) (Property, error) {
	property := Property{Name: name, Offset: offset}
	var err error
	if property.Type, err = reader.fstring(); err != nil {
		return Property{}, fmt.Errorf("type: %w", err)
	}
	if property.Size, err = reader.u32(); err != nil {
		return Property{}, fmt.Errorf("size: %w", err)
	}
	if property.ArrayIndex, err = reader.u32(); err != nil {
		return Property{}, fmt.Errorf("array index: %w", err)
	}

	switch property.Type {
	case "StructProperty":
		if property.Meta.StructType, err = reader.fstring(); err != nil {
			return Property{}, fmt.Errorf("struct type: %w", err)
		}
		if property.Meta.StructGUID, err = reader.guid(); err != nil {
			return Property{}, fmt.Errorf("struct GUID: %w", err)
		}
	case "BoolProperty":
		value, readErr := reader.u8()
		if readErr != nil {
			return Property{}, fmt.Errorf("bool tag value: %w", readErr)
		}
		property.Meta.BoolValue = value != 0
	case "ByteProperty", "EnumProperty":
		if property.Meta.EnumType, err = reader.fstring(); err != nil {
			return Property{}, fmt.Errorf("enum type: %w", err)
		}
	case "ArrayProperty", "SetProperty":
		if property.Meta.InnerType, err = reader.fstring(); err != nil {
			return Property{}, fmt.Errorf("inner type: %w", err)
		}
	case "MapProperty":
		if property.Meta.KeyType, err = reader.fstring(); err != nil {
			return Property{}, fmt.Errorf("key type: %w", err)
		}
		if property.Meta.ValueType, err = reader.fstring(); err != nil {
			return Property{}, fmt.Errorf("value type: %w", err)
		}
	default:
		if !supportedPlainPropertyType(property.Type) {
			return Property{}, fmt.Errorf("unsupported property tag type %q", property.Type)
		}
	}
	if property.PropertyGUID, err = reader.optionalGUID(); err != nil {
		return Property{}, fmt.Errorf("property GUID: %w", err)
	}
	return property, nil
}

func supportedPlainPropertyType(propertyType string) bool {
	switch propertyType {
	case "Int8Property", "Int16Property", "IntProperty", "Int64Property",
		"UInt8Property", "UInt16Property", "UInt32Property", "UInt64Property",
		"FixedPoint64Property", "FloatProperty", "DoubleProperty",
		"NameProperty", "StrProperty", "TextProperty", "ObjectProperty",
		"SoftObjectProperty", "WeakObjectProperty", "LazyObjectProperty",
		"InterfaceProperty", "DelegateProperty", "MulticastDelegateProperty":
		return true
	default:
		return false
	}
}

func readPropertyPayload(reader *archiveReader, property *Property, path string) (any, error) {
	switch property.Type {
	case "BoolProperty":
		if property.Size != 0 {
			return nil, fmt.Errorf("BoolProperty has nonzero size %d", property.Size)
		}
		return property.Meta.BoolValue, nil
	case "ByteProperty":
		if property.Meta.EnumType == "" || property.Meta.EnumType == "None" {
			return reader.u8()
		}
		value, err := reader.fstring()
		return EnumValue{Type: property.Meta.EnumType, Value: value}, err
	case "EnumProperty":
		value, err := reader.fstring()
		return EnumValue{Type: property.Meta.EnumType, Value: value}, err
	case "Int8Property":
		return reader.i8()
	case "Int16Property":
		return reader.i16()
	case "IntProperty", "FixedPoint64Property":
		return reader.i32()
	case "Int64Property":
		return reader.i64()
	case "UInt8Property":
		return reader.u8()
	case "UInt16Property":
		return reader.u16()
	case "UInt32Property":
		return reader.u32()
	case "UInt64Property":
		return reader.u64()
	case "FloatProperty":
		return reader.f32()
	case "DoubleProperty":
		return reader.f64()
	case "NameProperty", "StrProperty", "ObjectProperty":
		return reader.fstring()
	case "StructProperty":
		value, err := readStructBody(reader, property.Meta.StructType, path)
		return StructValue{Type: property.Meta.StructType, Value: value}, err
	case "ArrayProperty":
		return readArrayValue(reader, property, path)
	case "MapProperty":
		return readMapValue(reader, property, path)
	case "SetProperty":
		return readSetValue(reader, property, path)
	default:
		return nil, fmt.Errorf("unsupported property type %q", property.Type)
	}
}

func readStructBody(reader *archiveReader, structType, path string) (any, error) {
	switch structType {
	case "Guid":
		return reader.guid()
	case "DateTime", "Timespan":
		return reader.i64()
	case "Vector":
		x, err := reader.f64()
		if err != nil {
			return nil, err
		}
		y, err := reader.f64()
		if err != nil {
			return nil, err
		}
		z, err := reader.f64()
		return Vector{X: x, Y: y, Z: z}, err
	case "Vector2D":
		x, err := reader.f64()
		if err != nil {
			return nil, err
		}
		y, err := reader.f64()
		return Vector2D{X: x, Y: y}, err
	case "Quat":
		x, err := reader.f64()
		if err != nil {
			return nil, err
		}
		y, err := reader.f64()
		if err != nil {
			return nil, err
		}
		z, err := reader.f64()
		if err != nil {
			return nil, err
		}
		w, err := reader.f64()
		return Quat{X: x, Y: y, Z: z, W: w}, err
	case "Rotator":
		pitch, err := reader.f64()
		if err != nil {
			return nil, err
		}
		yaw, err := reader.f64()
		if err != nil {
			return nil, err
		}
		roll, err := reader.f64()
		return Rotator{Pitch: pitch, Yaw: yaw, Roll: roll}, err
	case "LinearColor":
		red, err := reader.f32()
		if err != nil {
			return nil, err
		}
		green, err := reader.f32()
		if err != nil {
			return nil, err
		}
		blue, err := reader.f32()
		if err != nil {
			return nil, err
		}
		alpha, err := reader.f32()
		return LinearColor{R: red, G: green, B: blue, A: alpha}, err
	case "Color":
		blue, err := reader.u8()
		if err != nil {
			return nil, err
		}
		green, err := reader.u8()
		if err != nil {
			return nil, err
		}
		red, err := reader.u8()
		if err != nil {
			return nil, err
		}
		alpha, err := reader.u8()
		return Color{B: blue, G: green, R: red, A: alpha}, err
	case "IntPoint":
		x, err := reader.i32()
		if err != nil {
			return nil, err
		}
		y, err := reader.i32()
		return IntPoint{X: x, Y: y}, err
	case "IntVector":
		x, err := reader.i32()
		if err != nil {
			return nil, err
		}
		y, err := reader.i32()
		if err != nil {
			return nil, err
		}
		z, err := reader.i32()
		return IntVector{X: x, Y: y, Z: z}, err
	default:
		return readPropertyList(reader, path)
	}
}

func readArrayValue(reader *archiveReader, property *Property, path string) (any, error) {
	count, err := reader.u32()
	if err != nil {
		return nil, err
	}
	innerType := property.Meta.InnerType
	if innerType == "StructProperty" {
		descriptor, err := readPropertyTag(reader)
		if err != nil {
			return nil, fmt.Errorf("struct-array descriptor: %w", err)
		}
		if descriptor.Type != "StructProperty" {
			return nil, fmt.Errorf("struct-array descriptor type is %q", descriptor.Type)
		}
		if uint64(descriptor.Size) != uint64(reader.remaining()) {
			return nil, fmt.Errorf(
				"struct-array descriptor declares %d bytes, have %d",
				descriptor.Size,
				reader.remaining(),
			)
		}
		if err := validateCount(
			reader.state.cfg,
			"struct array",
			count,
			reader.remaining(),
			minimumStructSize(descriptor.Meta.StructType),
		); err != nil {
			return nil, err
		}
		rawBase := reader.offset()
		raw, err := reader.take(reader.remaining())
		if err != nil {
			return nil, err
		}
		elementPath, err := joinPath(reader.state.cfg, path, descriptor.Name)
		if err != nil {
			return nil, err
		}
		return ArrayValue{
			InnerType: innerType,
			Structs: &StructArray{
				Count:        count,
				Name:         descriptor.Name,
				Type:         descriptor.Type,
				Size:         descriptor.Size,
				ArrayIndex:   descriptor.ArrayIndex,
				StructType:   descriptor.Meta.StructType,
				StructGUID:   descriptor.Meta.StructGUID,
				PropertyGUID: descriptor.PropertyGUID,
				raw:          raw,
				base:         rawBase,
				path:         elementPath,
				cfg:          reader.state.cfg,
			},
		}, nil
	}
	if err := validateCount(
		reader.state.cfg,
		"array",
		count,
		reader.remaining(),
		minimumBareSize(innerType, ""),
	); err != nil {
		return nil, err
	}
	values, err := readTypedArray(reader, innerType, count, path)
	if err != nil {
		return nil, err
	}
	return ArrayValue{InnerType: innerType, Values: values}, nil
}

func readTypedArray(reader *archiveReader, valueType string, count uint32, path string) (any, error) {
	switch valueType {
	case "ByteProperty", "UInt8Property":
		return reader.take(int(count))
	case "Int8Property":
		values := make([]int8, count)
		for i := range values {
			value, err := reader.i8()
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		return values, nil
	case "BoolProperty":
		values := make([]bool, count)
		for i := range values {
			value, err := reader.u8()
			if err != nil {
				return nil, err
			}
			values[i] = value != 0
		}
		return values, nil
	case "Int16Property":
		values := make([]int16, count)
		for i := range values {
			value, err := reader.i16()
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		return values, nil
	case "UInt16Property":
		values := make([]uint16, count)
		for i := range values {
			value, err := reader.u16()
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		return values, nil
	case "IntProperty", "FixedPoint64Property":
		values := make([]int32, count)
		for i := range values {
			value, err := reader.i32()
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		return values, nil
	case "UInt32Property":
		values := make([]uint32, count)
		for i := range values {
			value, err := reader.u32()
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		return values, nil
	case "Int64Property":
		values := make([]int64, count)
		for i := range values {
			value, err := reader.i64()
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		return values, nil
	case "UInt64Property":
		values := make([]uint64, count)
		for i := range values {
			value, err := reader.u64()
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		return values, nil
	case "FloatProperty":
		values := make([]float32, count)
		for i := range values {
			value, err := reader.f32()
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		return values, nil
	case "DoubleProperty":
		values := make([]float64, count)
		for i := range values {
			value, err := reader.f64()
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		return values, nil
	case "EnumProperty", "NameProperty", "StrProperty", "ObjectProperty":
		values := make([]string, count)
		for i := range values {
			value, err := reader.fstring()
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		return values, nil
	default:
		if valueType != "Guid" {
			return nil, fmt.Errorf("unsupported array element type %q", valueType)
		}
		values := make([]any, count)
		for i := range values {
			value, err := readBareValue(reader, valueType, "", path)
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		return values, nil
	}
}

func readMapValue(reader *archiveReader, property *Property, path string) (any, error) {
	keyPath, err := joinPath(reader.state.cfg, path, "Key")
	if err != nil {
		return nil, err
	}
	valuePath, err := joinPath(reader.state.cfg, path, "Value")
	if err != nil {
		return nil, err
	}
	keyHint := structHint(reader.state.cfg, keyPath, property.Meta.KeyType)
	valueHint := structHint(reader.state.cfg, valuePath, property.Meta.ValueType)

	removedCount, err := reader.u32()
	if err != nil {
		return nil, err
	}
	if err := validateCount(
		reader.state.cfg,
		"map removed-key",
		removedCount,
		reader.remaining(),
		minimumBareSize(property.Meta.KeyType, keyHint),
	); err != nil {
		return nil, err
	}
	removed := make([]any, removedCount)
	for i := range removed {
		removed[i], err = readBareValue(reader, property.Meta.KeyType, keyHint, keyPath)
		if err != nil {
			return nil, fmt.Errorf("removed map key %d: %w", i, err)
		}
	}

	count, err := reader.u32()
	if err != nil {
		return nil, err
	}
	minimum := minimumBareSize(property.Meta.KeyType, keyHint) +
		minimumBareSize(property.Meta.ValueType, valueHint)
	if err := validateCount(reader.state.cfg, "map", count, reader.remaining(), minimum); err != nil {
		return nil, err
	}
	rawBase := reader.offset()
	raw, err := reader.take(reader.remaining())
	if err != nil {
		return nil, err
	}
	return &MapValue{
		KeyType:         property.Meta.KeyType,
		ValueType:       property.Meta.ValueType,
		KeyStructType:   keyHint,
		ValueStructType: valueHint,
		Removed:         removed,
		Count:           count,
		raw:             raw,
		base:            rawBase,
		path:            path,
		keyPath:         keyPath,
		valuePath:       valuePath,
		cfg:             reader.state.cfg,
	}, nil
}

func readSetValue(reader *archiveReader, property *Property, path string) (any, error) {
	elementPath, err := joinPath(reader.state.cfg, path, property.Meta.InnerType)
	if err != nil {
		return nil, err
	}
	elementHint := structHint(reader.state.cfg, elementPath, property.Meta.InnerType)

	removedCount, err := reader.u32()
	if err != nil {
		return nil, err
	}
	if err := validateCount(
		reader.state.cfg,
		"set removed-element",
		removedCount,
		reader.remaining(),
		minimumBareSize(property.Meta.InnerType, elementHint),
	); err != nil {
		return nil, err
	}
	removed := make([]any, removedCount)
	for i := range removed {
		removed[i], err = readBareValue(reader, property.Meta.InnerType, elementHint, elementPath)
		if err != nil {
			return nil, fmt.Errorf("removed set element %d: %w", i, err)
		}
	}

	count, err := reader.u32()
	if err != nil {
		return nil, err
	}
	if err := validateCount(
		reader.state.cfg,
		"set",
		count,
		reader.remaining(),
		minimumBareSize(property.Meta.InnerType, elementHint),
	); err != nil {
		return nil, err
	}
	rawBase := reader.offset()
	raw, err := reader.take(reader.remaining())
	if err != nil {
		return nil, err
	}
	return &SetValue{
		ElementType:       property.Meta.InnerType,
		ElementStructType: elementHint,
		Removed:           removed,
		Count:             count,
		raw:               raw,
		base:              rawBase,
		path:              elementPath,
		cfg:               reader.state.cfg,
	}, nil
}

func readBareValue(reader *archiveReader, valueType, structType, path string) (any, error) {
	switch valueType {
	case "StructProperty":
		if structType == "" {
			structType = "StructProperty"
		}
		if structType == "StructProperty" {
			return readPropertyList(reader, path)
		}
		return readStructBody(reader, structType, path)
	case "Guid":
		return reader.guid()
	case "BoolProperty":
		value, err := reader.u8()
		return value != 0, err
	case "ByteProperty", "UInt8Property":
		return reader.u8()
	case "Int8Property":
		return reader.i8()
	case "Int16Property":
		return reader.i16()
	case "UInt16Property":
		return reader.u16()
	case "IntProperty", "FixedPoint64Property":
		return reader.i32()
	case "UInt32Property":
		return reader.u32()
	case "Int64Property":
		return reader.i64()
	case "UInt64Property":
		return reader.u64()
	case "FloatProperty":
		return reader.f32()
	case "DoubleProperty":
		return reader.f64()
	case "EnumProperty", "NameProperty", "StrProperty", "ObjectProperty":
		return reader.fstring()
	default:
		return nil, fmt.Errorf("unsupported bare value type %q at %s", valueType, displayPath(path))
	}
}

func structHint(cfg *decodeConfig, path, valueType string) string {
	if valueType != "StructProperty" {
		return ""
	}
	if hint := cfg.typeHints[path]; hint != "" {
		return hint
	}
	return "StructProperty"
}

func minimumBareSize(valueType, structType string) int {
	switch valueType {
	case "BoolProperty", "ByteProperty", "Int8Property", "UInt8Property":
		return 1
	case "Int16Property", "UInt16Property":
		return 2
	case "IntProperty", "UInt32Property", "FloatProperty", "FixedPoint64Property":
		return 4
	case "Int64Property", "UInt64Property", "DoubleProperty":
		return 8
	case "Guid":
		return 16
	case "StructProperty":
		return minimumStructSize(structType)
	case "EnumProperty", "NameProperty", "StrProperty", "ObjectProperty":
		return 4
	default:
		return 1
	}
}

func minimumStructSize(structType string) int {
	switch structType {
	case "Guid":
		return 16
	case "DateTime", "Timespan":
		return 8
	case "Vector", "Rotator":
		return 24
	case "Vector2D":
		return 16
	case "Quat":
		return 32
	case "LinearColor":
		return 16
	case "Color":
		return 4
	case "IntPoint":
		return 8
	case "IntVector":
		return 12
	default:
		// The FString "None" terminator of an empty tagged property list.
		return 9
	}
}

func joinPath(cfg *decodeConfig, parent, child string) (string, error) {
	size := uint64(len(parent)) + 1 + uint64(len(child))
	if size > uint64(cfg.maxPathBytes) {
		return "", &LimitError{
			Kind:  "property path bytes",
			Value: size,
			Limit: uint64(cfg.maxPathBytes),
		}
	}
	if parent == "" {
		return "." + child, nil
	}
	return parent + "." + child, nil
}

func displayPath(path string) string {
	if path == "" {
		return "<root>"
	}
	return path
}
