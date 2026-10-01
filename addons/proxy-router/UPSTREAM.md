# Historical GOST attribution

The previous `ghcr.io/mklarsen/homerouter-proxy:3.3.0` image used GOST v3.3.0:

- Project: <https://github.com/go-gost/gost>
- Release: `v3.3.0`
- Pinned commit: `cb76f63754768c7b5d68895a0d51635b0141b80f`
- License: MIT, reproduced in [`LICENSE.GOST`](LICENSE.GOST)

The vendored GOST source and its manual publisher have been removed. The current Homerouter proxy is a separate implementation and does not contain or derive from GOST code. No GOST CI, scheduled build, or update workflow is used.

The GOST license remains in [`LICENSE.GOST`](LICENSE.GOST) for historical attribution.
