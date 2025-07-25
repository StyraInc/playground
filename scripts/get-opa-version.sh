#!/usr/bin/env bash

OPA_VERSION="$(go list -m github.com/open-policy-agent/opa | awk '{print $2}')"

if [[ -n "${OPA_VERSION}" ]]; then
  echo "${OPA_VERSION}"
else
  echo "v??"
fi
