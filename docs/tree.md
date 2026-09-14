# Complete project tree

Source and configuration files are shown below, including existing repository
metadata. Git internals, ignored test results, and local build artifacts are omitted.

```text
ais-test-bench/
|-- .claude/
|   `-- settings.json
|-- .vscode/
|   `-- settings.json
|-- cmd/
|   |-- ais-test-bench/
|   |   |-- doc.go
|   |   `-- main.go
|   `-- README.md
|-- configs/
|   |-- config.yaml
|   `-- README.md
|-- deployments/
|   `-- README.md
|-- docs/
|   |-- dependencies.md
|   |-- packages.md
|   |-- README.md
|   |-- startup.md
|   `-- tree.md
|-- internal/
|   |-- ais/
|   |   |-- application/
|   |   |   |-- doc.go
|   |   |   `-- ports.go
|   |   |-- domain/
|   |   |   |-- doc.go
|   |   |   `-- types.go
|   |   |-- infrastructure/
|   |   |   `-- doc.go
|   |   `-- README.md
|   |-- api/
|   |   |-- dto/
|   |   |   |-- doc.go
|   |   |   `-- error.go
|   |   |-- handlers/
|   |   |   `-- doc.go
|   |   |-- middleware/
|   |   |   |-- doc.go
|   |   |   `-- middleware.go
|   |   |-- doc.go
|   |   `-- router.go
|   |-- app/
|   |   |-- config/
|   |   |   |-- config.go
|   |   |   `-- doc.go
|   |   |-- bootstrap.go
|   |   |-- doc.go
|   |   |-- lifecycle.go
|   |   `-- server.go
|   |-- management/
|   |   |-- service/
|   |   |   |-- doc.go
|   |   |   `-- ports.go
|   |   `-- README.md
|   |-- networking/
|   |   |-- tcp/
|   |   |   `-- doc.go
|   |   |-- udp/
|   |   |   `-- doc.go
|   |   `-- README.md
|   |-- simulation/
|   |   |-- application/
|   |   |   |-- doc.go
|   |   |   `-- ports.go
|   |   |-- domain/
|   |   |   |-- doc.go
|   |   |   `-- types.go
|   |   |-- infrastructure/
|   |   |   `-- doc.go
|   |   `-- README.md
|   |-- targets/
|   |   |-- application/
|   |   |   |-- doc.go
|   |   |   `-- ports.go
|   |   |-- domain/
|   |   |   |-- doc.go
|   |   |   `-- types.go
|   |   |-- infrastructure/
|   |   |   `-- doc.go
|   |   `-- README.md
|   |-- visualization/
|   |   |-- service/
|   |   |   |-- doc.go
|   |   |   `-- ports.go
|   |   `-- README.md
|   `-- README.md
|-- scripts/
|   `-- README.md
|-- taskfile/
|   |-- clean.ps1
|   |-- deadcode.ps1
|   |-- README.md
|   `-- test.ps1
|-- test/
|   |-- integration/
|   |   `-- README.md
|   |-- simulation/
|   |   `-- README.md
|   |-- testdata/
|   |   `-- README.md
|   `-- README.md
|-- web/
|   |-- admin/
|   |   |-- assets/
|   |   |   |-- index.html
|   |   |   `-- README.md
|   |   |-- assets.go
|   |   `-- doc.go
|   |-- viewer/
|   |   |-- assets/
|   |   |   |-- index.html
|   |   |   `-- README.md
|   |   |-- assets.go
|   |   `-- doc.go
|   `-- README.md
|-- .env.template.ps1
|-- .gitattributes
|-- .gitignore
|-- .todo
|-- AGENTS.md
|-- CLAUDE.md
|-- go.mod
|-- LICENSE
|-- README.md
`-- Taskfile.yml
```
