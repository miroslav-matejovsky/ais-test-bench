# Display browser verification

`display-browser.mjs` checks the embedded display using Chromium's DevTools
Protocol and Node's built-in HTTP, WebSocket, filesystem, and assertions. It
requires no installed Node packages or frontend build. This optional browser
check supplements the Go handler and API tests run by `task all`.

Start the default combined app with its three stations and at least one received
target:

```powershell
go run ./cmd/ais-testbench -addr 127.0.0.1:18080
```

In another PowerShell session, start headless Edge with a dedicated temporary
profile and a local debugging port, then run the check from the repository root:

```powershell
$displayBrowserProfile = Join-Path $env:TEMP 'ais-display-browser-check'
Start-Process -FilePath 'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe' -WindowStyle Hidden -ArgumentList @('--headless=new', '--disable-gpu', '--no-first-run', '--remote-debugging-port=19222', ('--user-data-dir=' + $displayBrowserProfile), 'about:blank')
$env:AIS_BROWSER_EVIDENCE = Join-Path $env:TEMP 'ais-display-evidence'
node internal/ui/testdata/display-browser.mjs
```

`AIS_BROWSER_ORIGIN` and `AIS_BROWSER_CDP` override the default app and debugging
origins. `AIS_BROWSER_EVIDENCE` is optional; when supplied, screenshots capture
coverage, capabilities, receiver provenance, a message, and mobile layout. Close
the dedicated browser after testing. The script removes its response injection
and network blocking at completion, including failed assertions.

The script reads a live snapshot as a source of coverage geometry and a received
NMEA sentence, then injects deterministic presentation fixtures into that browser.
It makes no simulation writes. These fixtures test presentation and concurrency;
the display package's Go tests separately validate NMEA and snapshot consistency.

Checks cover overlapping receivers and older positions, station-only and empty
reception sets, disabled sites/channels, lost and expired targets, nullable AIS
values, HTML-like labels, exact clipboard bytes, large cursors, bounded history,
gaps, 409 recovery, inspector pause, RF edits and original receiver attribution,
station removal, restart, delayed selection/history replies, union selection,
network staleness, focus, mobile layout, and Leaflet/tile failures.
