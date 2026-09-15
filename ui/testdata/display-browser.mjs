// Optional browser regression check. Requires a running combined app with its
// default stations and at least one reception, Node, and Chromium's CDP port.
// Uses only GETs; deterministic display responses are injected in this browser.
// See ui/testdata/README.md for invocation. No packages or frontend build needed.
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
const origin = process.env.AIS_BROWSER_ORIGIN || 'http://127.0.0.1:18080';
const cdp = process.env.AIS_BROWSER_CDP || 'http://127.0.0.1:19222';
const evidence = process.env.AIS_BROWSER_EVIDENCE;
const seed = await (await fetch(origin + '/display/api/observations?stations=all')).json();
assert(seed.stations.length >= 3 && seed.targets.length, 'Start the default combined app and wait for its first received target.');
const tabs = await (await fetch(cdp + '/json')).json();
const ws = new WebSocket(tabs.find(t => t.type === 'page').webSocketDebuggerUrl);
await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject; });
let sequence = 0;
const pending = new Map(), errors = [];
ws.onmessage = event => {
    const m = JSON.parse(event.data);
    if (m.id) { const p = pending.get(m.id); pending.delete(m.id); m.error ? p.reject(m.error) : p.resolve(m.result); }
    if (m.method === 'Runtime.exceptionThrown') errors.push(JSON.stringify(m.params.exceptionDetails));
};
function call(method, params = {}) {
    return new Promise((resolve, reject) => { const id = ++sequence; pending.set(id, { resolve, reject }); ws.send(JSON.stringify({ id, method, params })); });
}
async function evaluate(expression) {
    const r = await call('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true });
    if (r.exceptionDetails) throw new Error(JSON.stringify(r.exceptionDetails));
    return r.result.value;
}
function waitFor(predicate) {
    return evaluate(`new Promise((resolve,reject) => {
        const deadline = performance.now() + 15000;
        function check() {
            if (${predicate}) return resolve(true);
            if (performance.now() > deadline) return reject(new Error('Timed out: ' + ${JSON.stringify(predicate)}));
            requestAnimationFrame(check);
        }
        check();
    })`);
}
const click = selector => evaluate(`document.querySelector(${JSON.stringify(selector)}).click()`);
const change = (id, value) => evaluate(`document.getElementById(${JSON.stringify(id)}).value=${JSON.stringify(value)}; document.getElementById(${JSON.stringify(id)}).dispatchEvent(new Event('change'))`);
async function capture(name) {
    if (!evidence) return;
    await mkdir(evidence, { recursive: true });
    const result = await call('Page.captureScreenshot', { format: 'png', captureBeyondViewport: true });
    await writeFile(join(evidence, name + '.png'), Buffer.from(result.data, 'base64'));
}

function installFixture(seed) {
    const nativeFetch = window.fetch.bind(window);
    const clone = value => structuredClone(value);
    const base = clone(seed);
    base.simulationId = 'fixture-run-1';
    base.time = { now: '2026-09-14T12:00:00Z', elapsedMs: 90000, speed: 0, paused: true };
    base.stations = base.stations.slice(0, 3);
    base.stations.forEach((s, i) => {
        s.id = `s${i + 1}`; s.configRevision = '1'; s.rfRevision = '1';
        s.definition.name = ['Coast <img src=x onerror=alert(1)>', 'Northern receiver', 'Disabled harbour'][i];
        s.definition.enabled = i !== 2;
        if (i === 1) s.definition.channelB.enabled = false;
        if (i === 2) s.coverage = [];
    });
    const first = clone(seed.targets[0].report);
    Object.assign(first, { stationId: 's1', mmsi: 111000001, sequence: '9007199254740993', transmissionSequence: '9007199254740993',
        receivedAt: '2026-09-14T11:59:59Z', configRevision: '1', rfRevision: '1', channel: 'A' });
    first.receiver.name = base.stations[0].definition.name;
    first.navigation = { latitude: 52.02, longitude: 3.97, speed: 8, course: 90, heading: 90, utcSecond: 59 };
    first.scenario = { name: '<b>Scenario only</b>', categoryId: base.scenarioCategories[0]?.id || 'unknown' };
    const older = clone(first);
    Object.assign(older, { stationId: 's2', sequence: '44', transmissionSequence: '44', receivedAt: '2026-09-14T11:59:40Z' });
    older.receiver.name = base.stations[1].definition.name;
    older.navigation.latitude = 52.01;
    const unique = clone(older); unique.mmsi = 111000002; unique.sequence = '45';
    const noFix = clone(first); noFix.mmsi = 111000003; noFix.sequence = '9007199254740994';
    noFix.navigation = { latitude: null, longitude: null, speed: null, course: null, heading: null, utcSecond: null };
    const lost = clone(first); lost.mmsi = 111000004; lost.sequence = '1'; lost.receivedAt = '2026-09-14T11:58:00Z';
    function provenance(r, chosen = true) {
        const ageMs = Date.parse(base.time.now) - Date.parse(r.receivedAt);
        return { stationId: r.stationId, stationEnabled: true, sequence: r.sequence, transmissionSequence: r.transmissionSequence,
            receivedAt: r.receivedAt, channel: r.channel, estimatedPowerDbm: r.signal.estimatedPowerDbm, rfRevision: r.rfRevision,
            currentRfRevision: true, ageMs, status: ageMs > 60000 ? 'lost' : ageMs > 10000 ? 'stale' : 'fresh', chosen };
    }
    const target = (r, reports) => ({ mmsi: r.mmsi, ageMs: provenance(r).ageMs, status: provenance(r).status, report: clone(r), stations: reports.map((p, i) => provenance(p, i === 0)) });
    base.targets = [target(first, [first, older]), target(unique, [unique]), target(noFix, [noFix]), target(lost, [lost])];
    base.recentReceptions = [older, unique, first, noFix];
    const state = window.fixture = { base, first, older, count: 0, historyCount: 0, requests: [], failure: 0, removed: false, empty: false,
        expired: false, edited: false, run: 'fixture-run-1', historyMode: 'tail', delaySelection: false, historyDelay: false, waits: [], historyWaits: [] };
    state.release = () => state.waits.splice(0).forEach(resolve => resolve());
    state.releaseHistory = () => state.historyWaits.splice(0).forEach(resolve => resolve());
    window.fetch = async (url, options) => {
        if (!String(url).startsWith('/display/api/')) return nativeFetch(url, options);
        state.requests.push({ url: String(url), method: options?.method || 'GET' });
        if (String(url).includes('/observations')) {
            state.count++;
            const result = clone(base); result.simulationId = state.run;
            let ids = new URL(url, location.origin).searchParams.get('stations');
            if (state.delaySelection && ids === 's1') { state.delaySelection = false; await new Promise(resolve => state.waits.push(resolve)); }
            if (state.failure) return new Response('fixture upstream unavailable', { status: state.failure });
            if (state.empty) result.stations = [];
            else if (state.removed) result.stations = result.stations.filter(s => s.id !== 's1');
            if (ids === 'all') ids = result.stations.map(s => s.id); else ids = ids.split(',');
            if (ids.some(id => !result.stations.some(s => s.id === id))) return new Response('unknown station', { status: 404 });
            result.selection = ids;
            result.stations.forEach(s => { s.selected = ids.includes(s.id); });
            result.targets = result.targets.filter(t => t.stations.some(p => ids.includes(p.stationId)));
            result.targets.forEach(t => {
                t.stations = t.stations.filter(p => ids.includes(p.stationId));
                if (t.mmsi === first.mmsi && !ids.includes('s1')) {
                    t.report = clone(older); t.ageMs = 20000; t.status = 'stale'; t.stations[0].chosen = true;
                }
            });
            if (state.expired) result.targets = [];
            result.recentReceptions = result.recentReceptions.filter(r => ids.includes(r.stationId));
            result.currentTargets = result.targets.filter(t => t.status !== 'lost').length;
            result.lostTargets = result.targets.filter(t => t.status === 'lost').length;
            for (const s of result.stations) {
                const observations = result.targets.flatMap(t => t.stations.filter(p => p.stationId === s.id));
                s.currentTargets = observations.filter(p => p.status !== 'lost').length;
                s.lostTargets = observations.filter(p => p.status === 'lost').length;
                if (s.id === 's3') { s.recent.receiveRatio = null; s.recent.received = '0'; }
            }
            if (state.edited && result.stations[0]) {
                const s = result.stations[0]; s.definition.name = 'Edited coast'; s.configRevision = '2'; s.rfRevision = '2';
                s.definition.channelA.enabled = false; s.coverage = s.coverage.filter(c => c.channel !== 'A');
            }
            return Response.json(result);
        }
        state.historyCount++;
        const query = new URL(url, location.origin), id = query.pathname.split('/')[4];
        if (state.historyDelay) { state.historyDelay = false; await new Promise(resolve => state.historyWaits.push(resolve)); }
        if (state.historyMode === 'conflict') { state.historyMode = 'tail'; return new Response('different run', { status: 409 }); }
        const report = id === 's2' ? older : first;
        let receptions = query.searchParams.has('after') ? [] : [clone(report)];
        if (state.historyMode === 'gap') {
            receptions = Array.from({ length: 200 }, (_, i) => ({ ...clone(report), sequence: String(BigInt(first.sequence) + BigInt(i + 100)) }));
        }
        return Response.json({ simulationId: state.run, stationId: id, tail: !query.searchParams.has('after'),
            oldestAvailable: receptions[0]?.sequence || report.sequence, latestAvailable: receptions.at(-1)?.sequence || report.sequence,
            truncatedBefore: state.historyMode === 'gap' ? receptions[0].sequence : null,
            receptions, nextAfter: receptions.at(-1)?.sequence || report.sequence, hasMore: false, gap: state.historyMode === 'gap' });
    };
}
let injection;
try {
    await call('Runtime.enable'); await call('Page.enable');
    await call('Emulation.setDeviceMetricsOverride', { width: 1440, height: 1000, deviceScaleFactor: 1, mobile: false });
    injection = await call('Page.addScriptToEvaluateOnNewDocument', { source: `(${installFixture.toString()})(${JSON.stringify(seed)})` });
    await call('Page.navigate', { url: origin + '/display' });
    await waitFor(`document.querySelectorAll('#target-body tr').length === 3`);
    assert.equal(await evaluate(`document.querySelectorAll('.station-icon').length`), 3);
    assert(await evaluate(`document.querySelectorAll('#map .leaflet-overlay-pane svg path').length > 2`), 'coverage polygons are drawn');
    assert(await evaluate(`document.querySelector('#station-comparison').textContent.includes('<img') && !document.querySelector('#station-comparison img')`), 'station name is plain text');
    assert(await evaluate(`document.querySelector('#sim-clock').textContent.includes('Paused')`));
    const clock = await evaluate(`document.querySelector('#sim-clock').textContent`);
    await click('#fit-coverage'); await capture('display-coverage');
    const aPaths = await evaluate(`document.querySelectorAll('#map .leaflet-overlay-pane svg path').length`);
    await change('coverage-channel', 'B');
    assert(await evaluate(`document.querySelectorAll('#map .leaflet-overlay-pane svg path').length < ${aPaths}`), 'disabled channel B contributes no contours');
    await change('coverage-channel', 'A');
    await click('#show-coverage');
    assert.equal(await evaluate(`document.querySelectorAll('#map .leaflet-overlay-pane svg path').length`), 2, 'only targets remain');
    await click('#show-coverage'); await click('#show-lost');
    assert.equal(await evaluate(`document.querySelectorAll('#target-body tr').length`), 4);
    await click('#show-lost');
    await click('#target-body tr[data-key="111000003"] button');
    assert(await evaluate(`document.querySelector('#detail-summary').textContent.includes('unavailable')`));
    await click('#target-body tr[data-key="111000001"] button');
    assert.equal(await evaluate(`document.querySelectorAll('#provenance-body tr').length`), 2);
    assert(await evaluate(`document.querySelector('#provenance-body').textContent.includes('Older report')`));
    await capture('display-provenance');
    await click('#provenance-body tr[data-key="s2"] button');
    await waitFor(`document.querySelector('#detail-summary').textContent.includes('52.010000')`);
    assert.equal(await evaluate(`document.querySelectorAll('#target-body tr').length`), 2);
    await waitFor(`document.querySelector('#history-status').textContent.includes('Station history sample')`);
    await click('#message-body button');
    assert(await evaluate(`document.querySelector('#message-summary').textContent.includes('Reception-time config revision 1')`));
    await call('Browser.grantPermissions', { origin, permissions: ['clipboardReadWrite', 'clipboardSanitizedWrite'] });
    await click('#copy-nmea');
    await waitFor(`document.querySelector('#copy-status').textContent.startsWith('Copied exact NMEA')`);
    assert.equal(await evaluate(`navigator.clipboard.readText()`), await evaluate(`fixture.older.sentence`), 'clipboard preserves exact CRLF');
    await capture('display-message');
    await click('#all-stations');
    await waitFor(`document.querySelectorAll('#target-body tr').length === 3`);
    await click('#station-comparison tr[data-key="s1"] button');
    await waitFor(`document.querySelector('#detail-summary').textContent.includes('Published reception model parameters')`);
    await capture('display-capabilities');
    await waitFor(`document.querySelector('#history-status').textContent.includes('Station history sample')`);
    await click('#message-body button');
    await evaluate(`fixture.edited = true`);
    await waitFor(`document.querySelector('#detail-summary').textContent.includes('Config revision 2; RF revision 2')`);
    assert(await evaluate(`document.querySelector('#message-summary').textContent.includes('config revision 1') && document.querySelector('#message-summary').textContent.includes('Coast <img')`), 'historical receiver configuration stays pinned');
    await evaluate(`fixture.historyMode = 'gap'`);
    await waitFor(`document.querySelector('#history-status').textContent.startsWith('History gap')`);
    assert.equal(await evaluate(`document.querySelectorAll('#message-body tr').length`), 200);
    assert(await evaluate(`fixture.requests.some(r => r.url.includes('after=9007199254740993'))`), 'large cursor retained exactly');
    await click('#pause-history');
    const historyCount = await evaluate(`fixture.historyCount`);
    const pollCount = await evaluate(`fixture.count`);
    await waitFor(`fixture.count > ${pollCount + 1}`);
    assert.equal(await evaluate(`fixture.historyCount`), historyCount, 'inspector pause stops history polling');
    assert.equal(await evaluate(`document.querySelector('#sim-clock').textContent`), clock, 'paused virtual clock does not advance');
    await evaluate(`fixture.historyMode = 'tail'; fixture.failure = 503`);
    await waitFor(`document.querySelector('#display-view').classList.contains('stale')`);
    assert.equal(await evaluate(`document.querySelectorAll('#target-body tr').length`), 2, 'network failure retains observations');
    assert(await evaluate(`document.querySelector('#live-status').textContent.includes('Entire view is stale')`));
    await evaluate(`fixture.failure = 0`);
    await waitFor(`!document.querySelector('#display-view').classList.contains('stale')`);
    await click('#pause-history');
    await evaluate(`fixture.historyMode = 'conflict'`);
    const beforeConflict = await evaluate(`fixture.historyCount`);
    await waitFor(`fixture.historyCount > ${beforeConflict + 1}`);
    assert(await evaluate(`fixture.requests.filter(r => r.url.includes('/receptions')).at(-1).url.indexOf('after=') === -1`), '409 resets cursor');
    await evaluate(`fixture.removed = true`);
    await waitFor(`document.querySelectorAll('#station-comparison tr').length === 2 && document.querySelector('#all-stations').getAttribute('aria-pressed') === 'true'`);
    assert(await evaluate(`document.querySelector('#selection-notice').textContent.includes('removed')`));
    await click('#target-body tr[data-key="111000001"] button');
    await evaluate(`fixture.expired = true`);
    await waitFor(`document.querySelector('#detail-summary').textContent.includes('expired')`);
    await evaluate(`fixture.expired = false; fixture.removed = false; fixture.edited = false; fixture.run = 'fixture-run-2'`);
    await waitFor(`document.querySelector('#selection-notice').textContent.includes('restarted')`);
    assert.equal(await evaluate(`document.querySelector('#detail-summary').textContent`), 'Select a station or target.');
    assert.equal(await evaluate(`document.querySelector('#raw-nmea').value`), '');
    // Deliberately ignore AbortSignal in the fixture to exercise generation checks.
    await evaluate(`fixture.delaySelection = true; document.querySelector('#station-selection [data-id="s1"] input').click()`);
    await waitFor(`fixture.waits.length === 1`);
    await click('#all-stations');
    await evaluate(`fixture.release()`);
    await waitFor(`!document.querySelector('#display-view').classList.contains('pending')`);
    assert.equal(await evaluate(`document.querySelectorAll('#target-body tr').length`), 3, 'late selection response is ignored');
    await evaluate(`fixture.historyDelay = true`);
    await change('history-station', 's1');
    await waitFor(`fixture.historyWaits.length === 1`);
    await change('history-station', 's2');
    await evaluate(`fixture.releaseHistory()`);
    await waitFor(`document.querySelector('#history-status').textContent.includes('Station history sample')`);
    assert(await evaluate(`document.querySelector('#message-body').textContent.includes('Northern receiver') && !document.querySelector('#message-body').textContent.includes('Coast')`), 'late history response cannot restore another station');
    await click('#station-selection [data-id="s1"] input');
    await waitFor(`!document.querySelector('#display-view').classList.contains('pending')`);
    await click('#station-selection [data-id="s2"] input');
    await waitFor(`document.querySelector('#target-counts').textContent.includes('2 receiving stations')`);
    assert.equal(await evaluate(`document.querySelectorAll('#target-body tr').length`), 3, 'multiselection is a union');
    await evaluate(`fixture.run = 'fixture-run-3'`);
    await waitFor(`document.querySelector('#all-stations').getAttribute('aria-pressed') === 'true' && document.querySelectorAll('#target-body tr').length === 3`);
    assert.equal(await evaluate(`document.querySelector('#history-station').value`), '', 'restart clears explicitly selected station history');
    await evaluate(`document.querySelector('#station-filter').value='Northern';document.querySelector('#station-filter').dispatchEvent(new Event('input'));document.querySelector('#station-filter').focus()`);
    const beforeFocus = await evaluate(`fixture.count`);
    await waitFor(`fixture.count > ${beforeFocus}`);
    assert.equal(await evaluate(`document.activeElement.id`), 'station-filter', 'poll preserves keyboard focus');
    assert.equal(await evaluate(`document.querySelectorAll('#station-comparison tr').length`), 1);
    await evaluate(`document.querySelector('#station-filter').value='';document.querySelector('#station-filter').dispatchEvent(new Event('input'))`);
    await call('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 1, mobile: true });
    assert(await evaluate(`document.querySelector('#observation-details').getBoundingClientRect().top >= document.querySelector('#map').getBoundingClientRect().bottom`), 'mobile details below map');
    assert(await evaluate(`document.documentElement.scrollWidth <= 390`), 'mobile has no page overflow');
    await capture('display-mobile');
    await evaluate(`fixture.empty = true`);
    await waitFor(`!document.querySelector('#empty-stations').hidden`);
    assert(await evaluate(`document.querySelector('#empty-stations a').getAttribute('href') === '/manager'`));
    assert(await evaluate(`fixture.requests.every(r => r.method === 'GET')`), 'display never writes simulation state');
    // Map dependency failure must not stop tables or API polling.
    await call('Network.enable');
    await call('Network.setCacheDisabled', { cacheDisabled: true });
    await call('Network.setBlockedURLs', { urls: ['*unpkg.com*', '*tile.openstreetmap.org*'] });
    await call('Page.navigate', { url: origin + '/display' });
    await waitFor(`document.querySelectorAll('#target-body tr').length === 3`);
    assert(await evaluate(`document.querySelector('#tile-status').textContent.includes('Map library unavailable')`));
    await call('Network.setBlockedURLs', { urls: ['*tile.openstreetmap.org*'] });
    await call('Page.navigate', { url: origin + '/display' });
    await waitFor(`document.querySelector('#tile-status').textContent.includes('Map tiles unavailable')`);
    assert.equal(await evaluate(`document.querySelectorAll('#target-body tr').length`), 3);
    assert.deepEqual(errors, [], 'no uncaught JavaScript exceptions');
    console.log('Display browser checks passed: coverage/channels, selection/union/provenance, safe labels, unavailable AIS, lost/expired targets, history bounds/gap/409/pause, RF attribution, removal/restart, stale network, focus/mobile, and map dependency failures.');
} finally {
    if (injection) await call('Page.removeScriptToEvaluateOnNewDocument', { identifier: injection.identifier });
    await call('Network.setBlockedURLs', { urls: [] });
    ws.close();
}

