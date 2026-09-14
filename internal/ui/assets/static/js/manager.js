(() => {
    const input = document.getElementById("vessel-count");
    const apply = document.getElementById("apply-count");
    const status = document.getElementById("fleet-status");
    const saveStatus = document.getElementById("save-status");
    let initialized = false;
    let saving = false;

    async function request(path, options = {}) {
        const response = await fetch(path, { cache: "no-store", signal: AbortSignal.timeout(5000), ...options });
        if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
        return response.json();
    }
    function showFleet(data) {
        status.textContent = `${data.vessels.length} active vessel(s) | ${data.messageCount} / ${data.messageLimit} messages stored`;
    }
    document.getElementById("vessel-form").addEventListener("submit", async event => {
        event.preventDefault();
        if (saving) return;
        saving = true;
        apply.disabled = true;
        saveStatus.textContent = "Saving...";
        try {
            const data = await request("/api/vessels", {
                method: "PUT", headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ count: Number(input.value) }),
            });
            showFleet(data);
            saveStatus.textContent = `Vessel count set to ${data.vessels.length}.`;
        } catch (error) {
            saveStatus.textContent = `Could not save: ${error.message}`;
        } finally {
            saving = false;
            apply.disabled = false;
        }
    });

    async function refresh() {
        try {
            const [data, messages] = await Promise.all([request("/api/vessels"), request("/api/messages")]);
            if (!saving) showFleet(data);
            if (!initialized) {
                input.value = data.vessels.length;
                input.disabled = false;
                apply.disabled = false;
                initialized = true;
            }
            document.getElementById("messages").textContent = messages.slice(-10).reverse()
                .map(message => `${new Date(message.timestamp).toLocaleTimeString()}  MMSI ${message.mmsi}  ${message.sentence.trim()}`)
                .join("\n") || "No messages stored.";
        } catch (error) {
            status.textContent = `Updates unavailable (${error.message}). Retrying...`;
        } finally {
            window.setTimeout(refresh, 1000);
        }
    }
    refresh();
})();
