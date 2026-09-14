(() => {
    const input = document.getElementById("vessel-count");
    const apply = document.getElementById("apply-count");
    const status = document.getElementById("fleet-status");
    const saveStatus = document.getElementById("save-status");
    const messages = document.getElementById("messages");
    let initialized = false;
    let saving = false;

    async function request(path, options = {}) {
        const response = await fetch(path, { cache: "no-store", signal: AbortSignal.timeout(5000), ...options });
        if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
        return response.json();
    }
    function showFleet(fleet) {
        status.textContent = `${fleet.vessels.length} active vessel(s) | ${fleet.messageCount} / ${fleet.messageLimit} messages stored`;
    }
    // Input limits come from metadata; the server still validates every count.
    function applyMetadata(metadata) {
        const { maxVessels, messageHistoryLimit } = metadata.settings;
        input.max = maxVessels;
        document.getElementById("count-range").textContent = `(0-${maxVessels})`;
        document.getElementById("history-note").textContent =
            `The latest ${messageHistoryLimit.toLocaleString()} reports are held in memory. The last 10 are shown here.`;
    }
    document.getElementById("vessel-form").addEventListener("submit", async event => {
        event.preventDefault();
        if (saving) return;
        saving = true;
        apply.disabled = true;
        saveStatus.textContent = "Saving...";
        try {
            const fleet = await request("/api/vessels", {
                method: "PUT", headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ count: Number(input.value) }),
            });
            showFleet(fleet);
            saveStatus.textContent = `Vessel count set to ${fleet.vessels.length}.`;
        } catch (error) {
            saveStatus.textContent = `Could not save: ${error.message}`;
        } finally {
            saving = false;
            apply.disabled = false;
        }
    });

    async function refresh() {
        try {
            const [fleet, history, metadata] = await Promise.all([
                request("/api/vessels"), request("/api/messages"), initialized ? null : request("/api/metadata"),
            ]);
            if (!saving) showFleet(fleet);
            if (metadata) {
                applyMetadata(metadata);
                input.value = fleet.vessels.length;
                input.disabled = false;
                apply.disabled = false;
                initialized = true;
            }
            // Sentences keep their CRLF in the API; it is trimmed only for display.
            messages.textContent = history.messages.slice(-10).reverse()
                .map(message => `${new Date(message.timestamp).toLocaleTimeString()}  #${message.sequence}  MMSI ${message.mmsi}  ${message.sentence.trim()}`)
                .join("\n") || "No messages stored.";
        } catch (error) {
            status.textContent = `Updates unavailable (${error.message}). Retrying...`;
        } finally {
            window.setTimeout(refresh, 1000);
        }
    }
    refresh();
})();
