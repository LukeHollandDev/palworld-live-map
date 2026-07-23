# Licensing

Palworld Live Map is a multi-licence source distribution whose Go server
directly links GPL-covered decoder code. Individual components retain the
licences identified below, but the distributed server executable is a combined
work licensed as a whole under GPL-3.0-or-later.

| Component | Licence |
| --- | --- |
| Combined `palworld-live-map` Go server executable | [GPL-3.0-or-later](LICENSES/GPL-3.0-or-later.txt) |
| Original Go application source, web application source, project documentation, and other original project files unless marked otherwise | [MIT](LICENSE) |
| Selective save reader under `internal/savegame` | [Apache-2.0](internal/savegame/LICENSE), with its [NOTICE](internal/savegame/NOTICE) |
| Internal `palsav` package, including its ooz-derived Mermaid decoder | [GPL-3.0-or-later](internal/palsav/LICENSE), with its [NOTICE](internal/palsav/NOTICE) |
| Palhelm-derived portions of `internal/palsav` | Apache-2.0, reproduced in the package's [`LICENSES`](internal/palsav/LICENSES) directory and covered by its [NOTICE](internal/palsav/NOTICE) |
| React, React DOM, and Scheduler runtime code in the web bundle | [MIT](LICENSES/React-MIT.txt) |
| Palworld-derived map textures, screenshots, and extracted game data | Copyright Pocketpair; no licence is granted by this project. See the [map](assets/map/README.md) and [landmark](assets/landmarks/README.md) provenance notes. |

The root [MIT licence](LICENSE) continues to grant its permissions for the
original files to which it applies. The Apache-2.0 notices likewise remain in
force for their covered files. Because these permissive licences are compatible
with GPLv3, the combined server may be conveyed under GPL-3.0-or-later while
preserving the underlying component notices and permissions. Merely disabling
save loading at runtime does not remove the linked decoder or change the
server's licence.

Distributors of an executable or container must satisfy GPLv3's object-code
requirements for the complete combined server. That includes providing the
exact Corresponding Source used for the build, the internal `palsav` package
source, and the scripts and source needed to build the executable. A decoder-
only source archive is not sufficient. Published artefacts must also include or
point clearly to the applicable licence and attribution notices.

Published container images use
`GPL-3.0-or-later AND Apache-2.0 AND MIT AND
LicenseRef-Pocketpair-Proprietary` as their
`org.opencontainers.image.licenses` value. They identify the public source
repository through `org.opencontainers.image.source` and the exact matching
commit through `org.opencontainers.image.revision`. Recipients should use those
OCI labels to obtain the Corresponding Source for their image; a moving branch
or a different release is not a substitute. Every image also carries the exact
complete source used for its server binary under `/usr/src/palworld-live-map`
and the applicable licence and NOTICE files under `/licenses`.

## Decoder provenance

The Mermaid-compatible decoder in [`internal/palsav`](internal/palsav)
was ported and substantially modified from ooz-derived files distributed by
PalworldSaveTools, whose downstream package declares GPL-3.0-or-later. No
licence file or other affirmative copyright grant has been identified for the
original Powzix ooz source from which that work descends.

The GPL designation records the downstream terms under which the imported
implementation was supplied and under which this project treats the combined
server. It does not establish that Powzix granted permission to copy, modify, or
redistribute the original implementation. That unresolved provenance, and any
separate reverse-engineering or patent questions around an Oodle-compatible
decoder, require qualified legal review before redistribution. Moving the code
into Go does not remove those issues.

No proprietary Epic or RAD Game Tools Oodle runtime or source code is included.
`LicenseRef-Pocketpair-Proprietary` identifies the Palworld-derived assets
listed above; it does not grant redistribution permission.
