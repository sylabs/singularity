#!/bin/sh

set -e
set -u

if [ -d "vendor" ]; then
  echo "Please remove vendor directory before running this script"
  exit 255
fi

if [ ! -f "go.mod" ]; then
  echo "This script must be called from the project root directory,"
  echo "i.e. as scripts/update-license-dependencise.sh"
  exit 255
fi

$(go env GOROOT)/bin/go run github.com/google/go-licenses@v1.6.0 report ./... --ignore github.com/sylabs/singularity/v4 --template scripts/LICENSE_DEPENDENCIES.tpl > LICENSE_DEPENDENCIES.md
