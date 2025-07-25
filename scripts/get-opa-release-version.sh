#!/usr/bin/env bash

set -eo pipefail

OPA_VERSION="$1"
if curl --silent --fail https://api.github.com/repos/open-policy-agent/opa/releases/tags/${OPA_VERSION} >/dev/null; then
  echo $OPA_VERSION
  exit
fi

echo "latest"