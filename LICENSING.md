# Licensing

Palworld Live Map is a multi-licence distribution. A licence applies to the
component identified below; inclusion in the same repository or container does
not replace a component's own terms.

| Component | Licence |
| --- | --- |
| Go application, web application, project documentation, and other original project files unless marked otherwise | [MIT](LICENSE) |
| Selective save reader under `internal/savegame` | [Apache-2.0](internal/savegame/LICENSE), with its [NOTICE](internal/savegame/NOTICE) |
| Separately executed `palworld-save-decode` helper and ooz-derived decoder | [GPL-3.0-or-later](third_party/palworld-save-decode/LICENSES/GPL-3.0-or-later.txt) |
| SIMDe headers bundled with the helper | MIT and CC0-1.0, as marked in each file and reproduced under the helper's [`LICENSES`](third_party/palworld-save-decode/LICENSES) directory |
| React, React DOM, and Scheduler runtime code in the web bundle | [MIT](LICENSES/React-MIT.txt) |
| Palworld-derived map textures, screenshots, and extracted game data | Copyright Pocketpair; no licence is granted by this project. See the [map](assets/map/README.md) and [landmark](assets/landmarks/README.md) provenance notes. |

The container's `org.opencontainers.image.licenses` label is
`MIT AND Apache-2.0 AND GPL-3.0-or-later AND CC0-1.0 AND
LicenseRef-Pocketpair-Proprietary`. It includes the helper's exact corresponding
source under `/usr/src/palworld-save-decode` and all relevant licence notices
under `/licenses`.

No proprietary Epic or RAD Game Tools Oodle runtime or source code is included.
`LicenseRef-Pocketpair-Proprietary` identifies the Palworld-derived assets
listed above; it does not grant redistribution permission.
