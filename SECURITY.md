# Security

Please report vulnerabilities privately through GitHub's private vulnerability reporting rather than opening a public issue.

The Palworld REST API must remain on a private Docker network. This service is the only component intended for reverse-proxy exposure. REST Basic Auth stays in the backend, and upstream account IDs and IP addresses are never represented in the public data model.

This map intentionally has no viewer authentication. Anyone who can reach it can see live player names and positions, plus any enabled world-object layers. Keep it LAN/VPN-only if that information should not be public, or add access control at the reverse proxy.
