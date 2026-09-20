// Copyright 2025 ko Build Authors All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/spf13/cobra"

	"github.com/google/ko/pkg/commands/options"
	"github.com/google/ko/pkg/publish"
)

// baseImageResolver resolves a base image reference to a digest-pinned
// reference in the same repository.
type baseImageResolver func(ctx context.Context, ref string) (string, error)

// remoteBaseImageResolver resolves base images from a remote registry, or
// from the local daemon for ko.local references.
func remoteBaseImageResolver(insecure bool) baseImageResolver {
	return func(ctx context.Context, ref string) (string, error) {
		var nameOpts []name.Option
		if insecure {
			nameOpts = append(nameOpts, name.Insecure)
		}
		r, err := name.ParseReference(strings.TrimPrefix(ref, "ko://"), nameOpts...)
		if err != nil {
			return "", fmt.Errorf("parsing base image reference: %w", err)
		}
		if r.Context().RegistryStr() == publish.LocalDomain {
			img, err := daemon.Image(r, daemon.WithContext(ctx))
			if err != nil {
				return "", fmt.Errorf("loading %s from daemon: %w", r, err)
			}
			d, err := img.Digest()
			if err != nil {
				return "", err
			}
			return r.Context().Digest(d.String()).String(), nil
		}
		desc, err := remote.Get(r,
			remote.WithAuthFromKeychain(keychain),
			remote.WithContext(ctx),
			remote.WithUserAgent(ua()),
		)
		if err != nil {
			return "", fmt.Errorf("resolving %s: %w", r, err)
		}
		return r.Context().Digest(desc.Digest.String()).String(), nil
	}
}

// addLock augments our CLI surface with the lock command.
func addLock(topLevel *cobra.Command) {
	bo := &options.BuildOptions{}
	var verify bool
	var insecure bool

	lock := &cobra.Command{
		Use:   "lock",
		Short: "Generate or verify the lock file pinning base images to digests.",
		Long: `This sub-command resolves the base images configured in .ko.yaml
(defaultBaseImage and baseImageOverrides) to digest-pinned references and
writes them to the ko lock file (default: ko.lock).

Builds run with --reproducible (or KO_REPRODUCIBLE=true) read the lock file
and use the pinned digests, so the same commit always builds on top of the
same base image.

Use --verify to check that the lock file still matches what the configured
base image references resolve to, e.g. in CI.`,
		Example: `
  # Write or update ko.lock with the currently resolved base image digests.
  ko lock

  # Fail if ko.lock is missing, stale, or out of sync with .ko.yaml.
  ko lock --verify`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bo.InsecureRegistry = insecure
			if err := bo.LoadConfig(); err != nil {
				return err
			}
			resolve := remoteBaseImageResolver(insecure)
			path := lockFilePath(bo)

			if verify {
				return verifyLockFile(cmd.Context(), bo, path, resolve, cmd.OutOrStdout())
			}

			lf, err := lockFileFor(bo, adaptResolver(cmd.Context(), resolve))
			if err != nil {
				return err
			}
			if err := lf.write(path); err != nil {
				return fmt.Errorf("writing lock file: %w", err)
			}
			log.Printf("Wrote %s", path)
			return nil
		},
	}
	lock.Flags().BoolVar(&verify, "verify", false,
		"Verify the lock file matches the current resolution of the configured base images, instead of writing it.")
	lock.Flags().BoolVar(&insecure, "insecure-registry", false,
		"Whether to skip TLS verification on the registry")
	lock.Flags().StringVar(&bo.LockFile, "lock-file", options.DefaultLockFile,
		"Path to the ko lock file.")
	topLevel.AddCommand(lock)
}

func adaptResolver(ctx context.Context, resolve baseImageResolver) func(ref string) (string, error) {
	return func(ref string) (string, error) {
		return resolve(ctx, ref)
	}
}

func verifyLockFile(ctx context.Context, bo *options.BuildOptions, path string, resolve baseImageResolver, out io.Writer) error {
	locked, err := readLockFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("lock file %s does not exist; run `ko lock` to create it", path)
	}
	if err != nil {
		return err
	}
	for key, ref := range locked.entries() {
		if err := parseLockedReference(ref); err != nil {
			return fmt.Errorf("%s: %w", lockEntryName(key), err)
		}
	}

	current, err := lockFileFor(bo, adaptResolver(ctx, resolve))
	if err != nil {
		return err
	}

	diffs := diffLock(locked, current)
	if len(diffs) == 0 {
		fmt.Fprintf(out, "%s is up to date\n", path)
		return nil
	}
	for _, d := range diffs {
		fmt.Fprintf(out, "MISMATCH: %s\n", d)
	}
	return fmt.Errorf("lock file %s is out of date; run `ko lock` to update it", path)
}
