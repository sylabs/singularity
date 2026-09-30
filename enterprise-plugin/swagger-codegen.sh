#!/bin/bash

set -e
set -u

echo "Updating swagger API yml & codegen client to latest master"

# Local path to go-swagger binary
#swagger="swagger"
# Should also be able to run via docker, but doesn't work due to SELinux
# setup on my Fedora machine
swagger="docker run --rm -it --user $(id -u):$(id -g) -e GOPATH=$HOME/go:/go -v $HOME:$HOME -w $(pwd) quay.io/goswagger/swagger"

echo "Cloning repositories..."
mkdir swagger-tmp
git clone --depth=1 git@github.com:/sylabs/oci-library-shim ./swagger-tmp/cloud-library
git clone --depth=1 git@github.com:/sylabs/remote-build ./swagger-tmp/remote-build
git clone --depth=1 git@github.com:/sylabs/key-service ./swagger-tmp/key-service
git clone --depth=1 git@github.com:/sylabs/auth-service ./swagger-tmp/auth-service

echo "Updating API specifications..."
mkdir -p api
mkdir -p ./api/cloud-library
cp -r ./swagger-tmp/cloud-library/api/* ./api/cloud-library
mkdir -p ./api/remote-build
cp -r ./swagger-tmp/remote-build/api/* ./api/remote-build
mkdir -p ./api/key-service
cp -r ./swagger-tmp/key-service/api/* ./api/key-service
mkdir -p ./api/auth-service
cp -r ./swagger-tmp/auth-service/api/* ./api/auth-service

echo "Removing temporary repositories..."
rm -rf ./swagger-tmp

echo "API client codegen..."
rm -rf ./pkg/library
mkdir -p ./pkg/library
$swagger generate client -f ./api/cloud-library/v1/openapi.yml -t ./pkg/library

rm -rf ./pkg/buildmanaged
mkdir -p ./pkg/buildmanager
$swagger generate client -f ./api/remote-build/manager/v1/openapi.yml -t ./pkg/buildmanager

rm -rf ./pkg/buildserver
mkdir -p ./pkg/buildserver
$swagger generate client -f ./api/remote-build/server/v1/openapi.yml -t ./pkg/buildserver

rm -rf ./pkg/keyservice
mkdir -p ./pkg/keyservice
$swagger generate client -f ./api/key-service/v1/openapi.yml -t ./pkg/keyservice

rm -rf ./pkg/consentservice
mkdir -p ./pkg/consentservice
$swagger generate client -f ./api/auth-service/consent-service/v1/openapi.yml -t ./pkg/consentservice

rm -rf ./pkg/tokenservice
mkdir -p ./pkg/tokenservice
$swagger generate client -f ./api/auth-service/token-service/v1/openapi.yml -t ./pkg/tokenservice

