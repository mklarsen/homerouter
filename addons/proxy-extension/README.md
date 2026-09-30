# homerouter.io browser extension

This addon contains the Chromium extension source for the Homerouter authenticated proxy.

## Development

- Load this directory as an unpacked extension from `chrome://extensions`.
- The extension version is defined once in `manifest.json`; the popup reads it dynamically.
- Proxy settings are stored in the extension's local storage. Never commit credentials.

## Credits and license

Developed by MKLarsen. The popup links to the [Homerouter project](https://github.com/mklarsen/homerouter) and credits MKLarsen. This addon is covered by the repository's root `LICENSE`.
