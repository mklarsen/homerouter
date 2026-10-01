# Upstream source and attribution

The Go proxy executable in this addon is derived from the upstream GOST project:

- Project: <https://github.com/go-gost/gost>
- Release: `v3.3.0`
- Pinned commit: `cb76f63754768c7b5d68895a0d51635b0141b80f`
- License: MIT, reproduced in [`LICENSE.GOST`](LICENSE.GOST)

The `upstream/` directory contains the Go command source, module files, official Dockerfile, and English upstream README required to build GOST. It is a source snapshot, not an official GOST distribution. Upstream `.git` metadata, `.github` CI/nightly workflows, e2e tests, installers, and release tooling are intentionally not included.

Homerouter's image is maintained and published by Martin Kraus Larsen as `ghcr.io/mklarsen/homerouter-proxy`. It is not an official image from the GOST maintainers.
