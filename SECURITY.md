# Security

Please report vulnerabilities privately through GitHub's private vulnerability reporting rather than opening a public issue.

Do not expose the Palworld REST API or its credentials publicly. REST Basic Auth stays in the map backend, and upstream account IDs and IP addresses are not included in its public data model.

This map does not provide general viewer authentication. Anyone who can access it can see online player names and positions, plus any enabled world-object layers. Restrict access to the map if that information should not be public.

The optional saved-character connection authenticates only self-private routes;
it is not an access-control layer for the live map. It uses a nonce-selected
eight-slot inventory cycle and a separately observed restore sequence across
new immutable backup generations. Claims require at least sixteen distinct
stack fingerprints, reveal slot numbers only, and use a dedicated persistent
installation secret, bounded hash-at-rest challenges and sessions, and Secure,
HttpOnly, SameSite=Strict cookies. Exact inventory contents, Pal instance IDs,
raw save identifiers, proof evidence, completion state keys, and the
installation secret must never be returned by an API or written to logs.

With claims enabled, public player endpoints are live-only and omit saved
progress, offline players, last-seen values, and saved locations. Authenticated
completion responses are self-only, no-store, and project private save keys to
already-public catalogue IDs. Enable claims only behind HTTPS with the exact
public origin configured. Trust forwarding headers only from explicitly
configured proxy CIDRs, apply edge request limits, and keep the save mount and
secret file read-only to the container.
