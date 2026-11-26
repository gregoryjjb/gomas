
docker build --tag xgomas-arm64:latest --file scripts/Dockerfile.arm64 ./
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
docker build --tag xgomas-arm32:latest --file scripts/Dockerfile.arm32 ./
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
