## What this changes

<!-- One or two sentences on the behaviour, not the diff. -->

## Why

<!-- The problem it solves. Link an issue if there is one. -->

## Checks

- [ ] `gofmt -l .` prints nothing
- [ ] `go test ./...` passes
- [ ] `make smoke` passes if this touches the API, MCP, or the CLI
- [ ] A test covers the change, or the reason one is impractical is stated below
- [ ] Docs updated (`README.md`, `docs/`, the skill) if the surface changed
- [ ] `CHANGELOG.md` and `VERSION` bumped if this is a release

## Surface impact

<!-- Does this change anything in the stable list in docs/versioning.md?
     If yes, say what breaks for an already-attached agent. -->
