package mapdata

import "testing"

func TestLayerIDUsesShippedBoundsAndWorldTreePriority(t *testing.T) {
	tests := []struct {
		name string
		x    float64
		y    float64
		id   string
		ok   bool
	}{
		{name: "palpagos", x: -400000, y: 200000, id: "palpagos", ok: true},
		{name: "world tree", x: 500000, y: -600000, id: "world-tree", ok: true},
		{name: "overlap prefers world tree", x: 348000, y: -600000, id: "world-tree", ok: true},
		{name: "outside artwork", x: 900000, y: 900000, ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id, ok := LayerID(test.x, test.y)
			if id != test.id || ok != test.ok {
				t.Fatalf("LayerID(%g, %g) = %q, %v; want %q, %v", test.x, test.y, id, ok, test.id, test.ok)
			}
		})
	}
}

func TestKnownLayerRejectsUnknownOrMismatchedMetadata(t *testing.T) {
	if !KnownLayer("palpagos", palpagosBounds) || !KnownLayer("world-tree", worldTreeBounds) {
		t.Fatal("shipped layer metadata was rejected")
	}
	if KnownLayer("custom", palpagosBounds) || KnownLayer("palpagos", worldTreeBounds) {
		t.Fatal("unknown or mismatched layer metadata was accepted")
	}
}
