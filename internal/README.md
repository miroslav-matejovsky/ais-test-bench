# Internal packages

The modular monolith contains six bounded contexts: simulation, AIS, targets,
networking, management, and visualization. Application composition lives in
`app`; the server-rendered Manager and Display UIs live in `ui`.

Domain packages define values and invariants. Application packages define use
cases and consumer-owned ports. Infrastructure packages host concrete adapters.
Only `app` assembles implementations across contexts.

See [the package catalog](../docs/packages.md) for interfaces and dependency rules.
