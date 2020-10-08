/* Copyright 2019 The Bazel Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bazel_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bazelbuild/bazel-gazelle/testtools"
	"github.com/bazelbuild/rules_go/go/tools/bazel_testing"
)

var testArgs = bazel_testing.Args{
	Main: `
-- BUILD.bazel --
load("@bazel_gazelle//:def.bzl", "gazelle")

# gazelle:prefix example.com/m

gazelle(name = "gazelle")

gazelle(
    name = "gazelle-update-repos",
    args = [
        "-from_file=go.mod",
        "-to_macro=deps.bzl%go_repositories",
    ],
    command = "update-repos",
)

-- go.mod --
module example.com/m

go 1.15
-- hello.go --
package main

func main() {}
`,
	WorkspaceSuffix: `
load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies", "go_repository")

gazelle_dependencies()

# gazelle:repo test

go_repository(
    name = "errors_go_git",
    importpath = "github.com/pkg/errors",
    commit = "30136e27e2ac8d167177e8a583aa4c3fea5be833",
    patches = ["@bazel_gazelle//internal:repository_rules_test_errors.patch"],
    patch_args = ["-p1"],
    build_naming_convention = "go_default_library",
)

go_repository(
    name = "errors_go_mod",
    importpath = "github.com/pkg/errors",
    version = "v0.8.1",
    sum ="h1:iURUrRGxPUNPdy5/HRSm+Yj6okJ6UtLINN0Q9M4+h3I=",
)

go_repository(
		name = "com_github_apex_log",
		build_directives = ["gazelle:exclude handlers"],
		importpath = "github.com/apex/log",
		sum = "h1:J5rld6WVFi6NxA6m8GJ1LJqu3+GiTFIt3mYv27gdQWI=",
		version = "v1.1.0",
)
`,
}

func TestMain(m *testing.M) {
	bazel_testing.TestMain(m, testArgs)
}

func TestBuild(t *testing.T) {
	if err := bazel_testing.RunBazel("build", "@errors_go_git//:errors", "@errors_go_mod//:go_default_library"); err != nil {
		t.Fatal(err)
	}
}

func TestDirectives(t *testing.T) {
	err := bazel_testing.RunBazel("query", "@com_github_apex_log//handlers/...")
	if err == nil {
		t.Fatal("Should not generate build files for @com_github_apex_log//handlers/...")
	}
	if !strings.Contains(err.Error(), "no targets found beneath 'handlers'") {
		t.Fatal("Unexpected error:\n", err)
	}
}

func TestRepoConfig(t *testing.T) {
	if err := bazel_testing.RunBazel("build", "@bazel_gazelle_go_repository_config//:all"); err != nil {
		t.Fatal(err)
	}
	outputBase, err := getBazelOutputBase()
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(outputBase, "external/bazel_gazelle_go_repository_config")
	testtools.CheckFiles(t, outDir, []testtools.FileSpec{
		{
			Path: "WORKSPACE",
			Content: `
# Code generated by generate_repo_config.go; DO NOT EDIT.
# gazelle:repo test

go_repository(
    name = "com_github_apex_log",
    importpath = "github.com/apex/log",
)

go_repository(
    name = "errors_go_git",
    importpath = "github.com/pkg/errors",
)

go_repository(
    name = "errors_go_mod",
    importpath = "github.com/pkg/errors",
)
`,
		},
	})
}

func TestModcacheRW(t *testing.T) {
	if err := bazel_testing.RunBazel("query", "@errors_go_mod//:go_default_library"); err != nil {
		t.Fatal(err)
	}
	out, err := bazel_testing.BazelOutput("info", "output_base")
	if err != nil {
		t.Fatal(err)
	}
	outputBase := strings.TrimSpace(string(out))
	dir := filepath.Join(outputBase, "external/bazel_gazelle_go_repository_cache/pkg/mod/github.com/pkg/errors@v0.8.1")
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0200 == 0 {
		t.Fatal("module cache is read-only")
	}
}

// TODO(bazelbuild/rules_go#2189): call bazel_testing.BazelOutput once implemented.
func getBazelOutputBase() (string, error) {
	cmd := exec.Command("bazel", "info", "output_base")
	for _, e := range os.Environ() {
		// Filter environment variables set by the bazel test wrapper script.
		// These confuse recursive invocations of Bazel.
		if strings.HasPrefix(e, "TEST_") || strings.HasPrefix(e, "RUNFILES_") {
			continue
		}
		cmd.Env = append(cmd.Env, e)
	}
	buf := &bytes.Buffer{}
	cmd.Stdout = buf
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}
