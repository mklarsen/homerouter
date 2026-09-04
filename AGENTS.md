# AGENTS.md

Context for AI coding agents working in this repository. Read this before
touching anything; it encodes decisions that are not obvious from the code and
mistakes that have already been made once.

## What this repo is

`homerouter` turns an Ubuntu 24.04 LTS box into a router, firewall and Wi-Fi
access point. It is **configuration + Bash**, no application code. The target
machine is not this machine — the development host is Windows/WSL, so nothing
here can be run end-to-end locally. Validation happens through rendering and
syntax checks, not by starting services.

## Architecture in one paragraph

Everything site-specific lives in a git-ignored `.env`. `setup.sh` sources it
through `load_env` (in `scripts/lib.sh`), renders `config/**/*.template` with
`render_template` (envsubst with an **explicit** variable list), validates the
output, installs it with `install_managed` (content-compare, backup, chmod), and
restarts only the services whose files actually changed. `scripts/verify.sh`
checks the running result; `scripts/selftest.sh` does the offline half and is
what CI runs.

## Hard rules

1. **Never commit real network values.** No public IPs, no delegated prefixes,
   no passphrases. `.env.example` uses RFC 5737 / RFC 3849 documentation
   addresses. CI greps tracked files for the reference deployment's addresses
   and fails on a hit.
2. **Never push to `main`.** It is protected by a ruleset: pull request required,
   `lint & render` must pass, no force-push, no deletion. Use
   `gh pr create` → `gh pr checks <n> --watch` → `gh pr merge <n> --squash --delete-branch`.
   GitHub Actions cannot bypass the ruleset (it was tried; a personal repo
   rejects an `Integration` bypass actor), so no workflow may push commits.
3. **Keep scripts ShellCheck-clean.** CI runs
   `shellcheck -x setup.sh install.sh scripts/*.sh` and fails on warnings.
4. **Keep `setup.sh` idempotent.** A second run on unchanged input must report
   no changes and restart nothing.
5. **Firewall stays default-DROP.** New capabilities are added as commented
   examples, never enabled by default.

## Things that will bite you

- **envsubst name collisions.** `render_template` substitutes both `$VAR` and
  `${VAR}`. nftables uses the same syntax for its own `define`s, so the nft
  define names must not match any name in `TEMPLATE_VARS`. That is why the
  ruleset uses `$WAN`, `$LAN`, `$NET4`, `$SRVNET4` instead of the env names.
- **Optional features use comment-gating.** `SRV_CMT` is `""` or `"# "`, and
  every line of the optional server segment is prefixed with it. This keeps one
  template per file and makes a disabled feature visible as comments in the
  installed file. `SRV_LAN_CMT` gates a single rule the same way.
- **ShellCheck SC2034** fires on variables that only exist to be exported into
  templates. Add `# shellcheck disable=SC2034` above the assignment; do not
  delete the variable.
- **`set -e` and `A && B || C`.** Rewrite as `if`/`else`; SC2015 fails CI, and
  the construct silently misbehaves.
- **Empty optional values.** `render_template` deletes netplan list entries that
  rendered as bare `- /24`. nftables `define` cannot be empty, so
  `NFT_IP4_SECONDARY`/`NFT_IP4_TERTIARY` fall back to the primary address.
- **Windows line endings.** Git reports CRLF→LF warnings on commit; `.gitattributes`
  handles it, ignore the noise. Do not "fix" files by rewriting line endings.
- **`python3` on the Windows dev host** is the Microsoft Store stub. The netplan
  YAML check is skipped locally and only really runs in CI — do not conclude
  from a local pass that the YAML is valid.

## Workflow for a change

```bash
git checkout -b feat/thing
# edit template + .env.example + scripts/lib.sh (TEMPLATE_VARS) as needed
bash scripts/selftest.sh            # renders both SRV states, validates output
# add a bullet under "## [Unreleased]" in CHANGELOG.md
git commit && git push -u origin feat/thing
gh pr create --base main ...
gh pr checks <n> --watch --fail-fast
gh pr merge <n> --squash --delete-branch
```

Adding a configuration variable means touching four places: `.env.example`, the
defaults in `load_env`, the `TEMPLATE_VARS` array, and the template itself.
Forgetting `TEMPLATE_VARS` leaves a literal `${VAR}` in the output — the
self-test catches that.

## Releases

Driven by `CHANGELOG.md`. `release.yml` reads the newest
`## [x.y.z] - YYYY-MM-DD` heading; if that tag is not yet released it builds
`homerouter-vx.y.z.tar.gz` + `SHA256SUMS` and publishes the section as the
release notes. A merge without a new version heading publishes nothing. To cut a
release, rename `## [Unreleased]` to the version and add a fresh empty
`## [Unreleased]` above it, in the same pull request.

`install.sh` resolves `releases/latest` for the
`curl … | sudo bash` one-liner and verifies the SHA-256 checksum.

## Repository map

| Path | Purpose |
| --- | --- |
| `setup.sh` | idempotent deploy: preflight, render, validate, install, restart |
| `install.sh` | bootstrap from a published release into `/opt/homerouter` |
| `scripts/lib.sh` | `load_env`, `render_template`, `install_managed`, logging |
| `scripts/selftest.sh` | offline render + validate, both segment states (CI) |
| `scripts/verify.sh` | runtime health checks against the live system |
| `scripts/changelog.sh` | `version` / `notes` / `check` for the release workflow |
| `config/**/*.template` | the actual router configuration |
| `.github/ruleset-main.json` | the branch protection, tracked as code |

## Current state

Released through `v0.2.0`. The optional server/DMZ segment (`SRV_ENABLE`) is
implemented and validated but has never run on real hardware — treat the first
deployment report as the real test. Nothing has been verified on the physical
router yet: no `verify.sh` output from a live box exists.

## Style

Follow `CONTRIBUTING.md`. Comments explain *why*, not *what*. Prefer editing an
existing template over adding files. The user writes in Danish and English —
answer in the language they used; keep code, comments and docs in English.
