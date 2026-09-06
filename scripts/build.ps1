param([string]$Version = "0.1.0")
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$Dist = Join-Path $Root "dist"
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw "Go 1.23 or newer is required" }
Remove-Item $Dist -Recurse -Force -ErrorAction SilentlyContinue
New-Item $Dist -ItemType Directory | Out-Null
Push-Location $Root
try {
  go test ./...
  $Targets = @(@("windows","amd64","windows"), @("windows","arm64","windows"), @("darwin","amd64","macos"), @("darwin","arm64","macos"), @("linux","amd64","linux"), @("linux","arm64","linux"))
  foreach ($Target in $Targets) {
    $GoOS, $GoArch, $Platform = $Target
    $Launcher = if ($GoOS -eq "windows") { "bookmarkhub.exe" } else { "bookmarkhub" }
    $Core = if ($GoOS -eq "windows") { "bookmarkhub-core.exe" } else { "bookmarkhub-core" }
    $Package = Join-Path $Dist "bookmarkhub-$Platform-$GoArch"
    New-Item (Join-Path $Package "versions/$Version") -ItemType Directory -Force | Out-Null
    $env:CGO_ENABLED = "0"; $env:GOOS = $GoOS; $env:GOARCH = $GoArch
    go build -trimpath -ldflags="-s -w -X main.version=$Version" -o (Join-Path $Package "versions/$Version/$Core") ./cmd/bookmarkhub-core
    go build -trimpath -ldflags="-s -w" -o (Join-Path $Package $Launcher) ./cmd/bookmarkhub-launcher
    $CurrentJson = @{ version = $Version } | ConvertTo-Json
    [System.IO.File]::WriteAllText((Join-Path $Package "current.json"), $CurrentJson + [Environment]::NewLine, $Utf8NoBom)
    Compress-Archive -Path $Package -DestinationPath "$Package.zip"
  }
  $ExtensionPackage = Join-Path $Dist "bookmarkhub-extension-$Version"
  Copy-Item (Join-Path $Root "extension") $ExtensionPackage -Recurse
  Compress-Archive -Path $ExtensionPackage -DestinationPath "$ExtensionPackage.zip"
} finally { Pop-Location; Remove-Item Env:GOOS,Env:GOARCH,Env:CGO_ENABLED -ErrorAction SilentlyContinue }
