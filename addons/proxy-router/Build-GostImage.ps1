$ErrorActionPreference = "Stop"

$sourcePath = Join-Path $PSScriptRoot "gost-source"
$sourceTag = "v3.3.0"
$sourceUrl = "https://github.com/go-gost/gost.git"
$builderName = "remote-box"
$imageTag = "homerouter/gost:3.3.0-local.1"
$expectedCommit = "cb76f63754768c7b5d68895a0d51635b0141b80f"

if (-not (Test-Path $sourcePath)) {
  & git clone --depth 1 --branch $sourceTag $sourceUrl $sourcePath
  if ($LASTEXITCODE -ne 0) {
    throw "Could not clone GOST source tag '$sourceTag'."
  }
}

if (-not (Test-Path (Join-Path $sourcePath "Dockerfile"))) {
  throw "GOST source is incomplete. Remove '$sourcePath' and rerun this script."
}

$sourceCommit = (& git -C $sourcePath rev-parse HEAD).Trim()
if ($LASTEXITCODE -ne 0) {
  throw "Could not read the GOST source revision."
}
if ($sourceCommit -ne $expectedCommit) {
  throw "Unexpected GOST source revision '$sourceCommit'; expected '$expectedCommit'."
}

$sourceChanges = & git -C $sourcePath status --porcelain
if ($LASTEXITCODE -ne 0) {
  throw "Could not check the GOST source worktree."
}
if ($sourceChanges) {
  throw "The pinned GOST source has local modifications; refusing to build."
}

$builderInfo = & docker buildx inspect $builderName
if ($LASTEXITCODE -ne 0) {
  throw "Could not inspect Buildx builder '$builderName'."
}
if (($builderInfo -join "`n") -notmatch "ssh://root@10\.10\.10\.1") {
  throw "Buildx builder '$builderName' does not target Homerouter (10.10.10.1)."
}

$distPath = Join-Path $PSScriptRoot "dist"
New-Item -ItemType Directory -Path $distPath -Force | Out-Null
$archivePath = Join-Path $distPath "homerouter-gost-3.3.0-local.1-linux-amd64.tar"
& docker buildx build --builder $builderName --platform linux/amd64 --tag $imageTag --output "type=docker,dest=$archivePath" $sourcePath
if ($LASTEXITCODE -ne 0) {
  throw "Docker build failed."
}

Write-Output "Built $imageTag on Homerouter's remote-box builder and exported it to $archivePath"
