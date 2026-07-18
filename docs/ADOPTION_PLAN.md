# Adoption and launch roadmap

This is the working roadmap for making Palworld Live Map easy to discover, evaluate, install, and recommend.

## v0.2.0 launch

- [x] Add a deterministic, clearly labelled demo mode with no Palworld server or secrets.
- [x] Add one-Compose hosting for a new Palworld server and live map, keeping REST port 8212 private.
- [x] Replace TH.GL-derived files with native 8K textures exported from a local Palworld installation.
- [x] Add a pinned, repeatable exporter and an embedded provenance manifest.
- [x] Add a populated hero screenshot, troubleshooting, issue forms, and release automation.
- [x] Set the GitHub description and discovery topics.
- [ ] Select a reliable demo host, deploy the shared demo, and set its URL as the GitHub homepage.
- [ ] Upload `docs/images/social-preview.png` in GitHub repository settings.
- [ ] Send the prepared map-licensing/source enquiries to Pocketpair and TH.GL.
- [ ] Tag `v0.2.0` after CI and container smoke tests pass on the committed change.

## Launch

- Publish release notes and a 45–60 second setup/demo video.
- Announce to Palworld players first, then Palworld server operators and self-hosting communities.
- Offer the tested Compose integration to the Thijs Palworld Docker project.
- Track unique visitors, referrers, demo visits, image pulls, successful installs, and recurring setup failures.

## Follow-up adoption work

- Add Docker Hub only if users report GHCR friction.
- Add an Unraid Community Applications template after the Compose workflow is validated by real operators.
- Add other Palworld server images only with a maintainer and compatibility tests.
- Keep the project player-facing and read-only; do not dilute it into an administration dashboard.

See [`LAUNCH.md`](LAUNCH.md) for release copy and the distribution checklist.
