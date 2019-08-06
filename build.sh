#! /usr/bin/env bash
set -e

cd $(dirname "$0")

DO_GENERATE=true
DO_TEST=true
DO_VERIFY=true

while [[ $# -gt 0 ]]; do
  key="$1"
  case $key in
    --no-generate)
      DO_GENERATE=false
      shift
      ;;
    --no-test)
      DO_TEST=false
      shift
      ;;
    --no-verify)
      DO_VERIFY=false
      shift
      ;;
  esac  
done

mkdir -p ./assets
mkdir -p ./bin
mkdir -p ./mocks

export PATH=$PWD/bin:$PATH

if ! [ -x $PWD/bin/go-bindata ]; then
  echo "Building go-bindata"
  go build -o $PWD/bin/go-bindata ./vendor/github.com/kevinburke/go-bindata/go-bindata
fi

if ! [ -x $PWD/bin/mockgen ]; then
  echo "Building mockgen"
  go build -o $PWD/bin/mockgen ./vendor/github.com/golang/mock/mockgen
fi

export GO111MODULE=on

if $DO_VERIFY; then
  echo "Verifying modules"
  go mod verify
fi

echo "Running generator"
if $DO_GENERATE; then
  echo "Running go generate"
  go generate ./...

  echo "Generating assets"
  cd templates
  go-bindata -ignore *.go -pkg assets ./...
  mv -f bindata.go ../assets
  cd ..
fi

echo "Building..."
go build -ldflags "-s -w" -mod=readonly -mod=vendor -o ./bin/options ./main/main.go

if $DO_TEST; then
  echo "Running tests"
  go test ./... -count=1
fi

echo "Done."
