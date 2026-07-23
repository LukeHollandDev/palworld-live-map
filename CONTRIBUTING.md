# Contributing

Thanks for helping improve Palworld Live Map. Keep changes focused, easy to review, and safe for operators exposing the read-only site publicly.

## Development Workflow

1. Create a branch from `main`.
2. Make one cohesive change per pull request.
3. Run `make check`; run `make image` when changing the production container and `make exporter-check` when changing the Palworld Asset Exporter.
4. Update documentation, examples, and tests with behavior or configuration changes.
5. Open a pull request that explains the problem, the chosen approach, and how it was verified. Include screenshots for visible UI changes.

Do not commit credentials, private server responses, player identifiers, IP addresses, save data, or local environment files. Public API changes must continue to exclude REST credentials and upstream account/network identifiers.

## Commits

Use small, atomic commits with an imperative Conventional Commit subject:

```text
type(optional-scope): concise summary
```

Common types are `feat`, `fix`, `docs`, `refactor`, `test`, `build`, `ci`, and `chore`. Examples:

```text
feat(map): expose server description
fix(poller): retain metadata during upstream outages
docs: explain reverse-proxy deployment
```

Keep formatting-only or generated-file updates separate from behavioral changes when that makes the history easier to review. Rebase or squash fixup commits before requesting review.

## Pull Requests

A pull request should:

- have a clear, descriptive title using the same style as commit subjects;
- link its issue when one exists;
- describe user-visible and operational impact;
- call out security, privacy, compatibility, or deployment considerations;
- include or update tests for changed behavior; and
- pass frontend and Go checks, the asset-exporter fixtures/compile, and the gated container build in CI.

Reviewers may ask for a PR to be split when it mixes unrelated concerns. Draft pull requests are welcome for early design or compatibility feedback.
