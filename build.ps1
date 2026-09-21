# stage build script (Windows PowerShell 5.1)
#
#   .\build.ps1            builds bin\stage.exe
#   .\build.ps1 -Test      runs go vet + go test first
#
# CGO is off on purpose: building without a C compiler is a premise of this tool.
# Run this right after cloning AND after every submodule update -- bin\ is gitignored,
# so a stale exe can sit there silently and the hooks will check nothing.

[CmdletBinding()]
param(
    [switch]$Test
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path

# go looks at the module of the CURRENT folder. Move into ours so that calling this
# script from inside another Go module does not die with "outside main module".
Push-Location $root
try {
    $go = Get-Command go -ErrorAction SilentlyContinue
    if ($null -eq $go) {
        Write-Host "go not found. Install Go 1.26+ from https://go.dev/dl/ and open a new terminal." -ForegroundColor Red
        exit 1
    }
    Write-Host (& go version)

    $env:CGO_ENABLED = '0'

    if ($Test) {
        & go vet ./...
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
        & go test ./...
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    }

    $exe = Join-Path $root 'bin\stage.exe'
    $stamp = (Get-Date).ToString('yyyy-MM-ddTHH:mm:ssK')

    # Stamp which source built this exe. No git / failure -> empty -> the exe prints (dev).
    # No 2>$null here: with $ErrorActionPreference='Stop' PowerShell 5.1 turns a native
    # command's stderr into a terminating error, which killed the build outside a .git tree.
    $commit = ''
    $dirty = ''
    if (Get-Command git -ErrorAction SilentlyContinue) {
        try { $commit = (& git -C $root log -1 --format=%h -- cmd internal go.mod templates) } catch { $commit = '' }
        if ($LASTEXITCODE -ne 0) { $commit = '' }
        $global:LASTEXITCODE = 0
        if ($commit) {
            try { $status = (& git -C $root status --porcelain -- cmd internal go.mod templates) } catch { $status = '' }
            if ($LASTEXITCODE -ne 0) { $status = '' }
            $global:LASTEXITCODE = 0
            # Uncommitted source next to a commit stamp: mark it so nobody trusts the hash.
            if (($status | Out-String).Trim() -ne '') { $dirty = '-dirty' }
        }
    }
    if ($null -eq $commit) { $commit = '' }
    $commit = ($commit | Out-String).Trim()
    if ($commit -ne '') { $commit = "$commit$dirty" }

    $ldflags = "-s -w -X main.buildTime=$stamp -X main.buildCommit=$commit"
    Write-Host "build : $exe"
    # PowerShell 5.1 does not expand the variable if '-ldflags' and the value are one token.
    & go build -trimpath '-ldflags' $ldflags -o $exe (Join-Path $root 'cmd\stage')
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    $size = [math]::Round((Get-Item $exe).Length / 1MB, 1)
    Write-Host "done. $exe ($size MB)" -ForegroundColor Green
    exit 0
}
finally { Pop-Location }
