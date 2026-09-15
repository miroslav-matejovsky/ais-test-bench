import { mount } from "./runtime.js";
import { startStations } from "./stations.js";

// mountManager mounts a manager root rendered by ui.UI.RenderManager: fleet and
// speed controls, recent messages, and the station editor. It throws when root
// is not a manager root or is already mounted. options.fetch replaces the
// browser fetch for this instance, for example to add host CSRF headers.
export function mountManager(root, options = {}) {
    return mount(root, "data-ais-manager", "mountManager", options, runtime => {
        startFleet(runtime);
        startStations(runtime);
    });
}

function startFleet(runtime) {
    const ref = runtime.ref;
    const countForm = ref("vessel-form");
    const countInput = ref("vessel-count");
    const applyCount = ref("apply-count");
    const countStatus = ref("save-status");
    const speedForm = ref("speed-form");
    const speedInput = ref("speed");
    const presets = [...speedForm.querySelectorAll("[data-speed]")];
    const speedButtons = [ref("apply-speed"), ...presets];
    const speedStatus = ref("speed-status");
    const clock = ref("sim-clock");
    const fleetStatus = ref("fleet-status");
    const messages = ref("messages");

    // runId is the simulation shown. initialized enables the controls after the
    // first coherent refresh of that run.
    let runId = null;
    let initialized = false;
    let lastTime = null;
    // generation changes when a write starts and when it finishes. A poll whose
    // generation changed while it was in flight is discarded, so a response read
    // before or during a write never overwrites the write's confirmed result.
    let generation = 0;
    const saving = { count: false, speed: false };
    // A dirty field holds a user edit that polls must not overwrite.
    const dirty = { count: false, speed: false };
    runtime.onDestroy(() => { generation++; });

    async function request(path, options = {}) {
        const response = await runtime.fetch(path, options);
        if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
        return response.json();
    }

    // Simulation times are virtual UTC instants, shown as UTC text from the
    // server value and never extrapolated.
    function formatUTC(iso) {
        return `${iso.slice(0, 10)} ${iso.slice(11, 19)} UTC`;
    }
    function showClock(time, stale) {
        const speed = time.paused ? "paused" : `${time.speed}x`;
        clock.textContent = `Simulation time: ${formatUTC(time.now)} | Effective speed: ${speed}${stale ? " | stale" : ""}`;
        clock.classList.toggle("ais-stale", stale);
    }
    // The receipt time is real local time and shows connection freshness.
    function showFleet(fleet) {
        fleetStatus.textContent = `${fleet.vessels.length} active vessel(s) | ${fleet.messageCount} / ${fleet.messageLimit} messages stored | Received ${new Date().toLocaleTimeString()}`;
    }
    // No poll or initialization enables a button while its write is pending.
    function updateControls() {
        countInput.disabled = !initialized;
        applyCount.disabled = !initialized || saving.count;
        speedInput.disabled = !initialized;
        for (const button of speedButtons) button.disabled = !initialized || saving.speed;
    }
    // showValue shows the effective server value unless the user edits the field.
    function showValue(input, kind, value) {
        if (!dirty[kind] && !saving[kind] && document.activeElement !== input) input.value = value;
    }
    // Input limits come from metadata; the server still validates every value.
    function applyMetadata(metadata) {
        const { maxVessels, messageHistoryLimit, speed } = metadata.settings;
        countInput.max = maxVessels;
        ref("count-range").textContent = `(0-${maxVessels})`;
        speedInput.max = speed.max;
        speedInput.step = speed.step;
        ref("speed-range").textContent = `(0 or ${speed.min}-${speed.max})`;
        ref("history-note").textContent =
            `The latest ${messageHistoryLimit.toLocaleString()} reports are held in memory. The last 10 are shown here.`;
    }

    runtime.listen(countInput, "input", () => { dirty.count = true; });
    runtime.listen(speedInput, "input", () => { dirty.speed = true; });

    // write sends one change and keeps its buttons disabled until the server
    // answers. show renders a confirmed response and returns the status text. A
    // failed write keeps the draft for another attempt.
    async function write(kind, statusElement, path, body, show) {
        if (saving[kind]) return;
        saving[kind] = true;
        generation++;
        updateControls();
        statusElement.textContent = "Saving...";
        try {
            const value = await request(path, {
                method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body),
            });
            if (runtime.destroyed) return;
            if (value.simulationId !== runId) {
                statusElement.textContent = "The simulator restarted while saving. Review the current values.";
                return;
            }
            dirty[kind] = false;
            statusElement.textContent = show(value);
        } catch (error) {
            if (!runtime.destroyed) statusElement.textContent = `Could not save: ${error.message}`;
        } finally {
            if (!runtime.destroyed) {
                saving[kind] = false;
                generation++;
                updateControls();
            }
        }
    }

    runtime.listen(countForm, "submit", event => {
        event.preventDefault();
        write("count", countStatus, "vessels", { count: Number(countInput.value) }, fleet => {
            countInput.value = fleet.vessels.length;
            showFleet(fleet);
            return `Vessel count set to ${fleet.vessels.length}.`;
        });
    });
    runtime.listen(speedForm, "submit", event => {
        event.preventDefault();
        write("speed", speedStatus, "time", { speed: Number(speedInput.value) }, metadata => {
            speedInput.value = metadata.time.speed;
            lastTime = metadata.time;
            showClock(metadata.time, false);
            return metadata.time.paused ? "Simulation paused." : `Speed set to ${metadata.time.speed}x.`;
        });
    });
    for (const button of presets) {
        runtime.listen(button, "click", () => {
            speedInput.value = button.dataset.speed;
            dirty.speed = true;
            speedForm.requestSubmit();
        });
    }

    async function refresh() {
        const started = generation;
        try {
            const [fleet, history, metadata] = await Promise.all([request("vessels"), request("messages"), request("metadata")]);
            // A write started or finished during this poll, or the mount was
            // destroyed; the newer state wins.
            if (started !== generation) return;
            // Separate reads can straddle a restart; never render mixed runs.
            if (fleet.simulationId !== metadata.simulationId || history.simulationId !== metadata.simulationId) {
                fleetStatus.textContent = "The simulator restarted during this refresh. Retrying...";
                return;
            }
            const newRun = metadata.simulationId !== runId;
            if (newRun) {
                // Reset per-run form state before rendering the new run.
                runId = metadata.simulationId;
                initialized = false;
                dirty.count = dirty.speed = false;
                countStatus.textContent = "";
                speedStatus.textContent = "";
                countInput.value = fleet.vessels.length;
                speedInput.value = metadata.time.speed;
            }
            applyMetadata(metadata);
            lastTime = metadata.time;
            showClock(metadata.time, false);
            showFleet(fleet);
            showValue(countInput, "count", fleet.vessels.length);
            showValue(speedInput, "speed", metadata.time.speed);
            initialized = true;
            updateControls();
            // Sentences keep their CRLF in the API; it is trimmed only for display.
            messages.textContent = history.messages.slice(-10).reverse()
                .map(message => `${formatUTC(message.timestamp)}  #${message.sequence}  MMSI ${message.mmsi}  ${message.sentence.trim()}`)
                .join("\n") || "No messages stored.";
        } catch (error) {
            if (runtime.destroyed) return;
            fleetStatus.textContent = `Updates unavailable (${error.message}). Retrying...`;
            if (lastTime) showClock(lastTime, true);
        } finally {
            runtime.later(refresh, 1000);
        }
    }
    updateControls();
    refresh();
}
