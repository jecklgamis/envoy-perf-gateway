# Contributing to envoy-perf-gateway

Thanks for considering a contribution. This project is small and the
maintainer is one person, so keeping changes focused and well-tested
matters more here than in a large project with more reviewers to catch
things.

## Ways to contribute

- **Bug reports** - open an [issue](https://github.com/jecklgamis/envoy-perf-gateway/issues).
  Include what you ran, what you expected, and what actually happened.
  For anything involving the gateway/fetcher/config server, the exact
  `gatewayctl` command and, if relevant, `CONFIG_SOURCE_KIND` (`http` or
  `s3`) save a lot of back-and-forth.
- **Feature requests** - also an issue. A short description of the use
  case is more useful than a fully-specified design; this is a perf/chaos
  testing tool, so context on what you're trying to test helps a lot.
- **Pull requests** - see below.
- **Security issues** - please don't open a public issue for anything
  that looks like a real vulnerability (e.g. a way to write outside an
  intended directory, bypass the config server's token check, or similar).
  Use GitHub's [private vulnerability reporting](https://github.com/jecklgamis/envoy-perf-gateway/security/advisories/new)
  instead.

## Project layout

Four independent Go modules, one repo:

- `gatewayctl/` - the CLI. Own `go.mod`/`Makefile`; build/test/run it from
  that directory, not the repo root.
- `config_server/` - the small HTTP config-distribution service.
- `fetcher/` - runs inside the gateway container, polls `config_server`
  or S3, writes into `/etc/envoy/dynamic`.
- `app/` - `default_app`, the bundled echo server.

Plus `charts/` (two standalone Helm charts), `docs/index.html` (the
published User Guide - most user-facing behavior is documented there, not
in this repo's markdown), and `docs/architecture.md`. `CLAUDE.md` has a
more detailed architectural writeup, including some non-obvious gotchas
(Envoy's `disk_layer` symlink-swap requirement, a compressor
`typed_per_filter_config` quirk) worth reading before touching
`gatewayctl/internal/render` or `envoyconfig`.

## Building and testing

```bash
# gatewayctl
cd gatewayctl && go build ./... && go test ./...

# gateway / config server images
make all                      # gateway image
make -C config_server image   # config server image
```

**If your change touches `gatewayctl/internal/render`,
`gatewayctl/internal/envoyconfig`, `config/envoy.yaml`, or the
`Dockerfile`**, also run the integration test:

```bash
make integration-test
```

This boots a real Envoy against the rendered config and checks Envoy
actually *accepted* it (`cds`/`lds` `update_rejected`/`update_failure` are
`0`), not just that it compiled and marshaled to valid-looking YAML. That
distinction matters: a wrong Envoy proto message shape produces YAML that
looks fine and still gets silently rejected at listener-load time, with
no error anywhere except Envoy's own admin API - this has actually
happened during development (see `CLAUDE.md`'s compressor writeup), and
`go test` alone would not have caught it.

Both `go test ./...` and `make integration-test` run in CI on every PR
that touches the relevant paths - it's fine to let CI be your first real
run of the integration test if Docker locally is inconvenient, but if you
can run it locally first, a failure is much faster to debug with the
container still up than from a CI log.

## Code conventions

- **No comments that explain *what* the code does** - identifiers should
  do that. Comments here are reserved for *why*: a non-obvious constraint,
  a workaround for a specific Envoy/Go quirk, an invariant that would
  surprise a reader. If you'd remove a comment without losing anything,
  don't add it.
- **Config is generated from typed Go structs (`envoyconfig.go`), not
  string templates.** This is deliberate - see `render.go`'s package
  comment. If you're adding a new Envoy filter/extension, add its shape
  as structs there rather than building YAML strings, and prefer reusing
  an existing pattern (e.g. `FaultPerRoute`'s "global default + per-route
  `typed_per_filter_config` override" shape) over inventing a new one.
- **Don't add a flag/feature "for completeness"** - this CLI's flags map
  directly to real, load-bearing Envoy config knobs (see how `--http2`,
  `--compression`, `--host-header` were each added because something
  concrete needed them). If you're not sure a flag is needed yet, it
  probably isn't.
- **New CLI flags on `add-backend`/`fault`** generally want three things:
  a validator in `cmd/root.go` (see `validatePort`/`validatePercent`/etc.
  for the pattern), a unit test for the render-side behavior, and a
  mention in `docs/index.html`'s Routing/Fault Injection section - the
  User Guide is the primary place users look, not this repo's markdown.

## Commit messages and PRs

Look at `git log` for the tone this project uses: imperative mood
("Add", "Fix", "Guard", not "Added"/"Fixes"), a short summary line, and a
body that explains *why* a change was made and, for anything non-trivial,
*how it was verified* - "added a test" is less useful than "confirmed
against a real Envoy instance that X now returns 0 instead of rejecting
the config."

For a PR:

- Keep it focused. A bug fix doesn't need an accompanying refactor.
- If you touched anything under `gatewayctl/`, run `gofmt -l .` from that
  directory and fix anything it flags (excluding the one pre-existing,
  unrelated issue in `envoyconfig.go` if you see it - not yours to fix as
  part of an unrelated PR).
- Mention in the PR description if you ran `make integration-test`
  locally and what happened, if you didn't rely on CI for it.
- Don't feel obligated to update `CLAUDE.md`/the User Guide for a small
  fix, but do for anything that changes user-facing CLI behavior or the
  config-distribution architecture - both have gone stale in the past
  from real changes not being reflected there.

## License

By contributing, you agree your contribution is licensed under this
project's [Apache License 2.0](LICENSE).
