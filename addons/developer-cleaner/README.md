# DeClean POC

**DeClean** is short for **Developer Cleaner**, by homerouter.io. The name's
origin is also included in the settings page's **About DeClean** section.

Standalone Chrome Manifest V3 add-on for a future optional developer feature in
the homerouter.io proxy extension. It does not configure the proxy, contact the
router, or modify the existing extension. Developer mode and all automation are
off on first installation. There is no subscription, telemetry, remote code, or
runtime network dependency.

## Try It Locally

Requires Node.js 20.9+ for development and Chrome 120+ for the extension.

```powershell
cd addons/developer-cleaner
npm ci
npm run build
npm test
```

1. Open `chrome://extensions` and enable Chrome's **Developer mode**.
2. Choose **Load unpacked** and select this `addons/developer-cleaner` directory.
3. Pin **DeClean by homerouter.io** to the toolbar.
4. Open the popup's gear (**Open setup**), configure and save a preset, select it,
   and enable the add-on's separate **Developer mode** switch in setup.
5. Return to a disposable test website and open the popup. Press **Clean** to run
   the saved selected preset. Cache-only presets execute immediately; presets
   involving other data or risky follow-up actions require confirmation.

The compact popup contains status, setup and one **Clean** action. Configuration,
preset selection, target selection for manual review and the on/off switch live
on the setup page. Popup cleanup resolves the actual active browser tab, not a
previously selected setup tab, and never silently enables developer mode.

The gear opens the full settings page, where **Review cleanup** remains available
for previewing any preset without immediately running it.
The shortcut `Alt+Shift+X` and page/action
context menu open settings for review; they never silently delete data. Chrome
may reserve a shortcut; configure it at `chrome://extensions/shortcuts`.
Opening [options.html](options.html) directly displays the layout but deliberately
disables controls because Chrome extension APIs are unavailable on a file URL.

Use a separate Chrome test profile for real deletion tests, especially extension
data or window closing. No real browsing-data removal has been performed during
development validation; automated tests use mocked Chrome APIs.

## Feature Coverage

