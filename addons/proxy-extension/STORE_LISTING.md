# Chrome Web Store listing

Use this copy when publishing the extension. Keep the store listing in English.

## Name

homerouter.io

## Short description

Connect to the homerouter.io authenticated HTTPS proxy with verified credentials and one-click tunnel disconnect.

## Detailed description

homerouter.io is the official browser extension for the homerouter.io authenticated proxy.

Connect your Chromium browser through an encrypted HTTPS proxy directly from the toolbar. The extension verifies your proxy credentials before saving them, shows the current connection state, and lets you disconnect with one click when you are finished.

Features:

- Browser-level proxy control without changing system-wide network settings.
- Direct credential verification over HTTPS before credentials are saved.
- Clear connection status and actionable error messages.
- Configurable proxy host and port for self-hosted deployments.
- Optional server-side closure of active requests and tunnels when disconnecting.
- Local Chrome storage for proxy configuration.
- No analytics, browsing history collection, or personal information collection.

The extension requires a proxy username and a password of at least 12 characters. Credentials are sent only to the configured proxy verification and disconnect endpoints. A forced browser or operating-system shutdown cannot guarantee that the optional server-side disconnect request is delivered.

homerouter.io is open source and part of the Homerouter project:
https://github.com/mklarsen/homerouter

## Suggested category

Productivity

## Support URL

https://github.com/mklarsen/homerouter/issues

## Privacy disclosure

The extension handles proxy credentials and sends them to the configured homerouter.io verification and disconnect endpoints. Credentials are stored locally in Chrome extension storage. The extension does not collect analytics, browsing history, or personal information.

## Store assets

- Use the existing `icons/icon128.png` as the extension icon.
- Add screenshots showing the popup in its disconnected, settings, and connected states.
- Keep screenshots free of real usernames, passwords, private proxy hosts, and personal browsing data.
