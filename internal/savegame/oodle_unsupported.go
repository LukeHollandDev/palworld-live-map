//go:build !linux

package savegame

import "fmt"

func loadOodle(string) (oodleDecompressor, error) {
	return nil, fmt.Errorf("savegame: Oodle PlM decoding is supported only on Linux")
}
