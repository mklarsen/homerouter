param(
  [string]$RemoteHost = "10.10.10.1"
)

$ErrorActionPreference = "Stop"

$archivePath = Join-Path $PSScriptRoot "dist\homerouter-gost-3.3.0-local.1-linux-amd64.tar"
$overlayPath = Join-Path $PSScriptRoot "docker-compose.local-image.yml"
$remoteArchive = "/opt/stacks/homerouter-gost-3.3.0-local.1-linux-amd64.tar"
$remoteOverlay = "/opt/stacks/homerouter-gost-image.yml"
$imageTag = "homerouter/gost:3.3.0-local.1"

if (-not (Test-Path $archivePath)) {
  throw "Image archive is missing. Run Build-GostImage.ps1 first."
}

& scp $archivePath "${RemoteHost}:$remoteArchive"
if ($LASTEXITCODE -ne 0) {
  throw "Could not copy the image archive to Homerouter."
}

& scp $overlayPath "${RemoteHost}:$remoteOverlay"
if ($LASTEXITCODE -ne 0) {
  throw "Could not copy the local-image Compose override to Homerouter."
}

& ssh $RemoteHost "docker load -i $remoteArchive"
if ($LASTEXITCODE -ne 0) {
  throw "Could not load the built image on Homerouter."
}

& ssh $RemoteHost "docker image inspect $imageTag --format '{{.Id}}'"
if ($LASTEXITCODE -ne 0) {
  throw "The image was not found on Homerouter after loading."
}

Write-Output "Image staged on Homerouter. The running vpn-proxy service was not restarted."
