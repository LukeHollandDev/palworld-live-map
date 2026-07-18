# Launch checklist and copy

## Repository settings

- Description: `Self-hosted live map for Palworld dedicated servers—live players, bases, Pals, NPCs and server metrics. Docker; no client mods.`
- Topics: `palworld`, `palworld-server`, `palworld-dedicated-server`, `live-map`, `self-hosted`, `docker`, `docker-compose`, `game-server`, `golang`, `rest-api`.
- Social preview: upload `docs/images/social-preview.png` in the repository's social-preview settings.
- Homepage: set the stable hosted-demo URL after deployment.

## Release gate

- `make check` and `make image` pass.
- Both Compose files pass `docker compose config`.
- Demo container shows populated markers and an explicit demo badge.
- Full-stack instructions have been repeated from a clean directory.
- Map manifest hashes match the embedded images.
- Release notes call out demo mode, one-Compose hosting, first-party extraction provenance, and any compatibility changes.

## Short launch post

> I built a self-hosted live map for Palworld dedicated servers. It shows live players across Palpagos and World Tree, plus bases, workers, Pals, NPCs, FPS, uptime, and server capacity. Players need only a browser—there are no client mods, and the REST admin password stays in the backend. You can try the fictional-data demo first, add the map to an existing server, or launch a new Palworld server and map together with one Compose file.

Tailor the final sentence and link for each community. Lead Palworld communities with the player experience, and self-hosting communities with the container/security design.

## Distribution order

1. Publish the GitHub release and hosted demo.
2. Post the 45–60 second demo/setup video and release link in Palworld communities and relevant Discord servers.
3. Share the technical setup in self-hosted and Docker communities.
4. List it on Nexus Mods as an external dedicated-server utility, not a game mod.
5. Ask the Thijs Palworld Docker maintainer to link the tested integration.
6. Consider Docker Hub and an Unraid template after real installation reports validate the workflow.

Track GitHub traffic sources, unique visitors, demo referrals, container pulls, installation reports, and repeated setup failures. Stars are an outcome, not the only adoption signal.
