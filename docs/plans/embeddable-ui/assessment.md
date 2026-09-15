# Impact, feasibility, and risks

## Assessment

Feasible with the existing standard-library HTTP stack and embedded frontend.
Overall effort is L; complexity is high. Publishing package paths is straightforward.
Browser isolation, URL ownership, and lifecycle boundaries account for most work.
No new frontend framework or simulation algorithm is needed.

The engine's deterministic behavior, driver command ordering, station conflicts,
wire data, and display validation remain invariants. Public package and constructor
changes are intentional. Existing consumers must supply or accept the documented
logger default and migrate to the new configuration and mount contracts.

## Risks and controls

| Risk | Control and evidence |
| --- | --- |
| Prefix lost in templates, redirects, remote reads, or proxy paths | Explicit public URL config; root/nested/stripping-proxy tests; browser request assertions |
| Component changes break station drafts, history, or delayed-response handling | Preserve generation guards and existing browser fixtures; add destroy/remount cases |
| CSS, IDs, Leaflet, or htmx interfere with the host | Root-scoped CSS/DOM, unique IDs, isolated module assets, hostile host-page fixture |
| Local source bypasses received-only projection or semantic validation | Shared source-to-display pipeline; local/remote parity and malformed-source tests |
| Public interfaces become too broad | One two-method source seam; concrete runtime ownership; keep helpers internal |
| Lifecycle changes leak resources or discard errors | Side-effect-free constructors, explicit Run/Serve contracts, deterministic failure/shutdown tests |
| Consumer logger deadlocks or loses context/fields | Log outside state locks; captured slog records and instance isolation tests |
| Host authentication/CSRF cannot reach embedded requests | Ordinary middleware, same-origin defaults, configurable fetch and remote HTTP client |
| Map assets require permissive host CSP or overwrite host globals | Packaged module assets, inert configuration, explicit tile policy and fallback |
| Commands still hide reusable behavior | Compile an external consumer and run commands solely through public entry points |

## Tradeoffs

The local source replaces combined mode's HTTP transport equivalence with a shared
semantic contract. This removes listener management from embedding while retaining
independent remote HTTP validation tests. Document that difference explicitly.

Scoping CSS provides practical host-page integration, not complete isolation from
arbitrary aggressive host CSS. Namespaced selectors and documented style hooks are
the initial contract. Shadow DOM and framework-specific wrappers can be evaluated
later if demonstrated integration problems justify them.

Bundling Leaflet adds maintained third-party assets and license obligations but
removes the mandatory CDN script dependency. Map tiles remain a configured external
resource unless the host supplies its own tile service.

## Completion evidence

Implementation is accepted only after the final step's consumer, browser, and
`task all` checks pass. Planning acceptance consists of this assessment, the API
and ownership decisions, and the six implementation steps. No implementation or
test execution is claimed by this plan.
