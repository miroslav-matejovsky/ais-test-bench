// Public browser entry point for AIS test bench components. Import it once per
// asset base, then mount each root rendered by ui.UI.RenderManager or
// ui.UI.RenderDisplay:
//
//     import { mountManager, mountDisplay } from "/tools/ais/assets/js/ui.js";
//     const manager = mountManager(document.getElementById("fleet"), { fetch: hostFetch });
//     manager.destroy();
//
// Each mount function returns { destroy }. options.fetch is optional.
export { mountManager } from "./manager.js";
export { mountDisplay } from "./display.js";
