# Versioning

The version is the `VERSION` file, the heading in `CHANGELOG.md`, and a
git tag `vX.Y.Z` on the release commit. All three must match. `go install`
and GitHub Releases use the tag, not the file.

Until 1.0, the first number stays 0.

- **0.x.0** when we add a command, an HTTP/MCP field, or a documented
  behaviour people will rely on.
- **0.x.y** for fixes and docs that do not change that surface.
- Tag every release. Push the tag with the commit.
- Do not retag. If the tag is wrong, cut the next patch.

## What 1.0 means

1.0 is a compatibility promise, not a feeling that the product is done.

**Stable after 1.0** (a break is 2.0, or a documented exception):

- MCP tool names and their required fields
- HTTP `/v1` paths and required JSON fields
- CLI verbs: `init`, `serve`, `mcp`, `skill`, `config`, `owner`, `ledger`,
  `token`
- `~/.ledger/config` keys: `url`, `token`, `owner`, `ledger`
- Token shapes: owner-scoped admin, ledger-bound write, operator

Adding optional fields, new tools, or new CLI verbs is allowed in 1.x.
Renaming or removing the list above is not.

**Must be true before we cut v1.0.0:**

| # | Gate | Status |
| --- | --- | --- |
| 1 | Every shipped `VERSION` has a matching `v*` tag on `main` | Done |
| 2 | GitHub Releases for darwin/arm64, linux/amd64, windows/amd64. Getting Started can say download the binary or `go install …@v1.0.0` | Workflow on tag (`release.yml`). First artefacts ship with the next `v*` tag |
| 3 | Local loop proven: `make smoke` exits 0 (`scripts/smoke.sh`: init, serve, create, claim, note, close). MCP uses the same app methods; the HTTP loop is the proof | Protocol is `make smoke` |
| 4 | Hosted signup and at least one extra ledger used for real work | In progress (operator) |
| 5 | We are willing to stop renaming the stable list above | Done |

**Not required for 1.0:**

- MCP OAuth for ChatGPT / Claude on the web
- GitHub or magic-link login on self-host
- Homebrew, winget, or apt
- A guarantee that hosted will run forever

When 2 has a published release, 3 has a green `make smoke` on a laptop, and
4 is ticked, bump to 1.0.0, tag `v1.0.0`, and say so in the changelog.
