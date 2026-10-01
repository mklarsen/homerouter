$ErrorActionPreference = "Stop"

$sourcePath = Join-Path $PSScriptRoot "upstream"
$builderName = "remote-box"
$expectedCommit = "cb76f63754768c7b5d68895a0d51635b0141b80f"
$imageName = "ghcr.io/mklarsen/homerouter-proxy"
$versionTag = "3.3.0"
$commitTag = "3.3.0-cb76f63"

$revisionFile = Join-Path $sourcePath "UPSTREAM_REVISION"
if (-not (Test-Path (Join-Path $sourcePath "Dockerfile")) -or -not (Test-Path $revisionFile)) {
  throw "Vendored GOST source or UPSTREAM_REVISION is missing: $sourcePath"
}

$revisionLines = Get-Content $revisionFile
if ($revisionLines[-1].Trim() -ne $expectedCommit) {
  throw "Unexpected GOST source revision; expected '$expectedCommit'."
}
if ((Test-Path (Join-Path $sourcePath ".git")) -or (Test-Path (Join-Path $sourcePath ".github"))) {
  throw "Vendored source must not contain upstream Git metadata or CI workflows."
}

$builderInfo = & docker buildx inspect $builderName
if ($LASTEXITCODE -ne 0) {
  throw "Could not inspect Buildx builder '$builderName'."
}
if (($builderInfo -join "`n") -notmatch "ssh://root@10\.10\.10\.1") {
  throw "Buildx builder '$builderName' does not target Homerouter (10.10.10.1)."
}

$authStatus = (& gh auth status --hostname github.com 2>&1 | Out-String)
if ($LASTEXITCODE -ne 0 -or $authStatus -notmatch "write:packages") {
  throw "GitHub CLI needs the write:packages scope. Run: gh auth refresh -h github.com -s write:packages"
}

$ghToken = & gh auth token --hostname github.com
if ($LASTEXITCODE -ne 0 -or -not $ghToken) {
  throw "Could not read the authenticated GitHub CLI token."
}
$ghToken | & docker login ghcr.io --username mklarsen --password-stdin
$loginExitCode = $LASTEXITCODE
$ghToken = $null
if ($loginExitCode -ne 0) {
  throw "Docker login to ghcr.io failed."
}

$buildArgs = @(
  "buildx", "build",
  "--builder", $builderName,
  "--platform", "linux/amd64",
  "--file", (Join-Path $sourcePath "Dockerfile"),
  "--push",
  "--tag", "${imageName}:$versionTag",
  "--tag", "${imageName}:$commitTag",
  "--tag", "${imageName}:latest",
  "--label", "org.opencontainers.image.title=Homerouter Proxy",
  "--label", "org.opencontainers.image.description=Homerouter-maintained derivative of GOST v3.3.0; upstream source at github.com/go-gost/gost.",
  "--label", "org.opencontainers.image.source=https://github.com/mklarsen/homerouter",
  "--label", "org.opencontainers.image.url=https://github.com/mklarsen/homerouter/tree/main/addons/proxy-router",
  "--label", "org.opencontainers.image.licenses=MIT",
  "--label", "org.opencontainers.image.vendor=Homerouter / Martin Kraus Larsen"
)

& docker @buildArgs $sourcePath
if ($LASTEXITCODE -ne 0) {
  throw "Remote Buildx build or GHCR push failed."
}

Write-Output "Published ${imageName}:$versionTag, ${imageName}:$commitTag, and ${imageName}:latest"
