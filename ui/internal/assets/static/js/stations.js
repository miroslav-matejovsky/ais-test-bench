// startStations runs the manager's station table and editor inside a mounted
// manager runtime.
export function startStations(runtime) {
    const ref = runtime.ref;
    const form = ref("station-form");
    const fields = ref("station-fields");
    const status = ref("station-status");
    const editor = ref("station-editor");
    const conflictPanel = ref("station-conflict");
    const sectorsElement = ref("station-sectors");
    const controls = new Map();
    let snapshot = null;
    let draft = null; // Identity/revision at the time the draft was opened, never rebased by polling.
    let saving = false;
    let generation = 0;
    let conflict = false;
    runtime.onDestroy(() => { generation++; });

    function element(tag, text) {
        const node = document.createElement(tag);
        if (text !== undefined) node.textContent = text;
        return node;
    }
    // input creates a labelled control. IDs derive from the root ID, so several
    // managers on one page keep unique label and error associations.
    function input(parent, path, label, type, min, max) {
        const wrapper = element("div");
        const field = element("input");
        field.id = runtime.id(`station-${path.replaceAll(".", "-")}`);
        field.type = type;
        if (type !== "checkbox") field.required = true;
        if (type === "number") {
            field.step = "any";
            field.min = min;
            field.max = max;
        }
        const caption = element("label", label);
        caption.htmlFor = field.id;
        const error = element("span");
        error.className = "ais-field-error";
        error.id = `${field.id}-error`;
        field.setAttribute("aria-describedby", error.id);
        wrapper.append(caption, field, error);
        parent.append(wrapper);
        controls.set(path, { field, error });
        return field;
    }
    const site = ref("station-site-fields");
    input(site, "name", "Name (up to 80 characters)", "text");
    input(site, "enabled", "Station enabled", "checkbox");
    input(site, "latitude", "Latitude (degrees)", "number", -85, 85);
    input(site, "longitude", "Longitude (degrees)", "number", -180, 180);
    input(site, "antennaHeightMeters", "Antenna height above sea level (m, > 0)", "number", 0.000001, 500);
    input(site, "receiveGainDbi", "Receive gain (dBi)", "number", -10, 20);
    input(site, "feederLossDb", "Feeder loss (dB)", "number", 0, 30);
    for (const channel of ["A", "B"]) {
        const group = element("fieldset");
        group.append(element("legend", `Channel ${channel} (${channel === "A" ? "161.975" : "162.025"} MHz)`));
        ref("station-channel-fields").append(group);
        const key = `channel${channel}`;
        input(group, `${key}.enabled`, "Channel enabled", "checkbox");
        input(group, `${key}.sensitivityDbm`, "Sensitivity (dBm)", "number", -125, -80);
        input(group, `${key}.noisePenaltyDb`, "Noise penalty (dB)", "number", 0, 40);
        input(group, `${key}.dropProbability`, "Extra packet-drop probability (0-1)", "number", 0, 1);
    }

    function sectorRow(sector) {
        const row = element("div");
        row.className = "ais-station-sector ais-station-grid";
        const index = sectorsElement.children.length;
        for (const [key, label, max] of [["startDegrees", "Start bearing", 359.999999], ["endDegrees", "End bearing", 359.999999], ["lossDb", "Loss (dB)", 60]]) {
            const field = input(row, `shadowSectors.${index}.${key}`, label, "number", 0, max);
            field.value = sector[key];
            field.dataset.sectorField = key;
        }
        const remove = element("button", "Remove sector");
        remove.type = "button";
        runtime.listen(remove, "click", () => {
            const sectors = readSectors();
            sectors.splice([...row.parentNode.children].indexOf(row), 1);
            renderSectors(sectors);
        });
        row.append(remove);
        sectorsElement.append(row);
    }
    function readSectors() {
        return [...sectorsElement.children].map(row => Object.fromEntries(
            [...row.querySelectorAll("input")].map(field => [field.dataset.sectorField, Number(field.value)])));
    }
    function renderSectors(sectors) {
        for (const key of controls.keys()) if (key.startsWith("shadowSectors.")) controls.delete(key);
        sectorsElement.replaceChildren();
        for (const sector of sectors) sectorRow(sector);
        ref("station-add-sector").disabled = sectors.length >= 8;
    }
    function definition() {
        const result = { channelA: {}, channelB: {}, shadowSectors: readSectors() };
        for (const [path, { field }] of controls) {
            if (path.startsWith("shadowSectors.")) continue;
            const value = field.type === "checkbox" ? field.checked : field.type === "number" ? Number(field.value) : field.value;
            const parts = path.split(".");
            if (parts.length === 2) result[parts[0]][parts[1]] = value;
            else result[path] = value;
        }
        return result;
    }
    function clearErrors() {
        for (const { field, error } of controls.values()) {
            error.textContent = "";
            field.removeAttribute("aria-invalid");
        }
        ref("station-definition-error").textContent = "";
        ref("station-sector-error").textContent = "";
    }
    function fieldErrors(errors = {}) {
        for (const [path, message] of Object.entries(errors)) {
            const control = controls.get(path);
            if (control) {
                control.error.textContent = message;
                control.field.setAttribute("aria-invalid", "true");
            } else if (path.startsWith("shadowSectors")) ref("station-sector-error").textContent += `${message}\n`;
            else ref("station-definition-error").textContent += `${path}: ${message}\n`;
        }
    }
    async function request(path, options = {}) {
        const response = await runtime.fetch(path, options);
        const body = await response.text();
        let value;
        try { value = JSON.parse(body); } catch { throw new Error(body || `HTTP ${response.status}`); }
        if (!response.ok) {
            const error = new Error(value.error || `HTTP ${response.status}`);
            error.status = response.status;
            error.fields = value.fields;
            throw error;
        }
        return value;
    }
    function updateControls() {
        ref("station-new").disabled = !snapshot || saving || snapshot.stations.length >= snapshot.settings.maxStations;
        fields.disabled = !draft || saving;
        ref("station-save").disabled = !draft || saving || conflict;
        ref("station-rebase").disabled = saving;
        for (const button of ref("stations-body").querySelectorAll("button")) button.disabled = saving;
    }
    function showConflict() {
        conflict = true;
        conflictPanel.hidden = false;
        const current = snapshot.stations.find(s => s.id === draft.id);
        const sameRun = snapshot.simulationId === draft.simulationId;
        ref("station-current").textContent = JSON.stringify({ simulationId: snapshot.simulationId, stationSetRevision: snapshot.stationSetRevision,
            current: sameRun && current ? current.definition : "Original station unavailable. Keeping the draft will create a new station." }, null, 2);
        updateControls();
    }
    function accept(value) {
        const changed = !snapshot || value.simulationId !== snapshot.simulationId || value.stationSetRevision !== snapshot.stationSetRevision;
        snapshot = value;
        if (draft && (draft.simulationId !== value.simulationId || draft.revision !== value.stationSetRevision)) showConflict();
        if (changed) renderTable();
        updateControls();
        status.textContent = `${value.stations.length} / ${value.settings.maxStations} stations | Configuration at ${value.time.now} | Fetched ${new Date().toLocaleTimeString()}`;
    }
    function begin(station) {
        if (!snapshot || saving) return;
        // Switching forms is explicit; protect an existing unsaved draft.
        if (draft) { status.textContent = "Save or discard the current draft before selecting another station."; return; }
        const channel = { enabled: true, sensitivityDbm: -110, noisePenaltyDb: 0, dropProbability: 0 };
        const d = station?.definition || { name: "New station", enabled: true, latitude: 51.98, longitude: 4.05,
            antennaHeightMeters: 25, receiveGainDbi: 3, feederLossDb: 2, channelA: { ...channel }, channelB: { ...channel }, shadowSectors: [] };
        draft = { id: station?.id || null, simulationId: snapshot.simulationId, revision: snapshot.stationSetRevision };
        conflict = false;
        conflictPanel.hidden = true;
        for (const [path, { field }] of controls) {
            if (path.startsWith("shadowSectors.")) continue;
            const value = path.split(".").reduce((obj, key) => obj[key], d);
            if (field.type === "checkbox") field.checked = value;
            else field.value = value;
        }
        renderSectors(d.shadowSectors);
        clearErrors();
        ref("station-edit-label").textContent = draft.id ? `Editing ${station.definition.name} (${draft.id}), revision ${draft.revision}` : "Adding a receiving site";
        editor.open = true;
        updateControls();
        controls.get("name").field.focus();
    }
    function renderTable() {
        const rows = snapshot.stations.map(station => {
            const d = station.definition;
            const row = element("tr");
            row.dataset.key = station.id;
            const channels = [d.channelA.enabled ? "A" : "", d.channelB.enabled ? "B" : ""].filter(Boolean).join(" + ") || "none";
            for (const text of [`${d.name} (${station.id})${d.enabled ? "" : " - disabled"}`, `${d.latitude}, ${d.longitude}`, channels,
                `${d.antennaHeightMeters} m; A/B ${d.channelA.sensitivityDbm}/${d.channelB.sensitivityDbm} dBm; gain ${d.receiveGainDbi} dBi; feeder ${d.feederLossDb} dB`]) row.append(element("td", text));
            const actions = element("td");
            for (const [label, run] of [["Edit", () => begin(station)], [d.enabled ? "Disable" : "Enable", () => mutate(station, { ...d, enabled: !d.enabled })],
                ["Disable B preset", () => mutate(station, { ...d, channelB: { ...d.channelB, enabled: false } })], ["Delete", () => mutate(station, null)]]) {
                const button = element("button", label);
                button.type = "button";
                runtime.listen(button, "click", run);
                actions.append(button);
            }
            row.append(actions);
            return row;
        });
        ref("stations-body").replaceChildren(...rows);
    }
    async function save(id, value, identity) {
        if (saving) return;
        saving = true;
        generation++;
        clearErrors();
        updateControls();
        status.textContent = "Saving station change...";
        try {
            const path = id ? `stations/${encodeURIComponent(id)}` : "stations";
            const deleting = value === null;
            const query = new URLSearchParams({ simulationId: identity.simulationId, stationSetRevision: identity.revision });
            const result = await request(deleting ? `${path}?${query}` : path, {
                method: deleting ? "DELETE" : id ? "PUT" : "POST",
                headers: { "Content-Type": "application/json" },
                ...(deleting ? {} : { body: JSON.stringify({ simulationId: identity.simulationId, stationSetRevision: identity.revision, definition: value }) }),
            });
            if (runtime.destroyed) return;
            draft = null;
            conflict = false;
            conflictPanel.hidden = true;
            editor.open = false;
            accept(result);
            status.textContent += " | Change applied.";
        } catch (error) {
            if (runtime.destroyed) return;
            fieldErrors(error.fields);
            status.textContent = `Could not save: ${error.message}`;
            if (error.status === 409) {
                try {
                    const current = await request("stations");
                    if (runtime.destroyed) return;
                    accept(current);
                } catch (refreshError) {
                    if (runtime.destroyed) return;
                    status.textContent += `; refresh failed: ${refreshError.message}`;
                }
                if (draft) showConflict();
                status.textContent = "Configuration changed. Review current values before retrying.";
            }
        } finally {
            if (!runtime.destroyed) {
                saving = false;
                generation++;
                updateControls();
            }
        }
    }
    function mutate(station, value) {
        if (draft) { status.textContent = "Save or discard the open draft before another station change."; return; }
        save(station.id, value, { simulationId: snapshot.simulationId, revision: snapshot.stationSetRevision });
    }
    runtime.listen(form, "submit", event => {
        event.preventDefault();
        if (draft && !conflict && form.reportValidity()) save(draft.id, definition(), draft);
    });
    runtime.listen(ref("station-new"), "click", () => begin(null));
    runtime.listen(ref("station-cancel"), "click", () => {
        draft = null; conflict = false; conflictPanel.hidden = true; editor.open = false; clearErrors(); updateControls();
    });
    runtime.listen(ref("station-rebase"), "click", () => {
        if (!draft || saving) return;
        if (draft.simulationId !== snapshot.simulationId || !snapshot.stations.some(s => s.id === draft.id)) draft.id = null;
        draft.simulationId = snapshot.simulationId;
        draft.revision = snapshot.stationSetRevision;
        conflict = false;
        conflictPanel.hidden = true;
        ref("station-edit-label").textContent = draft.id ? `Draft for ${draft.id}; reviewed revision ${draft.revision}` : "Draft will create a new station";
        updateControls();
    });
    runtime.listen(ref("station-add-sector"), "click", () => {
        const sectors = readSectors();
        if (sectors.length < 8) renderSectors([...sectors, { startDegrees: 0, endDegrees: 30, lossDb: 15 }]);
    });
    async function refresh() {
        const started = generation;
        try {
            const result = await request("stations");
            if (started === generation && !saving) accept(result);
        } catch (error) {
            if (started === generation && !saving) status.textContent = `Station updates unavailable (${error.message}). Showing last configuration; retrying...`;
        } finally { runtime.later(refresh, 1000); }
    }
    updateControls();
    refresh();
}
