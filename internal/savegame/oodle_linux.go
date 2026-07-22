// The dynamic-loader portion is derived from new-world-tools/go-oodle.
// Copyright (c) 2021 Aleksandr Zelenin. Licensed under the MIT License.
// Modified to be instance-scoped and accept only an explicit caller path.
//go:build linux

package savegame

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

type dynamicOodle struct {
	mu         sync.Mutex
	decompress func(unsafe.Pointer, int, unsafe.Pointer, int64, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr, uintptr) uintptr
}

func loadOodle(absPath string) (oodleDecompressor, error) {
	if absPath == "" || !filepath.IsAbs(absPath) {
		return nil, fmt.Errorf("savegame: Oodle library path must be absolute")
	}
	st, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("savegame: stat Oodle library: %w", err)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("savegame: Oodle library is not a regular file")
	}
	handle, err := purego.Dlopen(absPath, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, fmt.Errorf("savegame: load Oodle library: %w", err)
	}
	symbol, err := purego.Dlsym(handle, "OodleLZ_Decompress")
	if err != nil {
		_ = purego.Dlclose(handle)
		return nil, fmt.Errorf("savegame: resolve OodleLZ_Decompress: %w", err)
	}
	d := &dynamicOodle{}
	purego.RegisterFunc(&d.decompress, symbol)
	// The handle intentionally remains open for the Reader's process lifetime;
	// closing it would invalidate the registered Go function while calls race.
	return d, nil
}

func (d *dynamicOodle) Decompress(src []byte, rawLen int) ([]byte, error) {
	if len(src) == 0 || rawLen <= 0 {
		return nil, fmt.Errorf("savegame: invalid Oodle input/output size")
	}
	out := make([]byte, rawLen)
	d.mu.Lock()
	n := d.decompress(unsafe.Pointer(&src[0]), len(src), unsafe.Pointer(&out[0]), int64(rawLen), 0, 0, 0, 0, 0, 0, 0, 0, 0, 3)
	d.mu.Unlock()
	runtime.KeepAlive(src)
	runtime.KeepAlive(out)
	if n == 0 {
		return nil, fmt.Errorf("savegame: Oodle decompression failed")
	}
	if int(n) != rawLen {
		return nil, fmt.Errorf("savegame: Oodle decompressed %d bytes, expected %d", n, rawLen)
	}
	return out, nil
}
