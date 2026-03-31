# Contributing to knowledgeMesh

Thanks for contributing.

## Basic Guidelines

- Open an issue before major changes to align on approach.
- Keep `cmd/` thin; place reusable and core logic in `internal/` or `pkg/` as appropriate.
- Prefer small, focused pull requests with clear descriptions.
- Add or update tests in `tests/` when behavior changes.
- Run local checks before submitting:

```bash
go test ./...
go build ./cmd/...
```

## Development Notes

- `cmd/` makes each module independently runnable.
- `internal/` keeps implementation private and easier to change.
- `pkg/` holds shared types that can be reused safely.
- `internal/` packages are private to this module.
- `pkg/` is for stable, shared interfaces and types intended for external use.
- Keep buyer, seller, and control concerns decoupled to support independent iteration.
