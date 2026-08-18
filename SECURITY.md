# Security

Please report vulnerabilities privately through GitHub's private vulnerability reporting rather than opening a public issue.

Do not expose the Palworld REST API or its credentials publicly. REST Basic Auth stays in the map backend, and upstream account IDs and IP addresses are not included in its public data model.

This map does not provide general viewer authentication. Anyone who can access it can see online player names and positions, plus any enabled world-object layers. Restrict access to the map if that information should not be public.

The optional saved-character connection authenticates only self-private routes;
it is not an access-control layer for the live map. Its primary proof is a
one-shot, one-question memory quiz generated from a safely completed backup.
The question exposes three to eight real item or Pal names from the answer's
saved container and can be replaced before answering. Starting or cycling a
challenge can therefore disclose this bounded set before identity is verified.
Questions may use the first two rows of common
inventory, equipment, food, or party slots. Dropped-item and key-item containers
are excluded. A wrong submission
consumes the challenge, and existing per-client and global limits bound guessing and private
decoder work. Correct option indexes, counts, dynamic instance IDs, raw item IDs, raw
save identifiers, proof evidence, and completion state keys must never be
returned by an API or written to logs. Challenges and
sessions are bounded and stored hash-at-rest. The browser receives a random
session bearer after a correct answer and keeps it only in page memory; it is
not persisted in cookies, browser storage, or URLs. Reloading requires another
check. If no supported group has three distinct choices, connection is refused
until the character changes that saved inventory, equipment, food, or party state.

Enabling claims does not change the existing public player endpoints: their
online and offline rosters, saved positions, levels, guild relationships,
last-seen values, and aggregate progression remain public. Only the new exact
per-landmark completion response is authenticated, self-only, no-store, and
projects private save keys to already-public catalogue IDs. HTTP and raw-IP
deployments are supported without origin configuration. HTTPS remains strongly
recommended on public or untrusted networks because HTTP cannot prevent bearer
or completion-response interception. Keep the save mount read-only and apply
edge request limits where the map is publicly reachable.
