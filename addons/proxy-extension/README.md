# homerouter.io browser extension

The official Chromium extension for the [homerouter.io authenticated proxy](https://github.com/mklarsen/homerouter). Connect your browser through an encrypted HTTPS proxy without changing system-wide network settings.

## What it does

- Connects and disconnects the browser proxy from a compact toolbar popup.
- Verifies proxy credentials directly over HTTPS before saving them.
- Keeps the proxy disabled until a valid username and password are available.
- Supports a configurable proxy host and port for local or self-hosted deployments.
- Requests server-side closure of active requests and tunnels when you disconnect.
- Shows the current connection state with a green toolbar icon ring while connected and reports configuration or verification errors clearly.

The extension is designed for people who want a quick, explicit browser-level connection to their homerouter.io proxy. It does not change other applications or your operating system's network configuration.

## Getting started

1. Download the versioned ZIP from the [GitHub releases](https://github.com/mklarsen/homerouter/releases) page, extract it, and load the extracted directory from `chrome://extensions` with Developer mode enabled. During development, load this directory as an unpacked extension.
2. Open the extension from the browser toolbar and enter your proxy host, port, username, and password.
3. Test the connection to verify the credentials.
4. Connect when you want browser traffic to use the proxy, and disconnect when you are finished.

The extension requires a proxy password of at least 12 characters. Credentials are saved only after verification succeeds and are kept in Chrome's local extension storage.

## Releases

The manifest version is the source of truth for the release title and ZIP filename. For example, manifest version `1.4.0` is published as `homerouter.io Browser Extension 1.4.0` with the asset `homerouter-extension-1.4.0.zip` and tag `extension-v1.4.0`. Packaged extension changes require a SemVer bump and a matching entry in this directory's `CHANGELOG.md`.

Merges to `main` that change packaged extension files publish a release automatically. Extension releases are not marked as the repository's latest release, which remains reserved for the Homerouter router installer.

## Privacy and security

- Proxy credentials are sent only to the configured homerouter.io proxy verification and disconnect endpoints.
- Credentials are stored locally in Chrome and are never included in the project source code.
- The extension does not collect analytics, browsing history, or personal information.
- The `<all_urls>` permission is required because Chrome's proxy and authentication APIs operate across the browser's network requests.
- A forced browser or operating-system shutdown cannot guarantee that the optional server-side disconnect request is delivered.

## Permissions

| Permission | Why it is needed |
| --- | --- |
| `proxy` | Apply and clear the browser proxy configuration. |
| `storage` | Store the proxy configuration locally. |
| `webRequest` | Handle proxy authentication challenges. |
| `webRequestAuthProvider` | Supply verified proxy credentials to Chrome. |
| `tabs` | Coordinate connection state with browser tabs. |
| `<all_urls>` | Allow proxy authentication and network handling for any destination. |

## Development

- The extension version is defined in `manifest.json`; the popup reads it dynamically.
- Run the extension tests with `node --test background.test.cjs` from this directory.
- Packaged extension changes must bump the manifest version and add matching notes to `CHANGELOG.md`. Merging those changes to `main` publishes a ZIP named `homerouter-extension-<version>.zip` in a GitHub release titled `homerouter.io Browser Extension <version>`.
- Extension release tags use the `extension-v<version>` namespace and are not marked as the repository's latest release; the router installer continues to use the latest router release.
- Never commit real proxy credentials.

## Credits and license

Developed by MKLarsen. The extension is part of the [Homerouter project](https://github.com/mklarsen/homerouter) and is covered by the repository's root `LICENSE`.
