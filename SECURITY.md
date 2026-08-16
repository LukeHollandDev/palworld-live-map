# Security

Please report vulnerabilities privately through GitHub's private vulnerability reporting rather than opening a public issue.

Do not expose the Palworld REST API or its credentials publicly. REST Basic Auth stays in the map backend, and upstream account IDs and IP addresses are not included in its public data model.

This map does not provide general viewer authentication. Anyone who can access it can see online player names and positions, plus any enabled world-object layers. Restrict access to the map if that information should not be public.

The optional saved-character connection authenticates only self-private routes;
it is not an access-control layer for the live map. Its primary proof is a
one-shot, two-question memory quiz generated from a safely completed backup.
Each question exposes one save-backed answer among seven global decoys and can
be replaced independently without changing the other cards. Questions may use
private inventory containers, equipment, food, or party slots. A wrong submission
consumes the challenge, and existing per-client and global limits bound guessing and private
decoder work. Correct option indexes, counts, dynamic instance IDs, raw item IDs, raw
save identifiers, proof evidence, completion state keys, and the installation
secret must never be returned by an API or written to logs. Challenges and
sessions are bounded and stored hash-at-rest; sessions use HttpOnly,
SameSite=Strict cookies (`Secure` under HTTPS).

The reversible inventory transition remains only as a fallback for saves that
cannot produce a safe quiz. For reload recovery, the browser may retain only
its validated slot pairs, proof phase, and advisory per-swap progress in
`sessionStorage`; bearer tokens, player identities, quiz questions or answers,
item data, and save progress must never be persisted there. That recovery record
is cleared only after verified completion or explicit recovery acknowledgement.

Enabling claims does not change the existing public player endpoints: their
online and offline rosters, saved positions, levels, guild relationships,
last-seen values, and aggregate progression remain public. Only the new exact
per-landmark completion response is authenticated, self-only, no-store, and
projects private save keys to already-public catalogue IDs. Configure the exact browser-facing HTTP or HTTPS
origin. HTTPS is strongly recommended on public or untrusted networks because
HTTP cannot prevent interception of a player's session and self-private
completion progress. HTTP uses a separate non-`Secure` cookie; it does not make
raw save identifiers, inventory contents, or progress available through public
endpoints. Exact-origin validation, CSRF checks, self-only routes, and no-store
responses remain enforced in both modes. Trust forwarding headers only from
explicitly configured proxy CIDRs, apply edge request limits, and keep the save
mount and secret file read-only to the container.
