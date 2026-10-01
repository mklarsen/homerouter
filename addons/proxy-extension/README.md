# homerouter.io browser extension

This addon contains the Chromium extension source for the Homerouter authenticated proxy.

## Development

- Load this directory as an unpacked extension from `chrome://extensions`.
- The extension version is defined once in `manifest.json`; the popup reads it dynamically.
- Version `1.3.8` requests server-side closure for the active proxy user when **AFBRYD TUNNEL** is used.
- Proxy settings are stored in the extension's local storage. Never commit credentials.
- The proxy is not enabled without a username and a password of at least 12 characters. The extension only saves credentials after a proxy-auth challenge verifies them; a cache-only browser login is not treated as verification.
- The explicit disconnect action asks the proxy server to close active requests/tunnels for the configured username. If that account is used on multiple devices, all its active connections are closed; a forced browser/OS shutdown cannot guarantee sending this request.

## Credits and license

Developed by MKLarsen. The popup links to the [Homerouter project](https://github.com/mklarsen/homerouter) and credits MKLarsen. This addon is covered by the repository's root `LICENSE`.
