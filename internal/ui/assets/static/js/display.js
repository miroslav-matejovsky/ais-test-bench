/* global L */
(() => {
    const status = document.getElementById("live-status");
    if (!window.L) {
        status.textContent = "Map library could not load. Check your internet connection and reload.";
        return;
    }
    const map = L.map("map").setView([52.02, 3.97], 11);
    const tiles = L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
        maxZoom: 19,
        attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
    }).addTo(map);
    tiles.on("tileerror", () => {
        document.getElementById("tile-status").textContent = "Some map tiles could not load. Vessel updates remain active.";
    });

    const markers = new Map();
    let simulationId = null;
    let framed = false;
    let lastUpdate = null;

    function fitVessels() {
        if (markers.size) {
            map.fitBounds(L.latLngBounds([...markers.values()].map(marker => marker.getLatLng())), {
                padding: [35, 35], maxZoom: 11,
            });
        }
    }
    document.getElementById("fit-vessels").addEventListener("click", fitVessels);

    function clearMarkers() {
        for (const marker of markers.values()) map.removeLayer(marker);
        markers.clear();
    }

    function format(value, unit, digits) {
        return value === null ? "not available" : `${digits === undefined ? value : value.toFixed(digits)} ${unit}`;
    }

    // Names and type labels are set as text, never as HTML.
    function details(vessel) {
        const element = document.createElement("div");
        element.className = "vessel-details";
        element.textContent = [
            vessel.name,
            `Type: ${vessel.typeName}`,
            `MMSI: ${vessel.mmsi}`,
            `Speed: ${format(vessel.speed, "kn", 1)}`,
            `Course: ${format(vessel.course, "deg", 1)}`,
            `Heading: ${format(vessel.heading, "deg")}`,
            `Position: ${vessel.latitude.toFixed(6)}, ${vessel.longitude.toFixed(6)}`,
            `Updated: ${new Date(vessel.updatedAt).toLocaleTimeString()}`,
        ].join("\n");
        return element;
    }

    // apply replaces the map's active set with one complete fleet.
    function apply(fleet) {
        if (fleet.simulationId !== simulationId) {
            // A restarted simulator may reuse MMSIs; never mix runs.
            clearMarkers();
            simulationId = fleet.simulationId;
            framed = false;
        }
        const active = new Set();
        let withoutFix = 0;
        for (const vessel of fleet.vessels) {
            if (vessel.latitude === null || vessel.longitude === null) {
                withoutFix++;
                continue;
            }
            active.add(vessel.mmsi);
            let marker = markers.get(vessel.mmsi);
            if (!marker) {
                marker = L.circleMarker([vessel.latitude, vessel.longitude], {
                    radius: 7, color: "#083b66", fillColor: "#168aad", fillOpacity: 0.9,
                }).addTo(map);
                marker.bindPopup(document.createElement("div"));
                const label = document.createElement("span");
                label.textContent = vessel.name;
                marker.bindTooltip(label);
                markers.set(vessel.mmsi, marker);
            }
            marker.setLatLng([vessel.latitude, vessel.longitude]);
            marker.setPopupContent(details(vessel));
        }
        for (const [mmsi, marker] of markers) {
            if (!active.has(mmsi)) {
                map.removeLayer(marker);
                markers.delete(mmsi);
            }
        }
        // Frame once per run; later updates keep the user's pan and zoom.
        if (!framed) {
            if (markers.size) {
                fitVessels();
            } else {
                const b = fleet.spawnBounds;
                map.fitBounds([[b.south, b.west], [b.north, b.east]], { maxZoom: 11 });
            }
            framed = true;
        }
        const noFix = withoutFix ? `, ${withoutFix} without position fix` : "";
        status.textContent = `Live: ${fleet.vessels.length} vessel(s)${noFix} | Updated ${new Date(fleet.updatedAt).toLocaleTimeString()}`;
    }

    // One request at a time; the next poll starts one second after completion.
    // The timeout exceeds the backend's five-second simulator deadline.
    async function refresh() {
        try {
            const response = await fetch("/display/api/vessels", { cache: "no-store", signal: AbortSignal.timeout(7000) });
            if (!response.ok) throw new Error((await response.text()).trim() || `HTTP ${response.status}`);
            apply(await response.json());
            lastUpdate = new Date();
        } catch (error) {
            const last = lastUpdate ? `Showing data from ${lastUpdate.toLocaleTimeString()}.` : "No vessel data yet.";
            status.textContent = `Updates unavailable (${error.message}). ${last} Retrying...`;
        } finally {
            window.setTimeout(refresh, 1000);
        }
    }
    refresh();
})();
