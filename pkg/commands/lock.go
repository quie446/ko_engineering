// Copyright 2026 ko Build Authors All Rights Reserved.
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
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/ko/pkg/build"
	"github.com/google/ko/pkg/commands/options"
	"github.com/spf13/cobra"
)

// addLock augments our CLI surface with lock.
func addLock(topLevel *cobra.Command) {
	var check, insecure bool
	lockfilePath := build.DefaultLockfile

	lock := &cobra.Command{
		Use:   "lock",
		Short: "Pin base image references to digests in a lockfile",
		Long: `This sub-command resolves the base images configured in .ko.yaml
(defaultBaseImage and baseImageOverrides) to immutable digests and records
them in a lockfile (ko.lock by default) that can be committed to version
control.

Builds can then consume the lockfile with --locked (or KO_LOCKED=true) to
guarantee that every build uses the exact same base image, which is a
prerequisite for reproducible image digests.`,
		Example: `
  # Write or refresh ko.lock for the current .ko.yaml configuration.
  ko lock

  # Verify that ko.lock still matches the current base image resolution,
  # e.g. in CI. Exits non-zero when the lockfile is out of date.
  ko lock --check`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bo := &options.BuildOptions{}
			if err := bo.LoadConfig(); err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			ctx := cmd.Context()
			if check {
				return checkBaseImageLock(ctx, cmd.OutOrStdout(), bo, lockfilePath, insecure)
			}
			return writeBaseImageLock(ctx, cmd.OutOrStdout(), bo, lockfilePath, insecure)
		},
	}
	lock.Flags().BoolVar(&check, "check", false,
		"Verify the lockfile matches the current base image resolution instead of updating it.")
	lock.Flags().StringVar(&lockfilePath, "lockfile", build.DefaultLockfile,
		"Path to the base image lockfile.")
	lock.Flags().BoolVar(&insecure, "insecure-registry", false,
		"Whether to allow insecure (HTTP) communication with registries when resolving base images.")
	topLevel.AddCommand(lock)
}

// resolveBaseImageLocks resolves every configured base image reference to
// its current digest and returns the result as a lockfile.
func resolveBaseImageLocks(ctx context.Context, bo *options.BuildOptions, insecure bool) (*build.BaseImageLockfile, error) {
	var nameOpts []name.Option
	if insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}

	// Cache resolutions so that identical references are only fetched once.
	resolved := map[string]build.LockedBaseImage{}
	resolve := func(ref string) (build.LockedBaseImage, error) {
		if locked, ok := resolved[ref]; ok {
			return locked, nil
		}
		r, err := name.ParseReference(ref, nameOpts...)
		if err != nil {
			return build.LockedBaseImage{}, fmt.Errorf("parsing base image reference %q: %w", ref, err)
		}
		head, err := remote.Head(r, remote.WithAuthFromKeychain(keychain), remote.WithContext(ctx))
		if err != nil {
			return build.LockedBaseImage{}, fmt.Errorf("resolving base image %q: %w", ref, err)
		}
		locked := build.LockedBaseImage{
			Ref:    ref,
			Pinned: r.Context().Digest(head.Digest.String()).String(),
		}
		resolved[ref] = locked
		return locked, nil
	}

	lock := &build.BaseImageLockfile{
		Version:            build.BaseImageLockfileVersion,
		BaseImageOverrides: map[string]build.LockedBaseImage{},
	}
	var err error
	if lock.DefaultBaseImage, err = resolve(bo.BaseImage); err != nil {
		return nil, err
	}
	for _, k := range slices.Sorted(maps.Keys(bo.BaseImageOverrides)) {
		locked, err := resolve(bo.BaseImageOverrides[k])
		if err != nil {
			return nil, err
		}
		lock.BaseImageOverrides[k] = locked
	}
	return lock, nil
}

func writeBaseImageLock(ctx context.Context, out io.Writer, bo *options.BuildOptions, path string, insecure bool) error {
	lock, err := resolveBaseImageLocks(ctx, bo, insecure)
	if err != nil {
		return err
	}
	if err := lock.Save(path); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote %s pinning %d base image(s)\n", path, 1+len(lock.BaseImageOverrides))
	return nil
}

func checkBaseImageLock(ctx context.Context, out io.Writer, bo *options.BuildOptions, path string, insecure bool) error {
	lock, err := build.LoadBaseImageLockfile(path)
	if err != nil {
		return fmt.Errorf("loading lockfile (run `ko lock` to create it): %w", err)
	}
	current, err := resolveBaseImageLocks(ctx, bo, insecure)
	if err != nil {
		return err
	}
	if drift := lock.Verify(current); len(drift) > 0 {
		for _, d := range drift {
			fmt.Fprintln(out, d)
		}
		return fmt.Errorf("%s is out of date; run `ko lock` to refresh it", path)
	}
	fmt.Fprintf(out, "%s is up to date\n", path)
	return nil
}

// applyBaseImageLock replaces the base image references in bo with the
// digest-pinned references from the lockfile, failing when the configured
// references have drifted from what was locked.
func applyBaseImageLock(bo *options.BuildOptions) error {
	lockPath := bo.LockfilePath
	if lockPath == "" {
		lockPath = build.DefaultLockfile
	}
	lock, err := build.LoadBaseImageLockfile(lockPath)
	if err != nil {
		return fmt.Errorf("--locked requires a lockfile (run `ko lock` to create one): %w", err)
	}
	if lock.DefaultBaseImage.Ref != bo.BaseImage {
		return fmt.Errorf("default base image drifted from %s: configured %q, locked %q; run `ko lock` to refresh it",
			lockPath, bo.BaseImage, lock.DefaultBaseImage.Ref)
	}
	bo.BaseImage = lock.DefaultBaseImage.Pinned
	for importpath, override := range bo.BaseImageOverrides {
		locked, ok := lock.BaseImageOverrides[importpath]
		if !ok {
			return fmt.Errorf("no locked base image for %q in %s; run `ko lock` to refresh it", importpath, lockPath)
		}
		if locked.Ref != override {
			return fmt.Errorf("base image for %q drifted from %s: configured %q, locked %q; run `ko lock` to refresh it",
				importpath, lockPath, override, locked.Ref)
		}
		bo.BaseImageOverrides[importpath] = locked.Pinned
	}
	return nil
}
