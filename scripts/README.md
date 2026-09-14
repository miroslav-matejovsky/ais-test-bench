# Build and development scripts

This directory is reserved for frontend build and distribution helpers.
Each frontend build will populate its own embedded asset directory before
`go build ./cmd/ais-test-bench`.

The existing PowerShell check helpers remain in `taskfile/`; run `task all`
from the repository root for formatting, static checks, and tests.
