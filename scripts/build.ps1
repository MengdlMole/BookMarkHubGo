param([string]$Version = "")
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$Dist = Join-Path $Root "dist"
$Utf8NoBom = New-Object System.Text.UTF8Encoding($false)
if ([string]::IsNullOrWhiteSpace($Version)) { $Version = (Get-Content (Join-Path $Root "VERSION") -Raw).Trim() }
if ($Version -notmatch '^(0|[1-9]\d*)(\.(0|[1-9]\d*)){0,3}$') { throw "Version must contain 1 to 4 dot-separated integers without leading zeros" }
foreach ($Part in $Version.Split('.')) { if ([int64]$Part -gt 65535) { throw "Each version component must be between 0 and 65535" } }
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
    if ($GoOS -eq "darwin") {
      Copy-Item (Join-Path $Root "scripts/macos-first-run.command") (Join-Path $Package "macos-first-run.command")
      Copy-Item (Join-Path $Root "scripts/README-macOS.txt") (Join-Path $Package "README-macOS.txt")
    }
    Compress-Archive -Path $Package -DestinationPath "$Package.zip"
  }
  $ExtensionPackage = Join-Path $Dist "bookmarkhub-extension-$Version"
  Copy-Item (Join-Path $Root "extension") $ExtensionPackage -Recurse
  $ExtensionManifestPath = Join-Path $ExtensionPackage "manifest.json"
  $ExtensionManifest = Get-Content $ExtensionManifestPath -Raw | ConvertFrom-Json
  $ExtensionManifest.version = $Version
  $ExtensionManifestJson = $ExtensionManifest | ConvertTo-Json -Depth 10
  [System.IO.File]::WriteAllText($ExtensionManifestPath, $ExtensionManifestJson + [Environment]::NewLine, $Utf8NoBom)
  if ((Get-Content $ExtensionManifestPath -Raw | ConvertFrom-Json).version -ne $Version) { throw "Failed to set extension version" }
  Compress-Archive -Path $ExtensionPackage -DestinationPath "$ExtensionPackage.zip"
  Write-Host "Portable packages version $Version created in $Dist"
} finally { Pop-Location; Remove-Item Env:GOOS,Env:GOARCH,Env:CGO_ENABLED -ErrorAction SilentlyContinue }
