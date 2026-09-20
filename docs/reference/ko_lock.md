## ko lock

Generate or verify the lock file pinning base images to digests.

### Synopsis

This sub-command resolves the base images configured in .ko.yaml
(defaultBaseImage and baseImageOverrides) to digest-pinned references and
writes them to the ko lock file (default: ko.lock).

Builds run with --reproducible (or KO_REPRODUCIBLE=true) read the lock file
and use the pinned digests, so the same commit always builds on top of the
same base image.

Use --verify to check that the lock file still matches what the configured
base image references resolve to, e.g. in CI.

```
ko lock [flags]
```

### Examples

```

  # Write or update ko.lock with the currently resolved base image digests.
  ko lock

  # Fail if ko.lock is missing, stale, or out of sync with .ko.yaml.
  ko lock --verify
```

### Options

```
  -h, --help                help for lock
      --insecure-registry   Whether to skip TLS verification on the registry
      --lock-file string    Path to the ko lock file. (default "ko.lock")
      --verify              Verify the lock file matches the current resolution of the configured base images, instead of writing it.
```

### Options inherited from parent commands

```
  -v, --verbose   Enable debug logs
```

### SEE ALSO

* [ko](ko.md)	 - Rapidly iterate with Go, Containers, and Kubernetes.

