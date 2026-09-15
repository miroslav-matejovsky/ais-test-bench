// Lifecycle shared by mounted components. A runtime owns one mount: its abort
// signal, timers, cleanup callbacks, and the pristine root markup restored by
// destroy. Every request, timer, and listener of a mount goes through it, so
// destroy stops all later work.

const mountedAttribute = "data-ais-mounted";

// claim validates a server-rendered component root and starts its runtime.
// marker is the component's data attribute. options.fetch replaces the browser
// fetch for every request of this mount; it receives the URL and a RequestInit
// with a signal, and must return a Response promise.
export function claim(root, marker, name, options = {}) {
    if (!(root instanceof Element) || !root.hasAttribute(marker)) {
        throw new TypeError(`${name}: root must be an element rendered with ${marker}`);
    }
    if (root.hasAttribute(mountedAttribute)) {
        throw new Error(`${name}: #${root.id} is already mounted; call destroy() before mounting again`);
    }
    const apiBase = root.dataset.apiBase;
    if (!apiBase || !apiBase.endsWith("/")) throw new Error(`${name}: #${root.id} has no data-api-base ending in "/"`);
    const fetchFunction = options.fetch ?? ((url, init) => globalThis.fetch(url, init));
    if (typeof fetchFunction !== "function") throw new TypeError(`${name}: options.fetch must be a function`);

    const pristine = [...root.childNodes].map(node => node.cloneNode(true));
    const refs = new Map([...root.querySelectorAll("[data-ref]")].map(node => [node.dataset.ref, node]));
    const lifetime = new AbortController();
    const timers = new Set();
    const cleanups = [];
    root.setAttribute(mountedAttribute, "");

    const runtime = {
        root,
        signal: lifetime.signal,
        get destroyed() { return lifetime.signal.aborted; },
        // ref returns the element with data-ref="name" inside this root.
        ref(refName) {
            const node = refs.get(refName);
            if (!node) throw new Error(`${name}: #${root.id} has no [data-ref="${refName}"]`);
            return node;
        },
        // id returns a document-unique element ID derived from the root ID.
        id(suffix) { return `${root.id}-${suffix}`; },
        // fetch requests path below the API base. The request aborts on destroy,
        // after timeout milliseconds, or when init.signal aborts.
        fetch(path, init = {}, timeout = 5000) {
            const signals = [lifetime.signal, AbortSignal.timeout(timeout)];
            if (init.signal) signals.push(init.signal);
            return fetchFunction(apiBase + path, { cache: "no-store", ...init, signal: AbortSignal.any(signals) });
        },
        // later runs callback after delay unless the mount is destroyed first.
        later(callback, delay) {
            if (lifetime.signal.aborted) return null;
            const timer = setTimeout(() => { timers.delete(timer); callback(); }, delay);
            timers.add(timer);
            return timer;
        },
        cancel(timer) { clearTimeout(timer); timers.delete(timer); },
        // listen adds a listener that destroy removes.
        listen(target, type, listener) { target.addEventListener(type, listener, { signal: lifetime.signal }); },
        onDestroy(callback) { cleanups.push(callback); },
        // destroy is idempotent. It aborts requests, clears timers, runs cleanups
        // in reverse order, and restores the markup rendered by the server, which
        // drops every listener and dynamic node. A write aborted here may already
        // have been committed by the server; a new mount reads the current state.
        destroy() {
            if (lifetime.signal.aborted) return;
            lifetime.abort(new DOMException(`${name} destroyed`, "AbortError"));
            for (const timer of timers) clearTimeout(timer);
            timers.clear();
            const errors = [];
            for (const cleanup of cleanups.splice(0).reverse()) {
                try { cleanup(); } catch (error) { errors.push(error); }
            }
            root.replaceChildren(...pristine);
            root.removeAttribute(mountedAttribute);
            if (errors.length) throw new AggregateError(errors, `${name}: cleanup failed`);
        },
    };
    return runtime;
}

// mount runs start with a new runtime and returns the public handle. A failed
// start releases everything it acquired.
export function mount(root, marker, name, options, start) {
    const runtime = claim(root, marker, name, options);
    try {
        start(runtime);
    } catch (error) {
        runtime.destroy();
        throw error;
    }
    return Object.freeze({ destroy: () => runtime.destroy() });
}
