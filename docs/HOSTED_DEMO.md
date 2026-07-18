# Hosting the demo

Demo mode needs no Palworld server, persistent disk, or secrets. It runs the real application with deterministic fictional data.

The repository does not currently endorse a specific hosted provider. Any container platform must run the public GHCR image, expose HTTP port `8080`, and set:

```text
DEMO_MODE=true
ADDR=:8080
```

## Free-host options to evaluate

| Provider | Strength | Trade-off |
| --- | --- | --- |
| Hugging Face Docker Spaces | Generous free CPU/RAM and a longer idle window | Requires a separate public Space repository or sync workflow |
| Back4app Containers | GitHub/Docker deployment without a card | Lower 256 MB free memory and US-only free runtime |
| Render | Familiar Git/Docker workflow | Free service sleeps after 15 minutes and wakes slowly |
| Northflank | Capable free developer sandbox | Requires a payment card |
| Cloud Run | Mature container platform and compute grant | Requires billing setup; map-image egress can exceed the small free allowance |

Railway and Fly.io are not treated as permanent free-demo targets because their current entry tiers do not provide a dependable ongoing free container.

## Smoke check

After deployment, verify:

```bash
curl -fsS https://YOUR-DEMO-DOMAIN/-/health
curl -fsS https://YOUR-DEMO-DOMAIN/api/config
```

The configuration response must contain `"demoMode":true`. The UI must display the “Demo data” badge and populated markers on both regions.

