// Optional host-page browser check. Requires the host fixture from
// ui/testdata/host, Node, and Chromium's CDP port. It mounts two managers and two
// displays from independent simulators in one hostile page and checks isolation,
// custom fetch, destroy, and remount. See ui/testdata/README.md for invocation.
import assert from 'node:assert/strict';
const origin = process.env.AIS_HOST_ORIGIN || 'http://127.0.0.1:18090';
const cdp = process.env.AIS_BROWSER_CDP || 'http://127.0.0.1:19222';
const denied = await fetch(origin + '/a/api/vessels');
assert.equal(denied.status, 403, 'host middleware rejects API requests without the host token');
const tabs = await (await fetch(cdp + '/json')).json();
const ws = new WebSocket(tabs.find(t => t.type === 'page').webSocketDebuggerUrl);
await new Promise((resolve, reject) => { ws.onopen = resolve; ws.onerror = reject; });
let sequence = 0;
const pending = new Map(), errors = [];
ws.onmessage = event => {
    const m = JSON.parse(event.data);
    if (m.id) { const p = pending.get(m.id); pending.delete(m.id); m.error ? p.reject(new Error(JSON.stringify(m.error))) : p.resolve(m.result); }
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
        const deadline = performance.now() + 20000;
        function check() {
            if (${predicate}) return resolve(true);
            if (performance.now() > deadline) return reject(new Error('Timed out: ' + ${JSON.stringify(predicate)}));
            requestAnimationFrame(check);
        }
        check();
    })`);
}
// listeners counts event listeners registered directly on the first match.
async function listeners(selector) {
    const { result } = await call('Runtime.evaluate', { expression: `document.querySelector(${JSON.stringify(selector)})` });
    return (await call('DOMDebugger.getEventListeners', { objectId: result.objectId })).listeners.length;
}
const ref = (root, name) => `document.querySelector('#${root} [data-ref=${name}]')`;
const count = (id, part) => `aisHost.requests.filter(r => r.id === '${id}' && r.url.includes('${part}')).length`;
// polls waits until root id has made n more requests to part.
async function polls(id, part, n) {
    const before = await evaluate(count(id, part));
    await waitFor(`${count(id, part)} >= ${before + n}`);
}
const api = async (path, init = {}) => {
    const response = await fetch(origin + path, { ...init, headers: { 'X-Host-CSRF': 'token-a', 'Content-Type': 'application/json', ...init.headers } });
    assert(response.ok, `${path}: HTTP ${response.status}`);
    return response.json();
};

let injection;
try {
    await call('Runtime.enable'); await call('Page.enable'); await call('DOM.enable');
    await call('Emulation.setDeviceMetricsOverride', { width: 1440, height: 1000, deviceScaleFactor: 1, mobile: false });
    // A host-global Leaflet must survive; CSP violations are recorded.
    injection = await call('Page.addScriptToEvaluateOnNewDocument', { source: `window.L = { host: 'leaflet' }; window.cspViolations = [];
        document.addEventListener('securitypolicyviolation', e => cspViolations.push(e.violatedDirective + ' ' + e.blockedURI));` });
    await call('Page.navigate', { url: origin + '/' });
    await waitFor(`window.aisHost?.ready`);
    await waitFor(`${ref('fleet-a', 'fleet-status')}.textContent.includes('active vessel') && ${ref('fleet-b', 'fleet-status')}.textContent.includes('active vessel')`);
    await waitFor(`document.querySelectorAll('#map-a .ais-station-icon').length >= 3 && document.querySelectorAll('#fleet-a [data-ref=stations-body] tr').length >= 3`);

    // Configured paths, custom fetch, and independent instances.
    assert(await evaluate(`aisHost.requests.every(r => r.url.startsWith({ 'fleet-a': '/a/api/', 'fleet-b': '/b/api/', 'map-a': '/a/display/api/', 'map-b': '/b/display/api/' }[r.id]))`), 'each root uses its configured API base');
    await waitFor(`${ref('map-a', 'live-status')}.textContent.startsWith('Connected')`);

    // Unique IDs, labels inside their own component, and untouched host elements.
    assert.deepEqual(await evaluate(`(() => { const ids = [...document.querySelectorAll('[id]')].map(e => e.id); return ids.filter((id, i) => ids.indexOf(id) !== i); })()`), [], 'no duplicate IDs');
    assert(await evaluate(`[...document.querySelectorAll('.ais-manager label[for], .ais-display label[for]')].every(label => label.closest('[data-ais-mounted]').contains(document.getElementById(label.htmlFor)))`), 'labels resolve inside their component');
    assert(await evaluate(`document.querySelectorAll('#fleet-a label[for]').length === 17 && document.querySelector('#fleet-b label[for="fleet-b-station-name"]') !== null`), 'station editor labels use root-derived IDs');
    assert.equal(await evaluate(`document.getElementById('vessel-count').value`), 'host');
    assert.equal(await evaluate(`document.getElementById('map').textContent`), 'Host map placeholder');
    assert.equal(await evaluate(`document.getElementById('messages').textContent`), 'Host messages');
    assert.equal(await evaluate(`getComputedStyle(document.getElementById('vessel-form')).display`), 'block', 'host form keeps its style');
    assert.equal(await evaluate(`getComputedStyle(document.getElementById('stations-table')).borderCollapse`), 'separate', 'host table keeps its style');
    assert.notEqual(await evaluate(`getComputedStyle(document.getElementById('apply-count')).paddingLeft`), '12px', 'host button keeps its style');
    assert.equal(await evaluate(`getComputedStyle(${ref('fleet-a', 'apply-count')}).paddingLeft`), '12px', 'component button is styled');
    await evaluate(`document.body.style.setProperty('--ais-accent', 'rgb(1, 2, 3)')`);
    assert.equal(await evaluate(`getComputedStyle(${ref('map-a', 'all-stations')}).backgroundColor`), 'rgb(1, 2, 3)', 'theme variable reaches the component');
    assert.equal(await evaluate(`window.L.host`), 'leaflet', 'host window.L is untouched');
    assert.equal(await evaluate(`typeof window.htmx`), 'undefined', 'components need no htmx');

    // Independent state: pausing A leaves B running.
    await evaluate(`document.querySelector('#fleet-a [data-speed="0"]').click()`);
    await waitFor(`${ref('fleet-a', 'speed-status')}.textContent === 'Simulation paused.'`);
    await polls('fleet-b', 'metadata', 2);
    assert(await evaluate(`${ref('fleet-a', 'sim-clock')}.textContent.includes('paused') && !${ref('fleet-b', 'sim-clock')}.textContent.includes('paused')`), 'instances keep separate state');

    // Keyboard focus survives polling.
    await evaluate(`document.getElementById('fleet-b-vessel-count').focus()`);
    await polls('fleet-b', 'vessels', 2);
    assert.equal(await evaluate(`document.activeElement.id`), 'fleet-b-vessel-count', 'poll preserves focus');

    // Hidden container: a zero-size map holds at most its center tile; once shown,
    // the resize observer lets Leaflet fill the container with tiles.
    assert(await evaluate(`document.querySelectorAll('#map-b .leaflet-tile').length <= 1`), 'hidden map loads no tile grid');
    await evaluate(`document.getElementById('slot-map-b').hidden = false`);
    await waitFor(`document.querySelectorAll('#map-b .leaflet-tile').length > 4`);
    // Narrow roots switch to one column through the container query.
    await evaluate(`document.getElementById('slot-map-a').style.width = '600px'`);
    await waitFor(`getComputedStyle(document.querySelector('#map-a .ais-display-layout')).gridTemplateColumns.split(' ').length === 1`);
    await waitFor(`${ref('map-a', 'map')}.querySelectorAll('.leaflet-tile').length > 0`);
    await evaluate(`document.getElementById('slot-map-a').style.width = ''`);

    // Duplicate and wrong-root mounting fail clearly.
    assert.match(await evaluate(`(() => { try { aisHost.ui.mountManager(document.getElementById('fleet-a')); return 'mounted'; } catch (e) { return e.message; } })()`), /already mounted/);
    assert.match(await evaluate(`(() => { try { aisHost.ui.mountDisplay(document.getElementById('fleet-b')); return 'mounted'; } catch (e) { return e.message; } })()`), /data-ais-display/);

    // Destroy during held requests: nothing renders, polls stop, listeners and
    // Leaflet go away, and destroy is idempotent.
    assert(await listeners('#fleet-a [data-ref=vessel-form]') > 0);
    await evaluate(`aisHost.holdIds = new Set(['fleet-a', 'map-a'])`);
    await waitFor(`aisHost.held.some(h => h.id === 'fleet-a') && aisHost.held.some(h => h.id === 'map-a')`);
    await evaluate(`aisHost.handles['fleet-a'].destroy(); aisHost.handles['fleet-a'].destroy(); aisHost.handles['map-a'].destroy(); aisHost.handles['map-a'].destroy()`);
    await evaluate(`aisHost.holdIds = new Set(); aisHost.release()`);
    const destroyedRequests = await evaluate(`aisHost.requests.filter(r => r.id === 'fleet-a' || r.id === 'map-a').length`);
    await polls('fleet-b', 'vessels', 3);
    await polls('map-b', 'observations', 2);
    assert.equal(await evaluate(`aisHost.requests.filter(r => r.id === 'fleet-a' || r.id === 'map-a').length`), destroyedRequests, 'destroyed roots stop polling');
    assert.equal(await evaluate(`${ref('fleet-a', 'fleet-status')}.textContent`), 'Connecting...', 'late responses do not render');
    assert.equal(await evaluate(`${ref('map-a', 'live-status')}.textContent`), 'Connecting...');
    assert.equal(await evaluate(`document.querySelectorAll('#map-a .leaflet-container, #fleet-a [data-ref=stations-body] tr').length`), 0, 'markup is restored');
    assert.equal(await evaluate(`document.querySelector('#fleet-a[data-ais-mounted], #map-a[data-ais-mounted]')`), null);
    assert.equal(await listeners('#fleet-a [data-ref=vessel-form]'), 0, 'listeners removed');

    // Remount refreshes state without duplicate polling.
    await evaluate(`aisHost.mount('fleet-a'); aisHost.mount('map-a')`);
    await waitFor(`${ref('fleet-a', 'fleet-status')}.textContent.includes('active vessel') && document.querySelectorAll('#map-a .ais-station-icon').length >= 3`);
    assert(await evaluate(`${ref('fleet-a', 'sim-clock')}.textContent.includes('paused')`), 'remount reads authoritative state');
    const remountStart = await evaluate(count('fleet-a', 'vessels'));
    await polls('fleet-b', 'vessels', 4);
    assert(await evaluate(count('fleet-a', 'vessels')) - remountStart <= 5, 'remount polls once per interval');

    // A conflicting server change keeps the draft of the affected instance only.
    await evaluate(`document.querySelector('#fleet-a [data-ref=stations-body] tr button').click()`);
    await waitFor(`${ref('fleet-a', 'station-editor')}.open`);
    await evaluate(`const f = document.getElementById('fleet-a-station-name'); f.value = 'Draft name'; f.dispatchEvent(new Event('input'))`);
    const stations = await api('/a/api/stations');
    const first = stations.stations[0];
    await api(`/a/api/stations/${encodeURIComponent(first.id)}`, { method: 'PUT', body: JSON.stringify({
        simulationId: stations.simulationId, stationSetRevision: stations.stationSetRevision, definition: { ...first.definition, name: 'Server change' } }) });
    await waitFor(`!${ref('fleet-a', 'station-conflict')}.hidden`);
    assert.equal(await evaluate(`document.getElementById('fleet-a-station-name').value`), 'Draft name', 'draft is preserved');
    assert(await evaluate(`${ref('fleet-b', 'station-conflict')}.hidden`), 'other instance has no conflict');
    await evaluate(`${ref('fleet-a', 'station-cancel')}.click()`);
    await api('/a/api/time', { method: 'PUT', body: JSON.stringify({ speed: 1 }) });

    assert.deepEqual(await evaluate(`cspViolations`), [], 'no Content-Security-Policy violations');
    assert.deepEqual(errors, [], 'no uncaught JavaScript exceptions');
    console.log('Host browser checks passed: configured paths, custom fetch, independent state, unique IDs and labels, host styles and globals, theme variable, focus, hidden and narrow maps, duplicate mounts, destroy during requests, remount, and conflict drafts.');
} finally {
    if (injection) await call('Page.removeScriptToEvaluateOnNewDocument', { identifier: injection.identifier });
    ws.close();
}
