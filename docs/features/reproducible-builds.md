# Reproducible Builds

`ko` can produce bit-for-bit reproducible image digests: the same module
path, the same commit, and the same base image reference always yield the
same image digest. This is opt-in, so existing builds are unaffected.

## Quick start

```shell
# 1. Pin the base images from .ko.yaml to digests in ko.lock, and commit it.
ko lock
git add ko.lock && git commit -m "Pin base images"

# 2. Build reproducibly (flag or environment variable).
ko build --reproducible ./cmd/myapp
# ...or:
KO_REPRODUCIBLE=true ko build ./cmd/myapp

# 3. Verify: building twice must give the same digest.
hack/verify-reproducible.sh ./cmd/myapp
```

## What `--reproducible` changes

Enabling reproducible mode (via `--reproducible` or `KO_REPRODUCIBLE=true`)
removes the remaining sources of nondeterminism:

- **Timestamps**: image config and file timestamps are pinned to the Unix
  epoch, unless you set `SOURCE_DATE_EPOCH` / `KO_DATA_DATE_EPOCH`
  explicitly (those still win).
- **VCS stamping**: `-buildvcs=false` is added to the default Go build
  flags, so the embedded `vcs.time` / `vcs.modified` settings cannot leak
  working-tree state into the binary.
- **Build ID**: `-buildid=` is added to the default ldflags, stripping the
  Go build ID.
- **Base images**: references from `.ko.yaml` are replaced with the
  digest-pinned references recorded in the lock file (default: `ko.lock`,
  override with `--lock-file`).

`-trimpath` remains on by default and is required for reproducibility; do
not combine reproducible mode with `--debug`, which disables optimizations.

Note that the Go toolchain version still affects the output binary. For
cross-machine reproducibility, pin the Go version as well (e.g. via
`go-version` in CI).

## The lock file

`ko lock` resolves `defaultBaseImage` and every `baseImageOverrides` entry
from `.ko.yaml` to a digest and writes `ko.lock`, mirroring the `.ko.yaml`
shape:

```yaml
defaultBaseImage: cgr.dev/chainguard/static@sha256:...
baseImageOverrides:
  github.com/example/app/cmd/server: gcr.io/distroless/static@sha256:...
```

Commit `ko.lock` alongside `.ko.yaml`. To roll back to an older base image,
check out the older lock file (or run `ko lock` against a pinned tag).

### Verifying the lock file

```shell
ko lock --verify
```

re-resolves the configured base images and fails with a non-zero exit code
if the lock file is missing, stale, or out of sync with `.ko.yaml` — for
example after `cgr.dev/chainguard/static:latest` moved upstream.

## CI example

See [`.github/workflows/reproducible.yaml`](https://github.com/ko-build/ko/blob/main/.github/workflows/reproducible.yaml):

```yaml
      - name: Verify base image lock file
        run: ko lock --verify

      - name: Build twice and compare digests
        run: hack/verify-reproducible.sh ./test
```

`hack/verify-reproducible.sh [MODULE]` builds the module twice with
`--push=false` (no registry needed) and fails unless both digests match.
