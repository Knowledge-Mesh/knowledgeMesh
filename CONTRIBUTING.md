# Contributing to knowledgeMesh

Thanks for your interest in contributing. knowledgeMesh is an open-source marketplace-style mesh for AI inference (buyers ↔ control pane ↔ sellers, libp2p QUIC overlay), and it grows faster when contributors help with everything from typo fixes to new transports.

This guide collects the conventions that keep the codebase coherent. If anything here is unclear, contradicts the code, or is out of date, that itself is a useful issue to file.

## Table of contents

- [Code of conduct](#code-of-conduct)
- [Where to ask questions](#where-to-ask-questions)
- [Reporting bugs](#reporting-bugs)
- [Reporting security issues](#reporting-security-issues)
- [Suggesting enhancements](#suggesting-enhancements)
- [Improving documentation](#improving-documentation)
- [Your first code contribution](#your-first-code-contribution)
- [Development environment](#development-environment)
- [Project layout discipline](#project-layout-discipline)
- [Coding style](#coding-style)
- [Tests](#tests)
- [Commit messages](#commit-messages)
- [Pull requests](#pull-requests)
- [Code review](#code-review)
- [Licensing](#licensing)
- [Release process](#release-process)
- [Maintainers](#maintainers)

## Code of conduct

This project follows the [Contributor Covenant](https://www.contributor-covenant.org/version/2/1/code_of_conduct/). Be respectful, assume good faith, and prefer technical discussion over personal critique. A formal `CODE_OF_CONDUCT.md` will be added to the repository — until that lands, this paragraph is the standard.

To report unacceptable behavior, contact the maintainers (see [Maintainers](#maintainers)). Reports are confidential.

## Where to ask questions

- **Operational and setup questions** — read [`AGENTS.md`](./AGENTS.md) (agent runbook with three onboarding paths), [`README.md`](./README.md) (human quickstart), and [`ARCHITECTURE.md`](./ARCHITECTURE.md) (system design + Mermaid diagrams). If your question is not answered there, that is a documentation issue worth filing.
- **General discussion** — open a [GitHub Discussion](https://github.com/Knowledge-Mesh/knowledgeMesh/discussions) when enabled, or a low-priority issue.
- **Real-time chat** — none yet. Coming when the community grows.

Please keep discussion in the repository so others benefit; avoid private DMs for general questions.

## Reporting bugs

Before filing, search [open and closed issues](https://github.com/Knowledge-Mesh/knowledgeMesh/issues?q=is%3Aissue) to avoid duplicates.

A good bug report includes:

1. **What happened** — exact command, exact output, in code blocks.
2. **What you expected** — the contract you believed you were operating under.
3. **Reproduction steps** — minimal commands a maintainer can paste verbatim.
4. **Environment** — `go version`, OS + arch, branch and commit hash (`git rev-parse --short HEAD`).
5. **Logs** — relevant log lines (run with `KM_P2P_DEBUG=1` for libp2p problems). Redact secrets.
6. **Scope** — the binary involved (`buyer`, `seller`, `control`, `relay`, `knowledgeMesh`) and the path you are on (mock sandbox, hosted public mesh, or own local mesh — see [`AGENTS.md`](./AGENTS.md) Path 1 / 2 / 3).

If a bug is reproducible against the hosted public mesh (`http://control.p2pinfer.cloud:8090`), say so explicitly — that affects whether the fix is in code or in operations.

## Reporting security issues

**Do not open a public issue for a security vulnerability.** Contact the maintainers privately (see [Maintainers](#maintainers)). A formal `SECURITY.md` with a private disclosure address and PGP key will follow.

Things that warrant private disclosure:

- Authentication or authorization bypasses (control HTTP API, JWT issues).
- Crash-on-input that can be triggered by a remote peer over libp2p.
- Anything affecting the integrity of the `inference_matches` or `billing_transactions` rows.
- Secrets accidentally committed on any branch.

Maintainers aim to acknowledge security reports within a few business days and ship a fix or mitigation as fast as severity demands.

## Suggesting enhancements

Open an issue with:

- **The problem** you are trying to solve. A user story is fine.
- **Why existing features cannot cover it.**
- **A rough proposal** — pseudocode, sketch CLI usage, or a design paragraph.
- **Blast radius** — schema migration? new wire protocol? public API change in `pkg/`?

Discuss before sending a large PR. Maintainers may suggest splitting it, deferring it, or trying a different shape; that conversation belongs in the issue, not in the PR thread.

## Improving documentation

Documentation PRs are very welcome and don't require advance discussion. Files to know:

| File | Purpose | Audience |
|---|---|---|
| [`README.md`](./README.md) | Quickstart, what the project is | Humans skimming |
| [`AGENTS.md`](./AGENTS.md) | Onboarding runbook with three flows | Agents and ops |
| [`ARCHITECTURE.md`](./ARCHITECTURE.md) | Components, request flow, billing, Mermaid diagrams | Developers |
| `docs/SETUP.md`, `docs/SELLER_GUIDE.md`, `docs/BUYER_GUIDE.md` | Role-scoped guides (on `fix/docs` and derivatives) | Operators |
| Per-package godoc on exported identifiers | API contracts | Developers calling the package |

When you change behavior, update the relevant docs in the same PR. Out-of-sync docs cause more support load than missing docs do.

## Your first code contribution

Looking for somewhere to start? Filter issues by labels (when present): `good first issue`, `help wanted`, `documentation`. The lowest-risk first PRs are documentation fixes, additional tests for existing behavior, typo fixes in user-facing strings, and small refactors of internal helpers that don't change a public API.

## Development environment

**Required:**

- Go **1.24.6 or later** (see [`go.mod`](./go.mod) for the exact module declaration).
- Git.

**For the full local mesh path:**

- PostgreSQL 14+.
- [Ollama](https://ollama.com) with at least one model pulled (e.g. `ollama pull llama3`).
- Optional: Docker (the relay binary ships a `Dockerfile.relay`).

**Recommended toolchain:**

- `gofmt`, `goimports`, `go vet` (ship with the Go toolchain).
- [`golangci-lint`](https://golangci-lint.run) — linter aggregator; will be wired into CI.
- [`staticcheck`](https://staticcheck.dev) — included via `golangci-lint`.

**End-to-end setup:** [`AGENTS.md`](./AGENTS.md) is the source of truth. It documents three paths (buyer with defaults, seller with defaults, own mesh) and is kept in sync with the code.

**Module-path note:** `go.mod` declares `module github.com/knowledgemeshgrid/knowledgemesh` while the repository lives at `github.com/Knowledge-Mesh/knowledgeMesh`. Neither path resolves for `go install` today; clone and `go build` locally instead. Resolving this mismatch is tracked separately.

## Project layout discipline

```
cmd/        # thin main packages — one per binary (buyer, seller, control, relay, knowledgeMesh, demo)
internal/   # private logic — buyer, seller, control, mesh, matchmaker, network, api, sandbox, policy, relay, state
pkg/        # shared, importable types — types, protocol, config
tests/      # integration tests (e.g. tests/network/ for libp2p)
configs/    # operational config templates
docs/       # role-scoped guides
```

Rules:

- **`cmd/`** holds only `main.go` files — no business logic. The pattern is `cmd/<binary>/main.go` ≤ ~100 lines, calling into `internal/`.
- **`internal/`** is private to this module. Other Go projects cannot import it. Use this for anything that may change shape.
- **`pkg/`** is the *stable* surface. Adding to `pkg/` is a public API commitment; removing or renaming an exported identifier is a breaking change. Be conservative.
- **Keep buyer, seller, and control concerns decoupled.** `internal/buyer` should not import `internal/seller`; both share via `pkg/types` or `internal/network` for protocol pieces.

When in doubt, put new code in `internal/`. Promoting to `pkg/` is cheap; demoting is breaking.

## Coding style

- **Formatting** — `gofmt -s -w .` plus `goimports -w .` for import management. PRs that only run formatters are welcome and will be merged quickly.
- **Vet** — `go vet ./...` must pass.
- **Naming** — follow [Effective Go](https://go.dev/doc/effective_go); exported names start with capitals; avoid stutter (`buyer.Buyer` is wrong, `buyer.Service` is right); avoid Hungarian prefixes.
- **Errors** — wrap with `fmt.Errorf("doing X: %w", err)` to preserve cause chains. No silent error swallowing. Use sentinel errors (`var ErrFoo = errors.New(...)`) so callers can `errors.Is` against them.
- **Logging** — `log.Printf` is acceptable for now; structured logging will move to a single package later. Don't introduce a new logger without discussion.
- **Concurrency** — pass `context.Context` as the first parameter of any function that does I/O or may block. Honor cancellation; don't fabricate background contexts in library code.
- **Globals** — avoid them. The known exceptions are embedded migrations and CLI command registrations.

## Tests

Tests are required for new behavior. Aim for:

- **Unit tests** in the same package: `foo_test.go` next to `foo.go`. Use a `_test` package suffix only when testing through the public API.
- **Integration tests** under [`tests/`](./tests) — for cross-package flows (e.g. [`tests/network/`](./tests/network) covers libp2p connection typing, hole punching, message routing, and P2P debug).
- **No test depends on a remote service or the hosted public mesh.** Stub via `httptest.Server` or in-process fakes.
- **Table-driven tests** preferred for input/output exercises.
- **Don't break existing tests** to make new ones pass without explanation in the PR description.

Run before pushing:

```bash
gofmt -s -w .
goimports -w .
go vet ./...
go build ./...      # catches compile errors in internal/ packages no cmd/ imports
go test ./...
```

`go build ./cmd/...` only checks packages reachable from a `main`; `go build ./...` is stricter and is the canonical build check.

CI will be added; until then, the burden of running these locally rests with the contributor.

## Commit messages

Follow the [Tim Pope format](https://tbaggery.com/2008/04/19/a-note-about-git-commit-messages.html):

```
Short imperative subject, ≤ 72 characters

Body that explains *why* this change exists, not what the diff already
shows. Wrap at 72 columns. Use as many paragraphs as you need.

Reference issues with "Fixes #123" or "Refs #123" so GitHub auto-links.
```

- **Imperative subject** — "Add health check" not "Added health check" or "Adds health check".
- **One concern per commit** — easier to bisect, easier to revert.
- **Mention the affected area** when not obvious: `matchmaker: emit decision rationale`, `seller: fix Ollama URL warning`, `docs: clarify default --control-url`.
- **Conventional Commits** (`feat:`, `fix:`, `docs:`, `refactor:`, etc.) are welcome but not required. If you use them, be consistent within a PR.

## Pull requests

**Before opening:**

1. Open an issue for non-trivial changes and discuss the approach. Drive-by 500-line PRs are usually rejected for refactor-only reasons.
2. Branch from `main` (or the relevant feature branch). Use a descriptive name: `feat/matchmaker-decision-rationale`, `fix/seller-ollama-url`, `docs/contributing-rewrite`.
3. Run the local checks above.
4. Update relevant documentation in the same PR.

**Opening:**

- **Title** — same rules as the commit subject (imperative, ≤ 72 chars).
- **Body** — describe what and why; the diff shows how. Sections that help reviewers: *Motivation*, *What changed*, *Test plan*, *Out of scope*.
- **Link issues** — `Fixes #N` for bug fixes, `Refs #N` for related work.
- **Draft PRs are welcome** — open early to get directional feedback.
- **Self-review** — read your own diff before requesting review.

**During review:**

- Address every comment, even if your response is "I disagree because …". Don't let comments rot.
- Force-pushes during review are fine if you keep the PR thread useful (e.g. `git rebase` to clean up). Some maintainers prefer fixup commits; ask if unsure.
- Keep the PR scope stable. New asks → new PRs.

**Before merge:**

- Squash if asked. Fixup commits ("address review", "fix typo") are usually squashed; meaningful commits in a multi-commit feature are usually kept.
- Make sure the final commit message is good. The PR title is *not* automatically the commit message — set the squash message explicitly when squash-merging.

## Code review

Maintainers review PRs as bandwidth allows. Reasonable expectations:

- A first response should arrive within a week of opening; ping politely if it doesn't.
- Small, well-scoped PRs merge faster than large ones.

What reviewers look for, in rough priority order:

1. Correctness — does it do what the description claims?
2. Tests — does the test cover the new path? Does it break old paths?
3. Architecture — does it respect `cmd/` / `internal/` / `pkg/` discipline? Does it cross unwanted package boundaries?
4. Style — gofmt, naming, error wrapping.
5. Documentation — README / AGENTS / ARCHITECTURE / godoc updated where behavior changed.

## Licensing

This project is licensed under the [Apache License 2.0](./LICENSE). By contributing, you agree your contribution is licensed under Apache-2.0.

A Developer Certificate of Origin (DCO) sign-off may be required in a future commit policy. Until then, no `Signed-off-by:` line is required, but if you include one (`git commit -s`) it will be respected.

We do not currently require a CLA.

## Release process

Versioning, changelog management, and release tooling are under construction. When formalized, this section will link to `RELEASING.md`. Until then, releases are tagged from `main` by maintainers.

If your PR introduces a behavior change worth flagging in a future changelog, mention it in the PR description under a `## Release notes` heading. That makes the changelog easier to assemble later.

## Maintainers

The current maintainer list will live in `MAINTAINERS.md` once that file exists. Until then, the GitHub repository's [contributors graph](https://github.com/Knowledge-Mesh/knowledgeMesh/graphs/contributors) is the de facto list. For private channels (security, conduct), reach the maintainers via their GitHub profile contact information; dedicated addresses will be published as the community grows.

---

Thanks again. The shortest path to landing your first PR is: pick a small thing, follow this guide, ask for early feedback, ship it.
