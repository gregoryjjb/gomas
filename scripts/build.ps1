
$VERSION=$args[0]

if ($VERSION -notmatch '^v\d+\.\d+\.\d+$') {
  $PREVVERSION=git describe --tags --abbrev=0
  Write-Host "Please provide a valid version (previous version was $PREVVERSION)"
  exit 1
}

Write-Host "Building $VERSION"

docker run --mount type=bind,source="$(Get-Location)",target=/gomas --interactive --workdir /gomas xgomas bash /gomas/scripts/make.sh $VERSION
