# Security

Please report vulnerabilities privately through GitHub's private vulnerability reporting rather than opening a public issue.

Do not expose the Palworld REST API or its credentials publicly. REST Basic Auth stays in the map backend, and upstream account IDs and IP addresses are not included in its public data model.

This map does not provide viewer authentication. Anyone who can access it can see player names and positions, plus any enabled world-object layers. With save support enabled, that can include offline players, guild membership, last-saved positions, levels, last-seen times, lifetime Pal captures, distinct Pals caught, and Paldeck unlock counts. Restrict access to the map if that information should not be public.

Mount only `Pal/Saved/SaveGames/0`, always read-only. The reader rejects symlinked snapshot artifacts and never writes to the game tree or invokes `/save`. Mermaid decompression now runs inside the long-lived Go service alongside the existing selective GVAS parser, with explicit input, output, collection, and nesting limits. Those limits reduce resource-exhaustion risk but are not a sandbox: a decoder defect, panic, or excessive CPU use can affect the whole service process.

`SAVE_TIMEOUT` bounds the surrounding snapshot operation, but it cannot forcibly preempt a decompression call already executing in-process. Invalid or unsupported save data fails the refresh without replacing the last good roster. The container still runs as a non-root user; use a read-only save mount, keep resource limits appropriate for the deployment, and do not expose untrusted users to a writable save path.

The project does not distribute or load Epic or RAD Game Tools' proprietary Oodle runtime or source. Its Oodle-compatible Go decoder has unresolved upstream copyright provenance documented in [LICENSING.md](LICENSING.md); security review does not resolve that licensing question.
