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
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/ko/pkg/commands/options"
)

// fakeResolver pins every ref to a digest derived from the ref name, e.g.
// "example.com/base:latest" -> "example.com/base@sha256:<64 chars>".
func fakeResolver(t *testing.T, digests map[string]string) func(ref string) (string, error) {
	t.Helper()
	return func(ref string) (string, error) {
		d, ok := digests[ref]
		if !ok {
			return "", fmt.Errorf("unexpected ref %q", ref)
		}
		repo, _, _ := strings.Cut(ref, ":")
		repo, _, _ = strings.Cut(repo, "@")
		return repo + "@" + d, nil
	}
}

const (
	digestA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	digestB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	digestC = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
)

func testBuildOptions() *options.BuildOptions {
	return &options.BuildOptions{
		BaseImage: "example.com/base:latest",
		BaseImageOverrides: map[string]string{
			"example.com/foo/cmd/bar": "example.com/other:v1",
		},
	}
}

func TestLockFileRoundTrip(t *testing.T) {
	bo := testBuildOptions()
	resolve := fakeResolver(t, map[string]string{
		"example.com/base:latest": digestA,
		"example.com/other:v1":    digestB,
	})

	lf, err := lockFileFor(bo, resolve)
	if err != nil {
		t.Fatalf("lockFileFor: %v", err)
	}
	if got, want := lf.DefaultBaseImage, "example.com/base@"+digestA; got != want {
		t.Errorf("DefaultBaseImage = %q, want %q", got, want)
	}
	if got, want := lf.BaseImageOverrides["example.com/foo/cmd/bar"], "example.com/other@"+digestB; got != want {
		t.Errorf("BaseImageOverrides = %q, want %q", got, want)
	}

	path := filepath.Join(t.TempDir(), "ko.lock")
	if err := lf.write(path); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := readLockFile(path)
	if err != nil {
		t.Fatalf("readLockFile: %v", err)
	}
	if diffs := diffLock(lf, got); len(diffs) != 0 {
		t.Errorf("round trip produced diffs: %v", diffs)
	}
}

func TestLockFileApplyToBuildOptions(t *testing.T) {
	bo := testBuildOptions()
	lf := &lockFile{
		DefaultBaseImage: "example.com/base@" + digestA,
		BaseImageOverrides: map[string]string{
			"example.com/foo/cmd/bar": "example.com/other@" + digestB,
		},
	}
	lf.applyToBuildOptions(bo)
	if got := bo.BaseImage; got != lf.DefaultBaseImage {
		t.Errorf("BaseImage = %q, want %q", got, lf.DefaultBaseImage)
	}
	if got := bo.BaseImageOverrides["example.com/foo/cmd/bar"]; got != "example.com/other@"+digestB {
		t.Errorf("BaseImageOverrides = %q", got)
	}
}

func TestDiffLock(t *testing.T) {
	locked := &lockFile{
		DefaultBaseImage: "example.com/base@" + digestA,
		BaseImageOverrides: map[string]string{
			"example.com/foo/cmd/bar": "example.com/other@" + digestB,
		},
	}

	t.Run("in sync", func(t *testing.T) {
		if diffs := diffLock(locked, locked); len(diffs) != 0 {
			t.Errorf("diffLock = %v, want none", diffs)
		}
	})

	t.Run("digest drift", func(t *testing.T) {
		current := &lockFile{
			DefaultBaseImage: "example.com/base@" + digestC,
			BaseImageOverrides: map[string]string{
				"example.com/foo/cmd/bar": "example.com/other@" + digestB,
			},
		}
		diffs := diffLock(locked, current)
		if len(diffs) != 1 || !strings.Contains(diffs[0], "defaultBaseImage") {
			t.Errorf("diffLock = %v, want one defaultBaseImage drift", diffs)
		}
	})

	t.Run("missing and extra entries", func(t *testing.T) {
		current := &lockFile{
			DefaultBaseImage: "example.com/base@" + digestA,
			BaseImageOverrides: map[string]string{
				"example.com/foo/cmd/baz": "example.com/other@" + digestB,
			},
		}
		diffs := diffLock(locked, current)
		if len(diffs) != 2 {
			t.Errorf("diffLock = %v, want two diffs", diffs)
		}
	})
}

