
$VERSION=$args[0]

if ($VERSION -notmatch '^v\d+\.\d+\.\d+$') {
  $PREVVERSION=git describe --tags --abbrev=0
  Write-Host "Please provide a valid version (previous version was $PREVVERSION)"
  exit 1
}

New-Item -ItemType Directory -Force -Path ./dist
Remove-Item ./dist/*
Remove-Item ./static/dist/*

Write-Host "Building $VERSION"

Write-Host "Bundling CSS"
npm run build

Write-Host "Building arm64"
docker run --mount type=bind,source="$(Get-Location)",target=/gomas --interactive --workdir /gomas xgomas-arm64 bash /gomas/scripts/make.sh $VERSION arm64
if ($LastExitCode -ne 0) { exit 1 }

Write-Host "Building arm32"
docker run --mount type=bind,source="$(Get-Location)",target=/gomas --interactive --workdir /gomas xgomas-arm32 bash /gomas/scripts/make.sh $VERSION arm32
if ($LastExitCode -ne 0) { exit 1 }
