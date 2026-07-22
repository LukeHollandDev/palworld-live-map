// Portions derived from Palhelm and modified for Palworld Live Map.
// Copyright 2026 Palhelm contributors. Licensed under Apache-2.0.
package savegame

import "strconv"

func asProperties(v any) (propertyMap, bool) {
	switch x := v.(type) {
	case propertyMap:
		return x, true
	case structData:
		p, ok := x.Value.(propertyMap)
		return p, ok
	default:
		return nil, false
	}
}

func propertyProperties(p propertyMap, name string) (propertyMap, bool) {
	q := p[name]
	if q == nil {
		return nil, false
	}
	return asProperties(q.Value)
}

func propertyInt(p propertyMap, name string) (int64, bool) {
	q := p[name]
	if q == nil {
		return 0, false
	}
	switch v := q.Value.(type) {
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		if v <= uint64(^uint64(0)>>1) {
			return int64(v), true
		}
	case enumData:
		// ByteProperty with enum type None carries its numeric byte as text.
		if n, err := strconv.ParseInt(v.Value, 10, 64); err == nil {
			return n, true
		}
	}
	return 0, false
}

func firstInt(p propertyMap, names ...string) int64 {
	for _, name := range names {
		if v, ok := propertyInt(p, name); ok {
			return v
		}
	}
	return 0
}

func firstBool(p propertyMap, names ...string) bool {
	for _, name := range names {
		if q := p[name]; q != nil {
			if v, ok := q.Value.(bool); ok {
				return v
			}
		}
	}
	return false
}

func firstString(p propertyMap, names ...string) string {
	for _, name := range names {
		if q := p[name]; q != nil {
			switch v := q.Value.(type) {
			case string:
				return v
			case enumData:
				return v.Value
			case structData:
				if s, ok := v.Value.(string); ok {
					return s
				}
			}
		}
	}
	return ""
}

func firstVector(p propertyMap, names ...string) (Vector, bool) {
	for _, name := range names {
		if q := p[name]; q != nil {
			if s, ok := q.Value.(structData); ok {
				if v, ok := s.Value.(Vector); ok {
					return v, true
				}
			}
		}
	}
	return Vector{}, false
}