Inspired by the public [Clear Cache feature list](https://clearcache.io/), including
its **PLUS** features (the paid tier at the time of research). This is an
independent implementation, not a copy of its code, branding or paid assets.
The locally installed `chrome-extension://...` options page was not accessible;
exact parity with undocumented options has not been established.

| Capability                    | POC implementation                                                                                                                           |
| ----------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| Data types                    | HTTP cache, Cache Storage, cookies, local storage, IndexedDB, service workers, file systems, browsing history, download history and autofill |
| Scope                         | Target tab origin, include list, exclude list, or whole browser                                                                              |
| Per-type domain rules (PLUS)  | Each selected type can inherit or override the preset scope and origin list                                                                  |
| Time ranges                   | Last 15 minutes, hour, day, week, 30 days, or all time                                                                                       |
| Presets (PLUS)                | Create, duplicate, edit and delete up to 20 named presets                                                                                    |
| Popup action                  | One-click execution of the saved cache-only preset; confirmation retained for other data and risky actions                                   |
| Schedules and triggers (PLUS) | Minute intervals, browser startup and non-window-closing tab-close events                                                                    |
| Close all windows (PLUS)      | Explicit preset option, manual confirmation required; normal non-incognito windows only                                                      |
| Import/export (PLUS)          | Versioned JSON, schema validation and a 100 KB import limit                                                                                  |
| Browser settings              | Import supported types selected in Chrome's clear-data dialog; map time range to the nearest available preset range                          |
| Sync                          | Opt-in Chrome account sync of presets and appearance only; 8 KB total shared item limit                                                      |
| App/extension data            | Explicit protected-web and extension origin-type options; whole-browser scope only                                                           |
| Appearance                    | Original homerouter.io dark neon style by default; optional light/system themes, distinct inactive/active/clearing/review states             |
| Extra safety                  | Expiring previews, one-use confirmation, local activity log, partial-failure reporting and no overlapping cleanup                            |

Notifications are described as "coming soon" on the reference product's public
page and are not implemented here.

## Safety And Chrome Limits

- The popup's **Clean** action builds its plan from saved settings and the actual
  active tab. Only cache-only presets without protected/extension origins or
  window closing bypass the confirmation dialog. The worker enforces this rule,
  not just the UI. Other manual cleanups require a fresh, 60-second preview and
  explicit acknowledgement. Changing settings or the target tab invalidates an
  active-tab preview. Turning developer mode off in setup stops future triggers
  and remaining cleanup steps; it cannot cancel an API call already in progress.
- Only cache-only presets with explicit include/exclude or whole-browser scope
  can run unattended. Active-tab presets and any preset involving other data,
  protected/extension origins or window closing queue manual approval instead.
  The toolbar displays `!`; open setup, select the queued preset and review.
- Whole-browser history, download-list and autofill removal cannot be filtered
  by origin. Such combinations are rejected before the first removal call,
  never silently widened to the entire browser.
- Origin lists accept exact HTTP(S) origins, including ports. A bare host expands
  to HTTP and HTTPS. Subdomains are not automatically included. Wildcards, paths,
  credentials and non-HTTP(S) schemes are rejected.
- Chrome removes cookies for the **entire registrable domain**, even when an
  origin names one subdomain. Exclusions also protect the registrable domain.
- Some website storage APIs do not honor time ranges precisely and may clear
  all storage for a selected origin. Removing website storage can destroy work
  or sign users out. Download history removal does not delete downloaded files.
- Chrome no longer supports extension-driven password deletion. Flash/plugin
  data, AppCache, WebSQL and server-bound certificate deletion are obsolete;
  these types are deliberately omitted rather than presenting no-op controls.
- App/extension data deletion can reset installed apps, other extensions and
  this add-on itself. It always requires manual approval. Password manager
  contents, DNS cache, HTTP-auth cache and remote/server caches are not cleared.
- Per-type operations are sequential, not transactional. A failure stops later
  types and prevents reload/window closing; already removed data cannot be
  restored. The activity log records partial completion.
- Incognito is not supported. Schedules are best effort while Chrome is running;
  they do not wake a closed browser. There is no browser-shutdown deletion hook.
- Sync publishes local presets on opt-in. Incoming changes disable developer
  mode and automation on the receiving device for review. Exported/imported
  settings also leave developer mode, sync and automation off. Origin lists and
  preset names are shared with the user's Chrome account only after opt-in.
- The last 20 cleanup results stay in extension-local storage. No browsing
  contents, cookie values, passwords or full URLs are written to that log.

API behavior follows Chrome's
[browsingData reference](https://developer.chrome.com/docs/extensions/reference/api/browsingData).

## Permissions

| Permission     | Reason                                                                          |
| -------------- | ------------------------------------------------------------------------------- |
| `browsingData` | Selected cleanup operations and import of browser cleanup settings              |
| `storage`      | Local settings, activity and opt-in preset sync                                 |
| `tabs`         | Target-tab title/URL selection, navigation checks, reload and tab-close trigger |
| `alarms`       | Opt-in interval scheduling across service-worker restarts                       |
| `contextMenus` | Open the manual cleanup review from page/action menus                           |

No host permissions, content scripts, proxy permissions or remote backend are
needed. Build dependencies only provide local artwork; they do not ship as
runtime JavaScript.

## Logo

The POC has its own developer-cleanup mark: mint code brackets inside a cyan reset
ring, a yellow sparkle and a dark neon frame. It keeps the proxy extension's
color direction without reusing its fingerprint/house symbol. At 16 pixels the
ring is omitted to keep the code brackets legible. The proxy logo is unchanged.

`npm run build` generates 16, 32, 48, 128 and 512 pixel PNGs and an SVG master in
`icons/` from the composition in `build.cjs`, using Lucide glyphs and Sharp.
The generated directory includes Lucide's license. These build artifacts are
ignored by Git and rebuilt locally or in CI.

## Validation And Future Integration

`npm test` uses Node's built-in runner, matching the existing proxy extension's
test convention. Tests cover origin/range planning, unsupported combinations,
default-off behavior, confirmation expiry/replay, navigation changes, partial
failures, safe automation and sync safety. CI also builds and syntax-checks the
add-on, then uploads only runtime files and built icons as an unpackable artifact.

For future integration, reuse `core.js` behind the main extension's developer
switch and adapt the message boundary in `background.js`. Do not add cleanup
permissions or behavior to the proxy extension until this isolated POC has been
tested in a disposable browser profile and accepted.

### POC Handover (2026-10-19)

- All 25 automated tests and the local asset build pass. UI flow checks use
  mocked extension APIs; no real browser data was removed during development.
- The popup document has a fixed 320 px intrinsic width. Keep that independent
  of `100vw` and test a narrow initial viewport to avoid Chrome popup collapse.
- Next acceptance check: reload the native toolbar popup and exercise selected
  cleanup operations in a disposable Chrome profile, including repeated runs
  and service-worker/browser restarts. No proxy-extension integration yet.