func TestVerifyLockFile(t *testing.T) {
	bo := testBuildOptions()
	resolve := func(_ context.Context, ref string) (string, error) {
		return fakeResolver(t, map[string]string{
			"example.com/base:latest": digestA,
			"example.com/other:v1":    digestB,
		})(ref)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "ko.lock")

	t.Run("missing lock file fails", func(t *testing.T) {
		var out bytes.Buffer
		if err := verifyLockFile(context.Background(), bo, path, resolve, &out); err == nil {
			t.Error("verifyLockFile succeeded, want error for missing lock file")
		}
	})

	lf, err := lockFileFor(bo, fakeResolver(t, map[string]string{
		"example.com/base:latest": digestA,
		"example.com/other:v1":    digestB,
	}))
	if err != nil {
		t.Fatalf("lockFileFor: %v", err)
	}
	if err := lf.write(path); err != nil {
		t.Fatalf("write: %v", err)
	}

	t.Run("up to date", func(t *testing.T) {
		var out bytes.Buffer
		if err := verifyLockFile(context.Background(), bo, path, resolve, &out); err != nil {
			t.Errorf("verifyLockFile: %v", err)
		}
	})

	t.Run("stale lock file fails", func(t *testing.T) {
		staleResolve := func(_ context.Context, ref string) (string, error) {
			return fakeResolver(t, map[string]string{
				"example.com/base:latest": digestC, // base image moved
				"example.com/other:v1":    digestB,
			})(ref)
		}
		var out bytes.Buffer
		err := verifyLockFile(context.Background(), bo, path, staleResolve, &out)
		if err == nil {
			t.Error("verifyLockFile succeeded, want error for stale lock file")
		}
		if !strings.Contains(out.String(), "MISMATCH") {
			t.Errorf("output %q does not report the mismatch", out.String())
		}
	})

	t.Run("non-digest lock entry fails", func(t *testing.T) {
		bad := &lockFile{DefaultBaseImage: "example.com/base:latest"}
		badPath := filepath.Join(dir, "bad.lock")
		if err := bad.write(badPath); err != nil {
			t.Fatalf("write: %v", err)
		}
		var out bytes.Buffer
		if err := verifyLockFile(context.Background(), bo, badPath, resolve, &out); err == nil {
			t.Error("verifyLockFile succeeded, want error for non-digest lock entry")
		}
	})
}

func TestApplyLockFileToBuildOptionsMissing(t *testing.T) {
	bo := testBuildOptions()
	path := filepath.Join(t.TempDir(), "ko.lock")
	if err := applyLockFileToBuildOptions(bo, path); err != nil {
		t.Fatalf("applyLockFileToBuildOptions: %v", err)
	}
	if bo.BaseImage != "example.com/base:latest" {
		t.Errorf("BaseImage = %q, want unchanged", bo.BaseImage)
	}
}

func TestLockFilePath(t *testing.T) {
	bo := &options.BuildOptions{WorkingDirectory: "/work", LockFile: "ko.lock"}
	if got, want := lockFilePath(bo), "/work/ko.lock"; got != want {
		t.Errorf("lockFilePath = %q, want %q", got, want)
	}
	bo.LockFile = "/abs/ko.lock"
	if got, want := lockFilePath(bo), "/abs/ko.lock"; got != want {
		t.Errorf("lockFilePath = %q, want %q", got, want)
	}
}

func TestAppendUnlessFlag(t *testing.T) {
	if got := appendUnlessFlag(nil, "-buildvcs", "-buildvcs=false"); len(got) != 1 || got[0] != "-buildvcs=false" {
		t.Errorf("appendUnlessFlag = %v", got)
	}
	in := []string{"-buildvcs=true"}
	if got := appendUnlessFlag(in, "-buildvcs", "-buildvcs=false"); len(got) != 1 || got[0] != "-buildvcs=true" {
		t.Errorf("appendUnlessFlag = %v, want unchanged", got)
	}
}

func TestReproducibleGobuildOptions(t *testing.T) {
	bo := &options.BuildOptions{Reproducible: true, SBOM: "none"}
	if _, err := gobuildOptions(bo); err != nil {
		t.Fatalf("gobuildOptions: %v", err)
	}
	if !contains(bo.DefaultFlags, "-buildvcs=false") {
		t.Errorf("DefaultFlags = %v, want -buildvcs=false", bo.DefaultFlags)
	}
	if !contains(bo.DefaultLdflags, "-buildid=") {
		t.Errorf("DefaultLdflags = %v, want -buildid=", bo.DefaultLdflags)
	}

	// User-provided values win.
	bo = &options.BuildOptions{
		Reproducible:   true,
		SBOM:           "none",
		DefaultFlags:   []string{"-buildvcs=true"},
		DefaultLdflags: []string{"-buildid=0xdeadbeef"},
	}
	if _, err := gobuildOptions(bo); err != nil {
		t.Fatalf("gobuildOptions: %v", err)
	}
	if contains(bo.DefaultFlags, "-buildvcs=false") {
		t.Errorf("DefaultFlags = %v, user value should win", bo.DefaultFlags)
	}
	if contains(bo.DefaultLdflags, "-buildid=") {
		t.Errorf("DefaultLdflags = %v, user value should win", bo.DefaultLdflags)
	}
}

func TestReproducibleRespectsSourceDateEpoch(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "not-a-number")
	bo := &options.BuildOptions{Reproducible: true, SBOM: "none"}
	if _, err := gobuildOptions(bo); err == nil {
		t.Error("gobuildOptions succeeded, want error for invalid SOURCE_DATE_EPOCH")
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
