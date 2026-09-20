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
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/ko/pkg/build"
	"github.com/google/ko/pkg/commands/options"
)

func writeTestLock(t *testing.T, lock *build.BaseImageLockfile) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ko.lock")
	if err := lock.Save(path); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	return path
}

func testBuildLock() *build.BaseImageLockfile {
	return &build.BaseImageLockfile{
		Version: build.BaseImageLockfileVersion,
		DefaultBaseImage: build.LockedBaseImage{
			Ref:    "cgr.dev/chainguard/static:latest",
			Pinned: "cgr.dev/chainguard/static@sha256:aaaa",
		},
		BaseImageOverrides: map[string]build.LockedBaseImage{
			"github.com/google/ko": {
				Ref:    "golang:latest",
				Pinned: "golang@sha256:bbbb",
			},
		},
	}
}

func TestApplyBaseImageLock(t *testing.T) {
	lockPath := writeTestLock(t, testBuildLock())
	bo := &options.BuildOptions{
		BaseImage:    "cgr.dev/chainguard/static:latest",
		LockfilePath: lockPath,
		BaseImageOverrides: map[string]string{
			"github.com/google/ko": "golang:latest",
		},
	}
	if err := applyBaseImageLock(bo); err != nil {
		t.Fatalf("applyBaseImageLock() = %v", err)
	}
	if got, want := bo.BaseImage, "cgr.dev/chainguard/static@sha256:aaaa"; got != want {
		t.Errorf("BaseImage = %q, want %q", got, want)
	}
	if got, want := bo.BaseImageOverrides["github.com/google/ko"], "golang@sha256:bbbb"; got != want {
		t.Errorf("BaseImageOverrides = %q, want %q", got, want)
	}
}

func TestApplyBaseImageLockDrift(t *testing.T) {
	lockPath := writeTestLock(t, testBuildLock())

	t.Run("default base image changed", func(t *testing.T) {
		bo := &options.BuildOptions{
			BaseImage:    "cgr.dev/chainguard/static:latest-glibc",
			LockfilePath: lockPath,
			BaseImageOverrides: map[string]string{
				"github.com/google/ko": "golang:latest",
			},
		}
		if err := applyBaseImageLock(bo); err == nil || !strings.Contains(err.Error(), "drifted") {
			t.Errorf("applyBaseImageLock() = %v, want drift error", err)
		}
	})

	t.Run("override changed", func(t *testing.T) {
		bo := &options.BuildOptions{
			BaseImage:    "cgr.dev/chainguard/static:latest",
			LockfilePath: lockPath,
			BaseImageOverrides: map[string]string{
				"github.com/google/ko": "golang:1.24",
			},
		}
		if err := applyBaseImageLock(bo); err == nil || !strings.Contains(err.Error(), "drifted") {
			t.Errorf("applyBaseImageLock() = %v, want drift error", err)
		}
	})

	t.Run("override missing from lock", func(t *testing.T) {
		bo := &options.BuildOptions{
			BaseImage:    "cgr.dev/chainguard/static:latest",
			LockfilePath: lockPath,
			BaseImageOverrides: map[string]string{
				"github.com/google/ko/test": "golang:latest",
			},
		}
		if err := applyBaseImageLock(bo); err == nil || !strings.Contains(err.Error(), "no locked base image") {
			t.Errorf("applyBaseImageLock() = %v, want missing entry error", err)
		}
	})

	t.Run("lockfile missing", func(t *testing.T) {
		bo := &options.BuildOptions{
			BaseImage:    "cgr.dev/chainguard/static:latest",
			LockfilePath: filepath.Join(t.TempDir(), "ko.lock"),
		}
		if err := applyBaseImageLock(bo); err == nil || !strings.Contains(err.Error(), "ko lock") {
			t.Errorf("applyBaseImageLock() = %v, want missing lockfile error", err)
		}
	})
}
