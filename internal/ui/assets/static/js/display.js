/* global L */
(() => {
    const el = id => document.getElementById(id);
    const text = (id, value) => { if (el(id).textContent !== value) el(id).textContent = value; };
    const fmt = (v, unit = "", digits = 1) => v == null ? "unavailable" : `${Number(v).toFixed(digits)} ${unit}`.trim();
    const utc = v => v ? v.replace("T", " ").replace("Z", " UTC") : "unavailable";
    const percent = v => v == null ? "no opportunities" : fmt(v * 100, "%");
    const age = ms => fmt(ms / 1000, "s virtual");
    const palette = ["#185b9b", "#9a4510", "#287341", "#813c96", "#a62c4f", "#007477", "#655700", "#4b509a",
        "#9b346c", "#486f18", "#8c442d", "#305c68", "#713a3a", "#515f27", "#62487e", "#475c7a"];
    const colors = new Map();
    const color = id => { if (!colors.has(id)) colors.set(id, palette[colors.size % palette.length]); return colors.get(id); };
    const node = (tag, value) => { const n = document.createElement(tag); n.textContent = value; return n; };
    let snapshot = null, selection = [], generation = 0, controller = null, timer = null;
    let fetching = false, immediate = false, framed = false, detail = null, lastFetched = null;
    let historyStation = "", historyGeneration = 0, historyController = null, historyRows = [], historyCursor = null;
    let historyFetching = false, historyBlocked = false, message = null;
    const targets = new Map(), sites = new Map(), coverage = new Map();
    let map = null;
    if (window.L) {
        map = L.map(el("map")).setView([52.02, 3.97], 11);
        L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
            maxZoom: 19,
            attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
        }).addTo(map).on("tileerror", () => text("tile-status", "Map tiles unavailable. Observation tables and receiver updates remain active."));
    } else text("tile-status", "Map library unavailable. Observation tables remain active; reload to retry the map.");
    el("fit-targets").disabled = !map;
    el("fit-coverage").disabled = !map;
    const stationName = id => snapshot?.stations.find(s => s.id === id)?.definition.name || id;
    const stationLabel = id => `${stationName(id)} [${id}]`;
    const channels = d => [d.channelA.enabled ? "A" : "A disabled", d.channelB.enabled ? "B" : "B disabled"].join(" / ");
    const navText = n => `Position: ${fmt(n.latitude, "deg", 6)}, ${fmt(n.longitude, "deg", 6)}; speed ${fmt(n.speed, "kn")}; course ${fmt(n.course, "deg")}; heading ${fmt(n.heading, "deg", 0)}; AIS UTC second ${fmt(n.utcSecond, "", 0)}`;
    const action = (label, run) => ({ label, run });

    // Keyed rows preserve focused controls and scroll during ordinary polls.
    function table(id, rows) {
        const body = el(id), existing = new Map([...body.children].map(r => [r.dataset.key, r]));
        rows.forEach(([key, values], index) => {
            key = String(key);
            let row = existing.get(key);
            if (!row) { row = document.createElement("tr"); row.dataset.key = key; }
            existing.delete(key);
            values.forEach((value, i) => {
                const cell = row.cells[i] || row.insertCell();
                if (typeof value === "object") {
                    let button = cell.firstElementChild;
                    if (!button) { button = node("button", ""); button.type = "button"; cell.replaceChildren(button); }
                    if (button.textContent !== value.label) button.textContent = value.label;
                    button.onclick = value.run;
                } else if (cell.textContent !== String(value)) cell.textContent = value;
            });
            if (body.children[index] !== row) body.insertBefore(row, body.children[index] || null);
        });
        for (const row of existing.values()) row.remove();
    }

    function resetHistory(id = "") {
        historyGeneration++;
        historyController?.abort();
        historyStation = id; historyRows = []; historyCursor = null; historyBlocked = false; message = null;
        text("copy-status", "");
        el("history-station").value = id;
        text("history-status", id ? "Loading station history..." : "Recent selected reception sample.");
        renderMessages(); renderMessage();
    }
    function choose(ids, nextDetail = null) {
        selection = [...ids].sort(); detail = nextDetail; generation++;
        controller?.abort(); resetHistory(selection.length === 1 ? selection[0] : "");
        text("selection-notice", "Loading selected receiving stations...");
        el("display-view").classList.add("pending");
        requestRefresh();
    }
    function renderSelectors() {
        const container = el("station-selection");
        const existing = new Map([...container.children].map(n => [n.dataset.id, n]));
        const options = new Map([...el("history-station").options].slice(1).map(o => [o.value, o]));
        for (const s of snapshot.stations) {
            let label = existing.get(s.id);
            if (!label) {
                label = document.createElement("label"); label.dataset.id = s.id;
                const input = document.createElement("input"); input.type = "checkbox";
                input.onchange = () => {
                    const ids = new Set(selection);
                    if (input.checked) ids.add(s.id); else ids.delete(s.id);
                    choose([...ids]);
                };
                label.append(input, node("span", "")); container.append(label);
            }
            existing.delete(s.id); label.firstChild.checked = selection.includes(s.id);
            label.lastChild.textContent = stationLabel(s.id); label.style.borderColor = color(s.id);
            let option = options.get(s.id);
            if (!option) { option = document.createElement("option"); option.value = s.id; el("history-station").append(option); }
            option.textContent = stationLabel(s.id); options.delete(s.id);
        }
        for (const n of existing.values()) n.remove();
        for (const o of options.values()) o.remove();
        el("all-stations").setAttribute("aria-pressed", String(!selection.length));
        el("history-station").value = historyStation;
    }
    const visibleTargets = () => snapshot.targets.filter(t => t.status !== "lost" || el("show-lost").checked);
    function clearMap() {
        for (const m of targets.values()) map?.removeLayer(m);
        for (const m of sites.values()) map?.removeLayer(m);
        for (const c of coverage.values()) map?.removeLayer(c.layer);
        targets.clear(); sites.clear(); coverage.clear();
    }
    function renderMap() {
        if (!map) return;
        const active = new Set(snapshot.stations.map(s => s.id));
        for (const [id, m] of sites) if (!active.has(id)) { map.removeLayer(m); sites.delete(id); }
        for (const [id, c] of coverage) if (!active.has(id)) { map.removeLayer(c.layer); coverage.delete(id); }
        for (const s of snapshot.stations) {
            const d = s.definition;
            let marker = sites.get(s.id);
            if (!marker) {
                marker = L.marker([d.latitude, d.longitude]);
                marker.on("click", () => choose([s.id], { kind: "station", id: s.id })); sites.set(s.id, marker);
            }
            const label = `${stationLabel(s.id)}: ${d.enabled ? "enabled" : "disabled"}; ${channels(d)}`;
            if (marker.options.title !== label) {
                const icon = node("span", `${d.enabled ? "BS" : "OFF"} ${d.name} [${s.id.slice(0, 8)}] ${channels(d)}`);
                icon.className = "station-map-label"; icon.style.borderColor = d.enabled ? color(s.id) : "#65717c";
                marker.setIcon(L.divIcon({ html: icon, className: "station-icon", iconSize: [160, 30], iconAnchor: [12, 15] }));
                marker.options.title = label;
            }
            marker.setLatLng([d.latitude, d.longitude]);
            if (el("show-stations").checked) marker.addTo(map); else map.removeLayer(marker);
            if (marker.getElement()) { marker.getElement().setAttribute("aria-label", label); marker.getElement().title = label; }
            let entry = coverage.get(s.id);
            if (!entry || entry.revision !== s.rfRevision) {
                if (entry) map.removeLayer(entry.layer);
                entry = { layer: L.layerGroup(), revision: s.rfRevision,
                    contours: s.coverage.map(contour => ({ contour, shape: L.geoJSON(contour.geometry, { interactive: false }) })) };
                coverage.set(s.id, entry);
            }
            const emphasized = !selection.length || selection.includes(s.id);
            for (const { shape, contour } of entry.contours) {
                const ch = d[`channel${contour.channel}`];
                if (d.enabled && ch.enabled && contour.channel === el("coverage-channel").value) {
                    shape.setStyle({ color: color(s.id), weight: emphasized ? 2 : 1, opacity: emphasized ? 0.85 : 0.3,
                        fillOpacity: contour.threshold === 0.9 ? (emphasized ? 0.13 : 0.04) : 0, dashArray: contour.threshold === 0.5 ? "7 5" : null });
                    if (!entry.layer.hasLayer(shape)) entry.layer.addLayer(shape);
                } else entry.layer.removeLayer(shape);
            }
            if (el("show-coverage").checked) entry.layer.addTo(map); else map.removeLayer(entry.layer);
        }
        const observed = new Set();
        for (const t of visibleTargets()) {
            const n = t.report.navigation;
            if (n.latitude == null || n.longitude == null) continue;
            observed.add(t.mmsi);
            let marker = targets.get(t.mmsi);
            if (!marker) {
                marker = L.circleMarker([n.latitude, n.longitude], { radius: 7 }).addTo(map);
                marker.bindTooltip(node("span", ""));
                marker.on("click", () => { detail = { kind: "target", id: t.mmsi }; renderDetails(); }); targets.set(t.mmsi, marker);
            }
            marker.setLatLng([n.latitude, n.longitude]);
            marker.setStyle({ color: t.status === "fresh" ? "#083b66" : "#525c65", fillColor: "#168aad",
                fillOpacity: t.status === "fresh" ? 0.9 : 0.25, dashArray: t.status === "lost" ? "3 3" : null });
            marker.getTooltip().getContent().textContent = `MMSI ${t.mmsi}: ${t.status}, last received ${utc(t.report.receivedAt)}`;
        }
        for (const [id, m] of targets) if (!observed.has(id)) { map.removeLayer(m); targets.delete(id); }
        if (!framed) { fit(false); framed = true; }
    }
    function fit(withCoverage) {
        if (!map || !snapshot) return;
        const bounds = L.latLngBounds([]);
        if (withCoverage) {
            for (const entry of coverage.values()) entry.layer.eachLayer(shape => {
                const contourBounds = shape.getBounds();
                if (contourBounds.isValid()) bounds.extend(contourBounds);
            });
        } else {
            for (const s of snapshot.stations) bounds.extend([s.definition.latitude, s.definition.longitude]);
            for (const m of targets.values()) bounds.extend(m.getLatLng());
        }
        if (!bounds.isValid()) {
            const b = snapshot.settings.spawnBounds; bounds.extend([b.south, b.west]); bounds.extend([b.north, b.east]);
        }
        map.fitBounds(bounds, { padding: [30, 30], maxZoom: 11 });
    }
    function renderStations() {
        const filter = el("station-filter").value.toLowerCase(), sort = el("station-sort").value;
        const rows = snapshot.stations.filter(s => stationLabel(s.id).toLowerCase().includes(filter));
        rows.sort((a, b) => {
            if (sort === "height") return b.definition.antennaHeightMeters - a.definition.antennaHeightMeters;
            if (sort === "received") return BigInt(a.recent.received) === BigInt(b.recent.received) ? 0 : BigInt(a.recent.received) > BigInt(b.recent.received) ? -1 : 1;
            if (sort === "ratio") return (b.recent.receiveRatio ?? -1) - (a.recent.receiveRatio ?? -1);
            return stationLabel(a.id).localeCompare(stationLabel(b.id));
        });
        table("station-comparison", rows.map(s => {
            const d = s.definition;
            const observations = snapshot.targets.flatMap(t => t.stations.filter(p => p.stationId === s.id));
            const count = state => observations.filter(p => p.status === state).length;
            const dates = [...observations, ...snapshot.recentReceptions.filter(r => r.stationId === s.id)].map(r => r.receivedAt).sort();
            const counts = s.selected ? `${count("fresh")} / ${count("stale")} / ${s.lostTargets}` : `${s.currentTargets} current (fresh + stale) / ${s.lostTargets} lost`;
            return [s.id, [action(stationLabel(s.id), () => choose([s.id], { kind: "station", id: s.id })),
                `${d.enabled ? "Enabled" : "Disabled"}; ${channels(d)}`,
                `${fmt(d.antennaHeightMeters, "m ASL")} / ${fmt(d.receiveGainDbi, "dBi")} / ${fmt(d.feederLossDb, "dB")}`,
                `${fmt(d.channelA.sensitivityDbm, "dBm")} / ${fmt(d.channelB.sensitivityDbm, "dBm")}`,
                counts, `${s.recent.received} / ${percent(s.recent.receiveRatio)}`, utc(dates.at(-1))]];
        }));
    }
    function renderTargets() {
        text("target-counts", `${snapshot.currentTargets} current (fresh or stale), ${snapshot.lostTargets} lost; ${snapshot.selection.length} receiving stations. ${snapshot.targets.filter(t => t.report.navigation.latitude == null || t.report.navigation.longitude == null).length} without a received position fix.`);
        table("target-body", visibleTargets().map(t => [t.mmsi, [
            action(String(t.mmsi), () => { detail = { kind: "target", id: t.mmsi }; renderDetails(); }), t.report.scenario.name,
            `${utc(t.report.receivedAt)} / ${age(t.ageMs)}`, t.status,
            `${t.report.channel} / ${fmt(t.report.signal.estimatedPowerDbm, "dBm")}`, navText(t.report.navigation),
        ]]));
    }
    function stationSummary(s) {
        const d = s.definition, settings = snapshot.settings, c = s.counters;
        const lines = [stationLabel(s.id), `${d.enabled ? "Enabled" : "Disabled"}; location ${d.latitude}, ${d.longitude}`,
            `Config revision ${s.configRevision}; RF revision ${s.rfRevision}; RF updated ${utc(s.rfUpdatedAt)}`,
            `Antenna ${fmt(d.antennaHeightMeters, "m ASL")}; gain ${fmt(d.receiveGainDbi, "dBi")}; feeder loss ${fmt(d.feederLossDb, "dB")}`,
            `Shadow sectors: ${d.shadowSectors.map(v => `${v.startDegrees}-${v.endDegrees} deg: ${v.lossDb} dB`).join("; ") || "none"}`];
        for (const name of ["A", "B"]) {
            const ch = d[`channel${name}`];
            lines.push(`Channel ${name}: ${fmt(settings.reception[`channel${name}FrequencyMhz`], "MHz", 3)}, ${ch.enabled ? "enabled" : "disabled"}; sensitivity ${ch.sensitivityDbm} dBm; noise penalty ${ch.noisePenaltyDb} dB; extra drop ${percent(ch.dropProbability)}`);
        }
        const tx = settings.transmitter, model = settings.reception;
        lines.push("More negative sensitivity receives weaker signals.",
            `Reference transmitter: ${fmt(tx.powerWatts, "W")}; antenna ${fmt(tx.heightMeters, "m ASL")}; gain ${fmt(tx.gainDbi, "dBi")}; feeder loss ${fmt(tx.feederLossDb, "dB")}`,
            `Published reception model parameters: site loss ${fmt(model.siteLossDb, "dB")}; path exponent ${fmt(model.pathExponent)}; effective Earth radius factor ${fmt(model.effectiveEarthRadiusFactor, "", 3)}; horizon taper starts at ${percent(model.horizonTaperStart)} of the radio horizon; zero-probability margin ${fmt(model.zeroProbabilityMarginDb, "dB")}; reference probability ${percent(model.referenceProbability)}; full-probability margin ${fmt(model.fullProbabilityMarginDb, "dB")}`);
        for (const c of s.coverage) lines.push(`Channel ${c.channel}, ${percent(c.threshold)}: ${fmt(c.minRadiusMeters / 1000)}-${fmt(c.maxRadiusMeters / 1000, "km")}; ${fmt(c.minRadiusMeters / 1852)}-${fmt(c.maxRadiusMeters / 1852, "NM")}`);
        lines.push(`Since creation: ${c.received} received / ${c.opportunities} opportunities (${percent(s.receiveRatio)})`,
            `Channel A: ${c.channelA.received}/${c.channelA.opportunities}; B: ${c.channelB.received}/${c.channelB.opportunities}`,
            `Virtual rate window: ${age(s.recent.durationMs)} of ${age(settings.observation.rateWindowMs)}; ${s.recent.received}/${s.recent.opportunities} (${percent(s.recent.receiveRatio)}); ${fmt(s.recent.receptionRate, "receptions/s")}, ${fmt(s.recent.opportunityRate, "opportunities/s")}`,
            `Targets: ${s.currentTargets} fresh + stale; ${s.lostTargets} lost`,
            `Simulation diagnostics: disabled site ${c.stationDisabled}; disabled channel ${c.channelDisabled}; outside horizon ${c.outsideHorizon}; insufficient margin ${c.insufficientMargin}; probabilistic loss ${c.probabilisticLoss}`,
            "Lifetime counters and the recent window can include earlier RF configurations. Observed targets and received messages appear below.");
        return lines.join("\n\n");
    }
    function renderDetails() {
        el("provenance-table").hidden = true;
        if (!detail) { text("detail-title", "Station and target details"); text("detail-summary", "Select a station or target."); return; }
        if (detail.kind === "station") {
            const station = snapshot.stations.find(s => s.id === detail.id);
            text("detail-title", "Station capabilities and reception");
            text("detail-summary", station ? stationSummary(station) : "This station was removed."); return;
        }
        text("detail-title", `AIS target ${detail.id}`);
        const t = snapshot.targets.find(t => t.mmsi === detail.id);
        if (!t) { text("detail-summary", "The last-known target expired or is not observed by the selected stations."); return; }
        const r = t.report, category = snapshot.scenarioCategories.find(c => c.id === r.scenario.categoryId)?.name || r.scenario.categoryId;
        text("detail-summary", [`MMSI ${t.mmsi}: ${t.status}; age ${age(t.ageMs)}`, navText(r.navigation),
            `Last received: ${utc(r.receivedAt)} by ${stationLabel(r.stationId)}`,
            `Channel ${r.channel}; estimated power ${fmt(r.signal.estimatedPowerDbm, "dBm")}`,
            `Scenario label (not AIS): ${r.scenario.name}; ${category}`,
            "Provenance rows refer to each station's latest report. An older report may have a different position."].join("\n\n"));
        el("provenance-table").hidden = false;
        table("provenance-body", t.stations.map(p => [p.stationId, [
            action(stationLabel(p.stationId), () => choose([p.stationId], { kind: "target", id: t.mmsi })),
            `${utc(p.receivedAt)} (${p.status})`, p.chosen ? "Chosen transmission" : "Older report",
            `${p.channel} / ${fmt(p.estimatedPowerDbm, "dBm")}`,
            `${p.rfRevision}${p.currentRfRevision ? " (current)" : " (earlier configuration)"}${p.stationEnabled ? "" : "; site now disabled"}`,
        ]]));
    }
    function renderMessage() {
        el("copy-nmea").disabled = !message;
        if (!message) { text("message-title", "Select a received message"); text("message-summary", ""); el("raw-nmea").value = ""; return; }
        const r = message;
        text("message-title", `Received AIS type ${r.messageType}: MMSI ${r.mmsi}`);
        text("message-summary", [`Station at reception: ${r.receiver.name} [${r.stationId}]`,
            `Exact virtual receive/transmit time: ${utc(r.receivedAt)}`,
            `Transmission ${r.transmissionSequence}; reception ${r.sequence}; channel ${r.channel}`, navText(r.navigation),
            `Scenario label (not AIS): ${r.scenario.name} / ${r.scenario.categoryId}`,
            `Estimated RF: power ${fmt(r.signal.estimatedPowerDbm, "dBm")}; effective sensitivity ${fmt(r.signal.effectiveSensitivityDbm, "dBm")}; margin ${fmt(r.signal.marginDb, "dB")}; probability ${percent(r.signal.probability)}; distance ${fmt(r.signal.distanceMeters, "m")}; bearing ${fmt(r.signal.bearingDegrees, "deg")}; horizon ${fmt(r.signal.horizonMeters, "m")}; shadow loss ${fmt(r.signal.shadowLossDb, "dB")}`,
            `Reception-time config revision ${r.configRevision}; RF revision ${r.rfRevision}. These settings remain attributed to this message after edits.`,
            `Receiver location ${r.receiver.latitude}, ${r.receiver.longitude}; antenna ${fmt(r.receiver.antennaHeightMeters, "m ASL")}; gain ${fmt(r.receiver.receiveGainDbi, "dBi")}; feeder loss ${fmt(r.receiver.feederLossDb, "dB")}`,
            `Receiver channel: ${r.receiver.channel.enabled ? "enabled" : "disabled"}; sensitivity ${fmt(r.receiver.channel.sensitivityDbm, "dBm")}; noise penalty ${fmt(r.receiver.channel.noisePenaltyDb, "dB")}; extra drop ${percent(r.receiver.channel.dropProbability)}`].join("\n\n"));
        if (el("raw-nmea").value !== r.sentence) el("raw-nmea").value = r.sentence;
    }
    function renderMessages() {
        const filter = el("message-filter").value.trim();
        table("message-body", [...historyRows].reverse().filter(r => String(r.mmsi).includes(filter)).map(r => [`${r.stationId}:${r.sequence}`, [
            action(`${r.receiver.name} [${r.stationId}]`, () => { message = r; text("copy-status", ""); renderMessage(); }), utc(r.receivedAt), r.mmsi,
            `${r.messageType} / ${r.channel}`, `${fmt(r.signal.estimatedPowerDbm, "dBm")} / ${fmt(r.signal.marginDb, "dB")}`, `${r.transmissionSequence} / ${r.sequence}`,
        ]]));
    }
    function render() {
        if (!snapshot) return;
        renderSelectors(); renderMap(); renderStations(); renderTargets(); renderDetails();
        if (!historyStation && !el("pause-history").checked) historyRows = snapshot.recentReceptions.slice(-200);
        renderMessages(); el("empty-stations").hidden = snapshot.stations.length !== 0;
        const tx = snapshot.settings.transmitter;
        text("coverage-legend", `Estimated per-message reception, reference transmitter: ${tx.powerWatts} W, antenna ${tx.heightMeters} m. Channel ${el("coverage-channel").value}. Filled: 90%; dashed outline: 50%. Station colors match the named selection controls. Disabled sites/channels have no receive area.`);
        text("sim-clock", `Simulation time: ${utc(snapshot.time.now)} | ${snapshot.time.paused ? "Paused" : `${snapshot.time.speed}x`}`);
    }
    function requestRefresh() {
        if (fetching) { immediate = true; return; }
        clearTimeout(timer); refresh();
    }
    // One main request at a time. Invalidated selections cannot apply results or
    // errors. Failed reads retain the previous complete snapshot and virtual time.
    async function refresh() {
        fetching = true; immediate = false;
        const token = generation, abort = new AbortController(); controller = abort;
        const timeout = setTimeout(() => abort.abort(), 7000);
        try {
            const response = await fetch(`/display/api/observations?stations=${encodeURIComponent(selection.join(",") || "all")}`, { cache: "no-store", signal: abort.signal });
            if (token !== generation) return;
            if (response.status === 404 && selection.length) {
                choose([]); text("selection-notice", "A selected station was removed. Switching to all stations."); return;
            }
            if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
            const next = await response.json();
            if (token !== generation) return;
            if (snapshot && snapshot.simulationId !== next.simulationId) {
                clearMap(); colors.clear(); framed = false; detail = null; resetHistory();
                const hadSelection = selection.length > 0;
                snapshot = null; selection = [];
                text("selection-notice", "Simulator restarted. Station selection and history were reset.");
                if (hadSelection) {
                    for (const id of ["station-comparison", "target-body", "provenance-body"]) table(id, []);
                    el("station-selection").replaceChildren();
                    for (const option of [...el("history-station").options].slice(1)) option.remove();
                    renderDetails();
                    text("sim-clock", "Simulation time: waiting for the new run");
                    text("target-counts", "Waiting for the new run's received observations.");
                    text("live-status", "Simulator restarted. Loading all stations...");
                    generation++; immediate = true; return;
                }
            }
            if (historyStation && !next.stations.some(s => s.id === historyStation)) {
                resetHistory(); text("history-status", "History station removed. Showing recent selection sample.");
            }
            snapshot = next; historyBlocked = false; lastFetched = new Date();
            el("display-view").classList.remove("stale", "pending");
            if (el("selection-notice").textContent === "Loading selected receiving stations...") text("selection-notice", "");
            render(); text("fetch-time", `Browser last fetched: ${lastFetched.toLocaleString()}`);
            text("live-status", "Connected. Showing a complete observation snapshot.");
        } catch (error) {
            if (token !== generation) return;
            el("display-view").classList.add("stale");
            text("live-status", `Observation updates unavailable (${error.message}). ${lastFetched ? "Entire view is stale; retaining the last complete snapshot." : "No observation data yet."} Retrying...`);
        } finally {
            clearTimeout(timeout); controller = null; fetching = false;
            timer = setTimeout(refresh, immediate ? 0 : 1000);
        }
    }
    // Cursors remain decimal strings and belong to one station and run. The
    // browser stores at most 200 rows plus the immutable, explicitly pinned report.
    async function refreshHistory() {
        if (historyFetching || !snapshot || !historyStation || historyBlocked || el("pause-history").checked || el("display-view").classList.contains("pending") || el("display-view").classList.contains("stale")) return;
        historyFetching = true;
        const token = historyGeneration, selectionToken = generation, run = snapshot.simulationId, id = historyStation;
        const abort = new AbortController(); historyController = abort;
        const timeout = setTimeout(() => abort.abort(), 7000);
        const current = () => token === historyGeneration && selectionToken === generation && run === snapshot?.simulationId && id === historyStation && !el("pause-history").checked && !el("display-view").classList.contains("stale") && !el("display-view").classList.contains("pending");
        try {
            const query = new URLSearchParams({ simulationId: run, limit: "200" });
            if (historyCursor !== null) query.set("after", historyCursor);
            const response = await fetch(`/display/api/stations/${encodeURIComponent(id)}/receptions?${query}`, { cache: "no-store", signal: abort.signal });
            if (!current()) return;
            if (response.status === 409 || response.status === 404) {
                resetHistory(id); historyBlocked = true;
                text("history-status", "History identity changed. Cursor cleared; waiting for current station snapshot.");
                requestRefresh(); return;
            }
            if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
            const page = await response.json();
            if (!current()) return;
            if (page.simulationId !== run || page.stationId !== id) {
                resetHistory(id); historyBlocked = true;
                text("history-status", "History response identity mismatch. Cursor cleared; refreshing station snapshot.");
                requestRefresh(); return;
            }
            if (page.gap) {
                historyRows = [];
                text("history-status", `History gap. Retained sequences ${page.oldestAvailable ?? "none"}-${page.latestAvailable ?? "none"}; truncated before ${page.truncatedBefore ?? "none"}. Current target observations are unaffected.`);
            } else if (!el("history-status").textContent.startsWith("History gap")) {
                text("history-status", `Station history sample; retained sequences ${page.oldestAvailable ?? "none"}-${page.latestAvailable ?? "none"}.${page.hasMore ? " More available; catching up." : ""}`);
            }
            const unique = new Map([...historyRows, ...page.receptions].map(r => [r.sequence, r]));
            historyRows = [...unique.values()].slice(-200); historyCursor = page.nextAfter; renderMessages();
        } catch (error) {
            if (current()) text("history-status", `History updates unavailable (${error.message}). Keeping loaded sample; retrying.`);
        } finally {
            clearTimeout(timeout); historyFetching = false;
            if (historyController === abort) historyController = null;
        }
    }
    el("all-stations").onclick = () => choose([]);
    el("fit-targets").onclick = () => fit(false);
    el("fit-coverage").onclick = () => fit(true);
    for (const id of ["coverage-channel", "show-coverage", "show-stations", "show-lost"]) el(id).onchange = render;
    el("station-filter").oninput = () => snapshot && renderStations();
    el("station-sort").onchange = () => snapshot && renderStations();
    el("message-filter").oninput = renderMessages;
    el("history-station").onchange = () => { resetHistory(el("history-station").value); render(); refreshHistory(); };
    el("pause-history").onchange = () => {
        historyGeneration++; historyController?.abort();
        if (!el("pause-history").checked) { render(); refreshHistory(); }
    };
    el("copy-nmea").onclick = async () => {
        if (!message) return;
        try { await navigator.clipboard.writeText(message.sentence); text("copy-status", "Copied exact NMEA, including CRLF."); }
        catch (error) { el("raw-nmea").focus(); el("raw-nmea").select(); text("copy-status", `Clipboard unavailable (${error.message}). Select and copy the NMEA text.`); }
    };
    async function historyLoop() { await refreshHistory(); setTimeout(historyLoop, 1000); }
    refresh(); historyLoop();
})();
