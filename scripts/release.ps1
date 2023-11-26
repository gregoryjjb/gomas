$ErrorActionPreference = "Stop"

$VERSION=$args[0]
$PREVVERSION=git describe --tags --abbrev=0
if ($LastExitCode -ne 0) { exit 1 }

if ($VERSION -notmatch '^v\d+\.\d+\.\d+$') {
  Write-Host "Please provide a valid version (previous version was $PREVVERSION)"
  exit 1
}

# Build
scripts/build.ps1 $VERSION
if ($LastExitCode -ne 0) { exit 1 }

# Update README
(Get-Content readme.md).Replace($PREVVERSION, $VERSION) | Set-Content readme.md
git add readme.md
git commit -m "Release $VERSION"
if ($LastExitCode -ne 0) { exit 1 }

# Tag
git tag -a $VERSION -m "Version $VERSION"
if ($LastExitCode -ne 0) { exit 1 }

# Push new tag
git push origin $VERSION
if ($LastExitCode -ne 0) { exit 1 }

# Create release
gh release create $VERSION --generate-notes --verify-tag .\dist\gomas-$VERSION-arm64.tgz .\dist\gomas-$VERSION-arm32.tgz
if ($LastExitCode -ne 0) { exit 1 }
