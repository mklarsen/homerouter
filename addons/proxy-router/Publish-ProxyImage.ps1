param(
  [Parameter(Mandatory = $true)]
  [ValidatePattern('^\d+\.\d+\.\d+$')]
  [string]$Version
)

$ErrorActionPreference = "Stop"
$sourcePath = $PSScriptRoot
$builderName = "remote-box"
$imageName = "ghcr.io/mklarsen/homerouter-proxy"

$builderInfo = & docker buildx inspect $builderName
if ($LASTEXITCODE -ne 0) {
  throw "Could not inspect Buildx builder '$builderName'."
}
if (($builderInfo -join "`n") -notmatch "ssh://root@10\.10\.10\.1") {
  throw "Buildx builder '$builderName' does not target Homerouter (10.10.10.1)."
}

$ghToken = & gh auth token --hostname github.com
if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($ghToken)) {
  throw "Could not read the existing GitHub CLI token. No authentication scopes were changed."
}
$ghToken | & docker login ghcr.io --username mklarsen --password-stdin
$loginExitCode = $LASTEXITCODE
$ghToken = $null
if ($loginExitCode -ne 0) {
  throw "GHCR login failed. Verify the existing token has package-write access; this script does not change authentication scopes."
}

$buildArgs = @(
  "buildx", "build",
  "--builder", $builderName,
  "--platform", "linux/amd64",
  "--file", (Join-Path $sourcePath "Dockerfile"),
  "--push",
  "--tag", "${imageName}:$Version",
  "--tag", "${imageName}:latest",
  "--label", "org.opencontainers.image.title=Homerouter Proxy",
  "--label", "org.opencontainers.image.description=Homerouter-owned standard-library HTTP and CONNECT proxy.",
  "--label", "org.opencontainers.image.source=https://github.com/mklarsen/homerouter",
  "--label", "org.opencontainers.image.url=https://github.com/mklarsen/homerouter/tree/main/addons/proxy-router",
  "--label", "org.opencontainers.image.licenses=MIT",
  "--label", "org.opencontainers.image.vendor=Homerouter / Martin Kraus Larsen"
)

& docker @buildArgs $sourcePath
if ($LASTEXITCODE -ne 0) {
  throw "Remote Buildx build or GHCR push failed."
}

Write-Output "Published ${imageName}:$Version and ${imageName}:latest"
