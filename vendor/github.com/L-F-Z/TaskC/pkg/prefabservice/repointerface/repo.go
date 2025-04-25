// Copyright 2025 Fengzhi Li
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package repointerface

import (
	"github.com/L-F-Z/TaskC/pkg/dcontext"
)

const REPO_PYPI = "PyPI"
const REPO_DOCKERHUB = "DockerHub"
const REPO_APT = "Apt"
const REPO_HUGGINGFACE = "HuggingFace"
const REPO_PREFAB = "Prefab"
const REPO_CLOSURE = "Closure"
const REPO_K8S = "k8s"

const FILETYPE_RAW string = "application/octet-stream"
const FILETYPE_COMPRESS string = "application/gzip"

type Repo interface {
	// init repository parameters by deployment context
	Init(context *dcontext.DeployContext) (err error)
	// generate a Environemnt SpecSheet by repository parametes and the given name & specifier
	// Should Init() the Repo before calling this function
	GetEnvSpec() EnvSpec

	// Finding 0 version is not an error, should return []Version{} and nil
	// This function MUST NOT rely on deployment context
	GetVersions(name string) (versions []Version, err error)
	SelectVersion(versions []Version) (selected Version, err error)

	// Finding 0 environment is not an error, should return []string{} and nil
	// This function MUST NOT rely on deployment context
	GetEnvs(name string, version Version) (envs []string, err error)
	SelectEnv(envs []string, envSpec EnvSpec) (selected string, err error)
	FilterEnv(envs []string) (selected []string)

	// fabricate a prefab according to a SpecSheet, then save the prefab and blueprint to dstDir
	// and return the path to prefab and blueprint
	// This function MUST NOT rely on deployment context
	Fabricate(name string, version Version, envs []string, dstDir string) (prefabPaths []string, blueprintPaths []string, fileType string, err error)
}
