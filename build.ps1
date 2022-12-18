
docker run --mount type=bind,source="$(Get-Location)",target=/gomas --interactive --workdir /gomas xgomas bash /gomas/make.sh
