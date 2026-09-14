(() => {
    const countForm = document.getElementById("vessel-form");
    const countInput = document.getElementById("vessel-count");
    const applyCount = document.getElementById("apply-count");
    const countStatus = document.getElementById("save-status");
    const speedForm = document.getElementById("speed-form");
    const speedInput = document.getElementById("speed");
    const presets = [...speedForm.querySelectorAll("[data-speed]")];
    const speedButtons = [document.getElementById("apply-speed"), ...presets];
    const speedStatus = document.getElementById("speed-status");
    const clock = document.getElementById("sim-clock");
    const fleetStatus = document.getElementById("fleet-status");
    const messages = document.getElementById("messages");

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

    async function request(path, options = {}) {
        const response = await fetch(path, { cache: "no-store", signal: AbortSignal.timeout(5000), ...options });
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
        clock.classList.toggle("stale", stale);
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
        document.getElementById("count-range").textContent = `(0-${maxVessels})`;
        speedInput.max = speed.max;
        speedInput.step = speed.step;
        document.getElementById("speed-range").textContent = `(0 or ${speed.min}-${speed.max})`;
        document.getElementById("history-note").textContent =
            `The latest ${messageHistoryLimit.toLocaleString()} reports are held in memory. The last 10 are shown here.`;
    }

    countInput.addEventListener("input", () => { dirty.count = true; });
    speedInput.addEventListener("input", () => { dirty.speed = true; });

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
            if (value.simulationId !== runId) {
                statusElement.textContent = "The simulator restarted while saving. Review the current values.";
                return;
            }
            dirty[kind] = false;
            statusElement.textContent = show(value);
        } catch (error) {
            statusElement.textContent = `Could not save: ${error.message}`;
        } finally {
            saving[kind] = false;
            generation++;
            updateControls();
        }
    }

    countForm.addEventListener("submit", event => {
        event.preventDefault();
        write("count", countStatus, "/api/vessels", { count: Number(countInput.value) }, fleet => {
            countInput.value = fleet.vessels.length;
            showFleet(fleet);
            return `Vessel count set to ${fleet.vessels.length}.`;
        });
    });
    speedForm.addEventListener("submit", event => {
        event.preventDefault();
        write("speed", speedStatus, "/api/time", { speed: Number(speedInput.value) }, metadata => {
            speedInput.value = metadata.time.speed;
            lastTime = metadata.time;
            showClock(metadata.time, false);
            return metadata.time.paused ? "Simulation paused." : `Speed set to ${metadata.time.speed}x.`;
        });
    });
    for (const button of presets) {
        button.addEventListener("click", () => {
            speedInput.value = button.dataset.speed;
            dirty.speed = true;
            speedForm.requestSubmit();
        });
    }

    async function refresh() {
        const started = generation;
        try {
            const [fleet, history, metadata] = await Promise.all([
                request("/api/vessels"), request("/api/messages"), request("/api/metadata"),
            ]);
            // A write started or finished during this poll; its result wins.
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
            fleetStatus.textContent = `Updates unavailable (${error.message}). Retrying...`;
            if (lastTime) showClock(lastTime, true);
        } finally {
            window.setTimeout(refresh, 1000);
        }
    }
    refresh();
})();
