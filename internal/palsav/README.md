# Internal Palworld save decoder

`palsav` is the repository's internal, read-only, standard-library-only Go
decoder for Palworld `.sav` files. It loads the Palworld container,
decompresses current Mermaid or legacy zlib saves, parses Unreal's GVAS archive,
and exposes ordered properties and lazy collections.

- Go 1.26.5
- Part of the root `palworld-live-map` Go module
- No external Go packages
- No cgo, DLL, proprietary Oodle runtime, or helper process
- Bounded parsing APIs for untrusted input

The package decoded and recursively walked every stored value in the 13 saves
used to develop it, including `Level.sav`, `LevelMeta.sav`, normal player
saves, and both 9,600-record DPS saves.

## Repository use

This package lives at `internal/palsav` in the root module. Go's `internal`
visibility rule deliberately prevents consumers outside
`github.com/LukeHollandDev/palworld-live-map` from importing it. It is a
repository implementation package, not a supported external API.

## Load a save

```go
package main

import (
	"fmt"
	"log"

	palsav "github.com/LukeHollandDev/palworld-live-map/internal/palsav"
)

func main() {
	save, err := palsav.Load("LevelMeta.sav")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(save.Header.ClassName)
	for _, property := range save.Properties {
		fmt.Printf("%s (%s): %#v\n",
			property.Name,
			property.Type,
			property.Value,
		)
	}
}
```

Use `Decode` for an in-memory complete `.sav` file and `ParseGVAS` when the
input has already been decompressed:

```go
save, err := palsav.Decode(containerBytes)
gvas, err := palsav.ParseGVAS(rawGVASBytes)
```

## Data model

`Properties` is a slice, not a map. This preserves serialized order, duplicate
names, and `ArrayIndex`. `Properties.Find` returns the first matching property.
Every `Property` also retains its exact size-counted payload in `Raw`.

`Property.Value` has these common concrete types:

| Unreal type | Go value |
| --- | --- |
| Bool/numeric properties | `bool` or the corresponding fixed-width number |
| `StrProperty`, `NameProperty`, `ObjectProperty` | `string` |
| `ByteProperty` | `uint8`, or `EnumValue` when enum-backed |
| `EnumProperty` | `EnumValue` |
| `StructProperty` | `StructValue` |
| `ArrayProperty` | `ArrayValue` |
| `MapProperty` | `*MapValue` |
| `SetProperty` | `*SetValue` |
| Safely tagged but unknown value encoding | `RawValue` |

Known native structs decode to `GUID`, `Vector`, `Vector2D`, `Quat`, `Rotator`,
`LinearColor`, `Color`, `IntPoint`, `IntVector`, or `int64` for
`DateTime`/`Timespan`. Other Palworld structs decode as ordered `Properties`.
Primitive arrays use compact typed slices; byte arrays use `[]byte`.

## Lazy collections

Maps, sets, and struct arrays retain their encoded region and decode elements
on demand. Iterators keep memory bounded and should be preferred for large
world maps and DPS saves:

```go
property := save.Properties.Find("SaveParameterArray")
arrayValue := property.Value.(palsav.ArrayValue)

iterator := arrayValue.Structs.Iterator()
for iterator.Next() {
	properties := iterator.Value().(palsav.Properties)
	_ = properties
}
if err := iterator.Err(); err != nil {
	log.Fatal(err)
}
```

`MapValue.Entries`, `SetValue.Values`, and `StructArray.Values` are convenient
eager alternatives. Iterators are independent and may be created more than
once.

Legacy GVAS map and set elements tagged only as `StructProperty` do not carry
their concrete struct identity. Built-in Palworld path hints cover the supplied
schema. Extend or override them with `Options.TypeHints`; keys are full paths
such as `.worldSaveData.BaseCampSaveData.Key`, and values are `Guid`, another
supported native struct name, or `StructProperty` for a tagged property list.
Map hints append `.Key` or `.Value`; set hints append the serialized inner type,
for example `.worldSaveData.SomeSet.StructProperty`.

## Limits and buffer ownership

Use `LoadWithOptions`, `DecodeWithOptions`, or `ParseGVASWithOptions` to set
limits:

```go
save, err := palsav.LoadWithOptions("Level.sav", palsav.Options{
	Limits: palsav.Limits{
		MaxInputBytes:  256 << 20,
		MaxOutputBytes: 512 << 20,
	},
	MaxDepth: 96,
})
```

Defaults are 1 GiB input/output, 16 MiB per FString, 64 KiB per constructed
property path, 10 million elements per collection, nesting depth 128, and
10 million eagerly parsed properties.
Limit failures can be inspected with:

```go
var limitErr *palsav.LimitError
if errors.As(err, &limitErr) {
	// Handle a configured resource limit.
}
```

`Save.Raw`, `Property.Raw`, `Trailer`, primitive byte arrays, and lazy
collections share the retained decompressed buffer. Treat all returned data as
immutable. `ParseGVAS` does not copy its input: the caller must not reuse or
mutate that byte slice while the returned `Save` is in use.

## Supported formats

The container reader supports:

- `PlM` type `0x31`, using a pure-Go decoder for the Oodle Mermaid subset
  exercised by the supplied saves
- `PlZ` type `0x31` (single zlib) and `0x32` (double zlib)
- The optional 12-byte CNK prefix used by some save tools
- GVAS save-game version 3 and custom-version format 3

The Mermaid decoder handles raw and memset quanta, mode-1 LZ chunks, raw/RLE
entropy, and the new Huffman forms present in these saves. It deliberately
rejects mode 0, tANS entropy, recursive entropy type 5, legacy Huffman tables,
and checksummed quanta rather than claiming support for arbitrary Oodle
streams.

The package does not encode, recompress, repair, or modify saves.

## Private fixture tests

Synthetic tests are self-contained. To run the optional integration suite
against a save tree without checking saves into source control:

```sh
PALWORLD_SAVE_FIXTURES=/path/to/world/snapshot go test ./internal/palsav
```

The directory should contain `Level.sav`, `LevelMeta.sav`, and `Players/`.
For private byte-exact regression checks, set
`PALWORLD_SAVE_GOLDEN_SHA256_FILE` to a file containing one decompressed
SHA-256 per input: sorted `Players/*.sav`, then `Level.sav`, then
`LevelMeta.sav`. `PALWORLD_SAVE_GOLDEN_SHA256` accepts the same whitespace-
separated values inline. Golden values stay outside the source tree.
The package directory's `.gitignore` excludes those paths, but it cannot
protect a manual archive of a directory containing real saves. Never add a
private save snapshot to the repository or a release archive.

## License and provenance

The package's contributors offer their work under `GPL-3.0-or-later`. The
Mermaid-compatible decoder is a Go port and substantial 2026-07-23 modification
of Powzix's ooz decoder as distributed by PalworldSaveTools, whose downstream
package declares `GPL-3.0-or-later`. No licence grant has been located in the
original Powzix repository, so the downstream GPL notice does not by itself
resolve that upstream provenance. Because this package is part of the server
executable, that combined binary is distributed under GPL-3.0-or-later, subject
to having the necessary upstream rights in the first place.

Some GVAS/container work is derived from Apache-2.0 Palhelm code. See
[`NOTICE`](NOTICE) and [`LICENSES`](LICENSES/) for exact revisions and notices.
The root [`LICENSING.md`](../../LICENSING.md) describes the combined server and
Corresponding Source distribution. This is an unofficial compatible reader and
is not affiliated with Epic Games, RAD Game Tools, or Pocketpair.
