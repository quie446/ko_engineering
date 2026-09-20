## ko lock

Pin base image references to digests in a lockfile

### Synopsis

This sub-command resolves the base images configured in .ko.yaml
(defaultBaseImage and baseImageOverrides) to immutable digests and records
them in a lockfile (ko.lock by default) that can be committed to version
control.

Builds can then consume the lockfile with --locked (or KO_LOCKED=true) to
guarantee that every build uses the exact same base image, which is a
prerequisite for reproducible image digests.

```
ko lock [flags]
```

### Examples

```

  # Write or refresh ko.lock for the current .ko.yaml configuration.
  ko lock

  # Verify that ko.lock still matches the current base image resolution,
  # e.g. in CI. Exits non-zero when the lockfile is out of date.
  ko lock --check
```

### Options

```
      --check               Verify the lockfile matches the current base image resolution instead of updating it.
  -h, --help                help for lock
      --insecure-registry   Whether to allow insecure (HTTP) communication with registries when resolving base images.
      --lockfile string     Path to the base image lockfile. (default "ko.lock")
```

### Options inherited from parent commands

```
  -v, --verbose   Enable debug logs
```

### SEE ALSO

* [ko](ko.md)	 - Rapidly iterate with Go, Containers, and Kubernetes.

