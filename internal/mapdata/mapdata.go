// Package mapdata owns the coordinate bounds shared by upstream projection and
// embedded-map validation. World Tree is checked first because the two shipped
// images overlap by a narrow strip.
package mapdata

var (
	palpagosBounds  = [4]float64{349400, 724400, -1099400, -724400}
	worldTreeBounds = [4]float64{689148.5, -476400, 347351.5, -818197}
)

const LayerCount = 2

// LayerID returns the shipped map containing a world coordinate.
func LayerID(x, y float64) (string, bool) {
	if contains(worldTreeBounds, x, y) {
		return "world-tree", true
	}
	if contains(palpagosBounds, x, y) {
		return "palpagos", true
	}
	return "", false
}

// KnownLayer verifies that manifest metadata matches one of the coordinate
// systems understood by the upstream projector.
func KnownLayer(id string, bounds [4]float64) bool {
	switch id {
	case "palpagos":
		return bounds == palpagosBounds
	case "world-tree":
		return bounds == worldTreeBounds
	default:
		return false
	}
}

func contains(bounds [4]float64, x, y float64) bool {
	maxX, maxY, minX, minY := bounds[0], bounds[1], bounds[2], bounds[3]
	return x >= minX && x <= maxX && y >= minY && y <= maxY
}
