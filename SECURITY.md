# Security

Please report vulnerabilities privately through GitHub's private vulnerability reporting rather than opening a public issue.

Do not expose the Palworld REST API or its credentials publicly. REST Basic Auth stays in the map backend, and upstream account IDs and IP addresses are not included in its public data model.

This map does not provide viewer authentication. Anyone who can access it can see live player names and positions, plus any enabled world-object layers. Restrict access to the map if that information should not be public.
