# Palworld Asset Exporter: third-party boundaries

The source under `exporter/src` and the fixture harness under `exporter/tests`
are original project code licensed under the repository's MIT licence.

The exporter restores third-party packages from NuGet at build time. Direct
dependencies and their upstream licences include:

| Dependency | Licence |
| --- | --- |
| CUE4Parse and CUE4Parse-Conversion | Apache-2.0 |
| Microsoft.Bcl.Memory | MIT |
| Newtonsoft.Json | MIT |
| SixLabors.ImageSharp | Six Labors Split License 1.0; this MIT-licensed open-source project qualifies for its Apache-2.0 grant |
| SkiaSharp native assets | MIT, with upstream Skia notices |

Exact direct and transitive versions are pinned in
`src/packages.lock.json` and `tests/packages.lock.json`. Anyone distributing a
built exporter image or binary should generate and include a complete licence
and notice inventory for that resolved dependency graph.

`export.sh` downloads a checksum-pinned `Mappings.usmap` from
`PalworldModding/UsefulFiles`. That repository does not currently state an
explicit licence. The mappings file is used as a local input and is not
committed to or redistributed by this repository.

Map textures, names, coordinates, and other data extracted from a Palworld
installation remain subject to Pocketpair's rights. Running the exporter does
not transfer ownership or grant redistribution permission for generated
output.
