# Security

Please report vulnerabilities privately through GitHub's private vulnerability reporting rather than opening a public issue.

Do not expose the Palworld REST API or its credentials publicly. REST Basic Auth stays in the map backend, and upstream account IDs and IP addresses are not included in its public data model.

This map does not provide viewer authentication. Anyone who can access it can see player names and positions, plus any enabled world-object layers. With save support enabled, that can include offline players, guild membership, last-saved positions, levels, last-seen times, lifetime Pal captures, distinct Pals caught, and Paldeck unlock counts. Restrict access to the map if that information should not be public.

Mount only `Pal/Saved/SaveGames/0`, always read-only. The decoder rejects symlinked snapshot artifacts and never writes to the game tree or invokes `/save`. Oodle is proprietary and is not distributed by this project; use only a runtime you are authorized to use, and pin its SHA-256 when using explicit download mode.
