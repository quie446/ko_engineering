#!/usr/bin/env bash

# Copyright 2026 ko Build Authors All Rights Reserved.
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

# Verifies that ko produces reproducible image digests and that the base
# image lockfile (when present) matches the current resolution.
#
# Usage:
#   hack/verify-reproducible.sh [IMPORTPATH]
#
# Run it from the root of the Go module you want to verify (the directory
# containing .ko.yaml and, optionally, ko.lock).
#
# Environment:
#   KO             - ko binary to exercise (default: ko from PATH)
#   KO_LOCKFILE    - path to the base image lockfile (default: ko.lock)
#   KO_DOCKER_REPO - repository used for the (unpushed) build references
#                    (default: example.com/ko-reproducible)
#
# The builds run with --push=false, so nothing is published; only the
# resulting digests are compared.

set -euo pipefail

IMPORTPATH="${1:-.}"
KO="${KO:-ko}"
LOCKFILE="${KO_LOCKFILE:-ko.lock}"
export KO_DOCKER_REPO="${KO_DOCKER_REPO:-example.com/ko-reproducible}"

LOCK_ARGS=()
if [[ -f "$LOCKFILE" ]]; then
  echo "==> Checking base image lockfile ($LOCKFILE)"
  "$KO" lock --check --lockfile="$LOCKFILE"
  LOCK_ARGS=(--locked --lockfile="$LOCKFILE")
else
  echo "==> No lockfile at $LOCKFILE; run 'ko lock' to pin base images"
fi

build_digest() {
  "$KO" build --push=false --reproducible ${LOCK_ARGS[@]+"${LOCK_ARGS[@]}"} "$IMPORTPATH" \
    | grep -oE 'sha256:[0-9a-f]{64}' | head -1
}

echo "==> Building $IMPORTPATH twice in reproducible mode"
DIGEST1="$(build_digest)"
DIGEST2="$(build_digest)"

echo "first build:  $DIGEST1"
echo "second build: $DIGEST2"

if [[ -z "$DIGEST1" || -z "$DIGEST2" ]]; then
  echo "ERROR: could not determine image digest from ko build output" >&2
  exit 1
fi
if [[ "$DIGEST1" != "$DIGEST2" ]]; then
  echo "ERROR: digests differ; build is not reproducible" >&2
  exit 1
fi
echo "==> OK: builds are reproducible ($DIGEST1)"
