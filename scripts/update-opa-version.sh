#!/usr/bin/env bash
# Script to revendor OPA and add changes if needed

set +e
set -x

usage() {
    echo "update-opa-version.sh <VERSION> eg. update-opa-version.sh v0.8.0"
}

# Check if OPA version provided
if [ $# -eq 0 ]
  then
    echo "OPA version not provided"
    usage
    exit 1
fi

export GO111MODULE=on

# Check if OPA update required
current=$(GOFLAGS=-mod=vendor go list -m -f '{{ .Version }}' github.com/open-policy-agent/opa)
if [ $current = $1 ]
  then
    printf "OPA already at %s\n" $1
    exit 0
fi

VERSION=$1

# Update OPA version
go get -u "github.com/open-policy-agent/opa@${VERSION}"
go mod tidy

git status |  grep  go.mod
if [ $? -eq 0 ]; then

  # update vendor
  go mod vendor

  # add changes
  git add .
fi
