# cat <<EOF | docker run --mount type=bind,source="$(pwd)",target=/gomas -i xgomas bash
# cd /gomas
# CC=aarch64-linux-gnu-gcc GOOS=linux GOARCH=arm64 GOARM=7 CGO_ENABLED=1 go build -o ./bin/arm64/gomas .
# EOF

docker run --mount type=bind,source="$(pwd)",target=/gomas --interactive --workdir /gomas xgomas bash /gomas/make.sh
