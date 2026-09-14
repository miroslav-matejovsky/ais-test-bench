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
    let centered = false;
    function fitVessels() {
        if (markers.size) {
            map.fitBounds(L.latLngBounds([...markers.values()].map(marker => marker.getLatLng())), {
                padding: [35, 35], maxZoom: 11,
            });
        }
    }
    document.getElementById("fit-vessels").addEventListener("click", fitVessels);

    async function refresh() {
        try {
            const response = await fetch("/api/vessels", { cache: "no-store", signal: AbortSignal.timeout(5000) });
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            const data = await response.json();
            const active = new Set();
            for (const vessel of data.vessels) {
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
                const details = document.createElement("div");
                details.className = "vessel-details";
                details.textContent = `${vessel.name}\nMMSI: ${vessel.mmsi}\nSpeed: ${vessel.speed.toFixed(1)} kn\nCourse: ${vessel.course.toFixed(1)} deg\nHeading: ${vessel.heading} deg\nPosition: ${vessel.latitude.toFixed(6)}, ${vessel.longitude.toFixed(6)}\nUpdated: ${new Date(vessel.updatedAt).toLocaleTimeString()}`;
                marker.setPopupContent(details);
            }
            for (const [mmsi, marker] of markers) {
                if (!active.has(mmsi)) {
                    map.removeLayer(marker);
                    markers.delete(mmsi);
                }
            }
            if (!centered && markers.size) {
                fitVessels();
                centered = true;
            }
            status.textContent = `Live: ${data.vessels.length} vessel(s) | Updated ${new Date(data.updatedAt).toLocaleTimeString()}`;
        } catch (error) {
            status.textContent = `Updates unavailable (${error.message}). Retrying...`;
        } finally {
            window.setTimeout(refresh, 1000);
        }
    }
    refresh();
})();
