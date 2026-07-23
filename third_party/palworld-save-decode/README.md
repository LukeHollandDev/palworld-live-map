# Palworld Save Decode

This directory contains the complete corresponding source for the
`palworld-save-decode` executable shipped in the Palworld Live Map container.
It is a decoder-only, process-isolated wrapper around the community ooz
implementation of Oodle-compatible Kraken, Mermaid, Selkie, Leviathan, LZNA,
and Bitknit decompression.

Build it with:

```sh
make
```

The helper accepts the declared decompressed size and reads a compressed Oodle
stream from standard input:

```sh
palworld-save-decode --raw-size 12345 < compressed.bin > raw.bin
```

## Provenance and licence

The decoder files under `decoder/` were copied from the `palooz` decoder in
`deafdudecomputers/PalworldSaveTools` commit
`f35b4d740259a7a75b11cccf2b7c35f928c1ab77`. That package declares
`GPL-3.0-or-later`; its decoder is derived from Powzix's ooz implementation.
Only the decoder files are included here. The separate upstream compressor
sources, some of which are restricted to educational use, are not included or
compiled.

Palworld Live Map modifies `decoder/kraken.cpp` to build without compressor or
standalone-tool symbols and to send diagnostics to standard error. It adds the
bounded stdin/stdout wrapper in `main.cpp`.

The helper and decoder are distributed under GPL-3.0-or-later. The bundled
SIMDe 0.8.4 compatibility headers originate from upstream commit
`dd0b662fd8cf4b1617dbbb4d08aa053e512b08e4` and retain their MIT and CC0-1.0
notices. Full licence texts are under `LICENSES/`.

This is an unofficial compatible decoder. It contains no proprietary Epic or
RAD Game Tools Oodle runtime or source code. It is not fuzz-safe; callers must
isolate it and bound its input, output, memory, and execution time.
