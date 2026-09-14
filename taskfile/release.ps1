# Tag and push a release. Go modules are versioned by git tag alone, so this
# script does not build or upload any artifacts.
param(
  [Parameter(Mandatory = $true)]
  [string]$Version
)

$ErrorActionPreference = "Stop"

$root = git rev-parse --show-toplevel
if (-not $root) { throw "not in a git repository" }
Set-Location $root

$core = $Version.Trim() -replace '^[vV]', ''
$semverPattern = '^\d+\.\d+\.\d+(-[0-9A-Za-z-.]+)?(\+[0-9A-Za-z-.]+)?$'
if ($core -notmatch $semverPattern) {
  throw "invalid version '$Version': expected semver like 1.2.3 or v1.2.3"
}
$tag = "v$core"

$branch = git rev-parse --abbrev-ref HEAD
if ($branch -ne "main") {
  throw "must be on main branch to release (current: $branch)"
}

git fetch origin main --quiet
if ($LASTEXITCODE -ne 0) { throw "git fetch origin main failed" }

$status = git status --porcelain
if ($status) {
  throw "working tree has uncommitted changes, commit or stash before releasing"
}

$localHead = git rev-parse HEAD
$remoteHead = git rev-parse origin/main
if ($localHead -ne $remoteHead) {
  throw "local main is not in sync with origin/main (unpushed or diverged commits)"
}

if (git tag -l $tag) {
  throw "tag $tag already exists"
}

Write-Host "creating tag $tag"
git tag -a $tag -m "Release $tag"
if ($LASTEXITCODE -ne 0) { throw "git tag failed" }

git push origin $tag
if ($LASTEXITCODE -ne 0) { throw "git push origin $tag failed" }

Write-Host "release done: $tag"
