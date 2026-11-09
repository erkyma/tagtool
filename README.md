# tagtool

Stupid simple semver git tags. You pick the kind of release, `tagtool` works out the number from the
existing tags, shows you the commit and asks before tagging.

```
$ tt
latest final   6.6.1
open line      6.7.0-beta.2
HEAD           a1b2c3d "Fix payroll rounding"

6.7.0-beta.2 → 6.7.0-beta.3  @ a1b2c3d "Fix payroll rounding"
[y/N]
```

## Install

```
go install github.com/erkyma/tagtool@latest
```

Or download a binary from [releases](https://github.com/erkyma/tagtool/releases).

The examples use `tt` as shorthand. Add an alias to `~/.zshrc`:

```
alias tt=tagtool
```

Zsh completion (after `compinit`):

```
source <(tagtool completion zsh)
```

## Usage

```
tt [scope] [stage] [flags]
```

| Stage | | Example |
|---|---|---|
| `a`, `alpha` | early pre-release | `6.7.0-alpha.1` |
| `b`, `beta` | pre-release | `6.7.0-beta.3` |
| `rc` | release candidate | `6.7.0-rc.1` |
| `f`, `final` | closes the open line | `6.7.0` |

| Scope | | From `6.6.1` |
|---|---|---|
| `ma`, `major` | breaking change | `7.0.0` |
| `mi`, `minor` | new feature | `6.7.0` |
| `p`, `patch` | bugfix | `6.6.2` |

Without a scope, a stage continues the open pre-release line, or starts a new minor line if the latest
tag is a final. Without a stage, a scope tags a final directly. With no arguments, `tt` shows the status
and offers the next pre-release on the current line; it never suggests a final.

| Flag | |
|---|---|
| `-l, --list [N]` | list the N highest tags with their commits (default 5) |
| `-r, --at <ref>` | tag `<ref>` instead of HEAD |
| `-p, --push` | push the new tag to origin |
| `-n, --dry-run` | show what would be tagged, change nothing |
| `-y, --yes` | skip confirmation (finals always ask) |
| `-o, --offline` | skip `git fetch --tags` before resolving |
| `-v, --version` | print the version |

### Examples

With latest final `6.6.1` and open line `6.7.0-beta.2`:

| Command | Tag | |
|---|---|---|
| `tt b` | `6.7.0-beta.3` | next beta on the open line |
| `tt rc` | `6.7.0-rc.1` | promote the open line to release candidate |
| `tt f` | `6.7.0` | release the open line |
| `tt p` | `6.6.2` | hotfix on the latest final |
| `tt p b` | `6.6.2-beta.1` | beta of a hotfix |
| `tt ma b` | `7.0.0-beta.1` | start a new major line |
| `tt b -n` | | preview without tagging |
| `tt f -r abc123` | `6.7.0` | release a specific commit |

## Safety

- **Never goes back:** no beta after an rc, and no pre-release or second final once `X.Y.Z` is released.
  Version lines are independent, so a `6.6.2` hotfix works while `6.7.0-beta.x` is open.
- Fetches tags before resolving, so a colleague's newer tag isn't missed.
- Refuses dirty working trees and commits that already carry a version tag.
- Finals always ask for confirmation, even with `-y`.

Tags are annotated, with or without a `v` prefix; new tags follow the prefix of the latest one. In a
repo with no version tags, `tt` asks once whether to use the `v` prefix.

## Development

```
task test      # run tests
task build     # build to bin/tagtool
task install   # go install
task shell     # add the tt alias and zsh completion to ~/.zshrc
```

Pushing a final tag (`v1.2.3`) builds binaries for macOS and Linux and publishes a GitHub
release. Pre-release tags only run the checks.
