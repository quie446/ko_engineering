#!/usr/bin/env bash

# Copyright 2025 ko Build Authors All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# verify-reproducible.sh builds a Go module twice with reproducible settings
# and fails unless both image digests are identical.
#
# Usage:
#   hack/verify-reproducible.sh [MODULE]
#
# MODULE defaults to ./test relative to the repository root. KO may point at
# a prebuilt ko binary; otherwise ko is built from the repository.

set -o errexit
set -o nounset
set -o pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODULE="${1:-${ROOT_DIR}/test}"

# The digest is computed before publishing, so --push=false still prints the
# exact reference that would be pushed. KO_DOCKER_REPO only needs to parse.
export KO_DOCKER_REPO="${KO_DOCKER_REPO:-example.com/ko-reproducible}"
export KO_REPRODUCIBLE=true
export CGO_ENABLED=0

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

KO="${KO:-}"
if [[ -z "${KO}" ]]; then
  KO="${TMP_DIR}/ko"
  echo "Building ko..."
  (cd "${ROOT_DIR}" && go build -o "${KO}" .)
fi

build_digest() {
  "${KO}" build --push=false "${MODULE}" 2>/dev/null \
    | grep -oE 'sha256:[0-9a-f]{64}' | head -1
}

echo "Build 1/2: ${MODULE}"
DIGEST1="$(build_digest)"
echo "  digest: ${DIGEST1}"

echo "Build 2/2: ${MODULE}"
DIGEST2="$(build_digest)"
echo "  digest: ${DIGEST2}"

if [[ -z "${DIGEST1}" || -z "${DIGEST2}" ]]; then
  echo "FAIL: could not determine image digest" && exit 1
fi
if [[ "${DIGEST1}" != "${DIGEST2}" ]]; then
  echo "FAIL: digests differ (${DIGEST1} != ${DIGEST2})" && exit 1
fi

echo "PASS: reproducible build, digest ${DIGEST1}"
