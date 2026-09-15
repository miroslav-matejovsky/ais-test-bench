// Standalone pages mount their rendered components with the public module.
import { mountDisplay, mountManager } from "./ui.js";

for (const root of document.querySelectorAll("[data-ais-manager]")) mountManager(root);
for (const root of document.querySelectorAll("[data-ais-display]")) mountDisplay(root);
