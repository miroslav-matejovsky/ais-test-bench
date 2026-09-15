# Browser verification

Two optional browser checks exercise the embedded components with Chromium's
DevTools Protocol and Node's built-in HTTP, WebSocket, filesystem, and assertions.
They need no installed Node packages or frontend build and supplement the Go
handler and API tests run by `task all`.

- `display-browser.mjs` checks display behavior on the standalone `/display` page
  of the combined app with deterministic injected responses.
- `host-browser.mjs` checks host-page embedding against `host/`, a Go program that
  mounts two managers and two displays from independent simulators in one page.

## Running

Start the default combined app with its three stations and at least one received
target, and the host fixture:

```powershell
go run ./cmd/ais-testbench -addr 127.0.0.1:18080
go run ./ui/testdata/host -addr 127.0.0.1:18090
```

In another PowerShell session, start headless Edge with a dedicated temporary
profile and a local debugging port, then run the checks one at a time from the
repository root:

```powershell
$browserProfile = Join-Path $env:TEMP 'ais-browser-check'
Start-Process -FilePath 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe' -WindowStyle Hidden -ArgumentList @('--headless=new', '--disable-gpu', '--no-first-run', '--remote-debugging-port=19222', ('--user-data-dir=' + $browserProfile), 'about:blank')
node ui/testdata/host-browser.mjs
$env:AIS_BROWSER_EVIDENCE = Join-Path $env:TEMP 'ais-display-evidence'
node ui/testdata/display-browser.mjs
```

`AIS_BROWSER_ORIGIN`, `AIS_HOST_ORIGIN`, and `AIS_BROWSER_CDP` override the app,
host, and debugging origins. `AIS_BROWSER_EVIDENCE` is optional; when supplied,
screenshots capture coverage, capabilities, receiver provenance, a message, and
mobile layout. Close the dedicated browser after testing. Each script removes its
injected scripts, and the display check its network blocking, at completion,
including failed assertions.

## Display check

The script reads a live snapshot as a source of coverage geometry and a received
NMEA sentence, then injects deterministic presentation fixtures into that browser.
It makes no simulation writes. These fixtures test presentation and concurrency;
the display package's Go tests separately validate NMEA and snapshot consistency.

Checks cover overlapping receivers and older positions, station-only and empty
reception sets, disabled sites/channels, lost and expired targets, nullable AIS
values, HTML-like labels, exact clipboard bytes, large cursors, bounded history,
gaps, 409 recovery, inspector pause, RF edits and original receiver attribution,
station removal, restart, delayed selection/history replies, union selection,
network staleness, focus, mobile layout, and failures of the Leaflet module and
map tiles.

## Host-page check

The host page reuses the old component IDs for its own form, table, map, and
messages, sets a host `window.L`, loads no inline scripts, and sends a strict
Content-Security-Policy. Its middleware rejects API requests without an
`X-Host-CSRF` header that only the page's custom fetch adds. The page's
`window.aisHost` records each root's requests and can hold them without honoring
abort signals.

Checks cover configured API paths per root, custom fetch headers, independent
simulator state, unique IDs and in-component labels, untouched host elements and
styles, a theme custom property, the host `window.L`, absent htmx, keyboard focus
across polls, a map that starts hidden, a narrow container layout, duplicate and
wrong-root mounting, destroy during held requests (no later requests, rendering,
listeners, or map), idempotent destroy, remount without duplicate polling, a
preserved station draft on a conflicting server change, Content-Security-Policy
violations, and uncaught exceptions. The check pauses instance A and restores its
speed at the end.
