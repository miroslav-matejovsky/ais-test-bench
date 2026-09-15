# Compile the public runnable examples as an external consumer module.
#
# The examples are copied into .tmp/consumer, a separate module that requires
# this repository through a local replace directive. Go rejects imports of
# another module's internal packages, so a successful build proves the examples
# use only published packages. Dependencies come from the local module cache;
# nothing is downloaded.
$ErrorActionPreference = 'Stop'

$examples = @(
    'simulation/example_test.go'
    'simulator/example_test.go'
    'ui/example_test.go'
    'testbench/example_test.go'
)

$root = (Get-Location).Path
$dir = Join-Path $root '.tmp/consumer'
if (Test-Path $dir) {
    Remove-Item -Recurse -Force $dir
}
New-Item -ItemType Directory -Path $dir | Out-Null

foreach ($example in $examples) {
    $target = Join-Path $dir (Split-Path $example -Parent)
    New-Item -ItemType Directory -Force -Path $target | Out-Null
    Copy-Item -Path (Join-Path $root $example) -Destination $target
}

$goVersion = (Select-String -Path (Join-Path $root 'go.mod') -Pattern '^go (\S+)$').Matches[0].Groups[1].Value
@"
module example.test/consumer

go $goVersion

require github.com/miroslav-matejovsky/ais-testbench v0.0.0

replace github.com/miroslav-matejovsky/ais-testbench => ../..
"@ | Set-Content -Path (Join-Path $dir 'go.mod') -Encoding utf8NoBOM
Copy-Item -Path (Join-Path $root 'go.sum') -Destination $dir

$env:GOWORK = 'off'
$env:GOPROXY = 'off'
$env:GOFLAGS = '-mod=mod'
Push-Location $dir
try {
    go mod tidy
    if ($LASTEXITCODE -ne 0) {
        exit 1
    }
    go vet ./...
    if ($LASTEXITCODE -ne 0) {
        exit 1
    }
}
finally {
    Pop-Location
}
Write-Host "consumer: examples compile as an external module"
