# Changelog

## [v0.2.1](https://github.com/nu0ma/spanner-readonly-cli/compare/v0.2.0...v0.2.1) - 2026-09-19

- docs: add MIT license by @nu0ma in https://github.com/nu0ma/spanner-readonly-cli/pull/49

## [v0.2.0](https://github.com/nu0ma/spanner-readonly-cli/compare/v0.1.5...v0.2.0) - 2026-09-19

- fix: reject duplicate columns, limit rows, and return JSON errors by @nu0ma in https://github.com/nu0ma/spanner-readonly-cli/pull/47

Compatibility notes:

- All data commands now return at most 100 rows by default. Check `truncated` for incomplete results, or use `--max-rows 0` for unlimited output.
- Duplicate result column names and STRUCT field collisions now return `InvalidArgument` instead of silently overwriting values.
- Errors include `code` and `retryable`. The `error` field remains a string, but message text may change. Usage errors exit with code 2; configuration and execution failures exit with code 1.

## [v0.1.5](https://github.com/nu0ma/spanner-readonly-cli/compare/v0.1.4...v0.1.5) - 2026-09-18

- chore(deps): update remaining Go dependencies by @nu0ma in https://github.com/nu0ma/spanner-readonly-cli/pull/45

## [v0.1.4](https://github.com/nu0ma/spanner-readonly-cli/compare/v0.1.3...v0.1.4) - 2026-09-18

- chore(deps): update Go, dependencies and GitHub Actions by @nu0ma in https://github.com/nu0ma/spanner-readonly-cli/pull/42
- chore(deps): bump cloud.google.com/go/spanner from 1.95.0 to 1.95.1 by @dependabot[bot] in https://github.com/nu0ma/spanner-readonly-cli/pull/36
- chore(deps): bump zizmorcore/zizmor-action from 0.6.3 to 0.6.4 by @dependabot[bot] in https://github.com/nu0ma/spanner-readonly-cli/pull/37
- fix: align command name with go install output by @nu0ma in https://github.com/nu0ma/spanner-readonly-cli/pull/43

## [v0.1.3](https://github.com/nu0ma/spanner-readonly-cli/compare/v0.1.2...v0.1.3) - 2026-07-07

- docs: recommend direnv for connection env vars; ignore .envrc by @nu0ma in https://github.com/nu0ma/spanner-readonly-cli/pull/15

## [v0.1.2](https://github.com/nu0ma/spanner-readonly-cli/compare/v0.1.1...v0.1.2) - 2026-07-07

- docs: remove Spanner Omni section from README by @nu0ma in https://github.com/nu0ma/spanner-readonly-cli/pull/5
- refactor: flatten internal/cli into root package main by @nu0ma in https://github.com/nu0ma/spanner-readonly-cli/pull/7
- ci: verify UPDATE/DELETE rejection with the real binary in CI by @nu0ma in https://github.com/nu0ma/spanner-readonly-cli/pull/8
- security: patch stdlib vulns, add govulncheck and dependabot, pin Omni image by @nu0ma in https://github.com/nu0ma/spanner-readonly-cli/pull/9
- ci: add 7-day cooldown to dependabot updates by @nu0ma in https://github.com/nu0ma/spanner-readonly-cli/pull/13
- chore(deps): bump google.golang.org/api from 0.283.0 to 0.286.0 by @dependabot[bot] in https://github.com/nu0ma/spanner-readonly-cli/pull/14
- chore(deps): bump actions/checkout from 6.0.3 to 7.0.0 by @dependabot[bot] in https://github.com/nu0ma/spanner-readonly-cli/pull/10

## [v0.1.1](https://github.com/nu0ma/spanner-readonly-cli/compare/v0.1.0...v0.1.1) - 2026-07-07

- ci: add build and actionlint jobs to match required status checks by @nu0ma in https://github.com/nu0ma/spanner-readonly-cli/pull/3
- ci: add zizmor and harden workflows by @nu0ma in https://github.com/nu0ma/spanner-readonly-cli/pull/4

## [v0.1.0](https://github.com/nu0ma/spanner-readonly-cli/commits/v0.1.0) - 2026-07-07
