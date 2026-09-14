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
|   |   |-- main.go
|   |   `-- main_test.go
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
|   |-- app/
|   |   |-- app.go
|   |   |-- app_test.go
|   |   `-- doc.go
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
|   |-- ui/
|   |   |-- assets/
|   |   |   |-- html/
|   |   |   |   |-- pages/
|   |   |   |   |   |-- display.tmpl
|   |   |   |   |   |-- home.tmpl
|   |   |   |   |   |-- manager.tmpl
|   |   |   |   |   `-- status.tmpl
|   |   |   |   `-- base.tmpl
|   |   |   |-- static/
|   |   |   |   |-- css/
|   |   |   |   |   `-- app.css
|   |   |   |   `-- js/
|   |   |   |       `-- htmx.min.js
|   |   |   |-- doc.go
|   |   |   `-- efs.go
|   |   |-- doc.go
|   |   |-- handlers.go
|   |   |-- handlers_test.go
|   |   `-- render.go
|   |-- visualization/
|   |   |-- service/
|   |   |   |-- doc.go
|   |   |   `-- ports.go
|   |   `-- README.md
|   `-- README.md
|-- taskfile/
|   |-- clean.ps1
|   |-- deadcode.ps1
|   |-- README.md
|   `-- test.ps1
|-- .env.template.ps1
|-- .gitattributes
|-- .gitignore
|-- .todo
|-- AGENTS.md
|-- CLAUDE.md
|-- go.mod
|-- go.sum
|-- LICENSE
|-- README.md
`-- Taskfile.yml
```
