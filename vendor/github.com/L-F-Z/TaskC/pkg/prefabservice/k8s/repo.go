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

package k8s

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"github.com/L-F-Z/TaskC/pkg/dcontext"
	"github.com/L-F-Z/TaskC/pkg/prefab"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/dockerhub"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

const SERVICE_BASE string = "https://registry.k8s.io"

type Repo struct {
	arch string
}

func (r *Repo) Init(ctx *dcontext.DeployContext) (err error) {
	value, exists := ctx.Get(dcontext.ARCH_KEY)
	if !exists {
		return fmt.Errorf("unable to get hardware architecture from context: %v", err)
	}
	arch, ok := value.(string)
	if !ok {
		return fmt.Errorf("context[hardware, architecture] is not a string")
	}
	r.arch = arch
	return
}

func NameNormalizer(name string) (normalized string) {
	return name
}

func (r *Repo) GetEnvSpec() repointerface.EnvSpec {
	return EnvSpec{Arch: r.arch}
}

func (r *Repo) GetVersions(name string) (versions []repointerface.Version, err error) {
	name = NameNormalizer(name)
	tags, err := dockerhub.GetTags(name, SERVICE_BASE)
	if err != nil {
		err = fmt.Errorf("unable to request versions from k8s.io: %v", err)
		return
	}
	for _, tag := range tags {
		versions = append(versions, Version(tag))
	}
	return
}

func (r *Repo) GetEnvs(name string, version repointerface.Version) (envs []string, err error) {
	name = NameNormalizer(name)
	envMap, err := dockerhub.GetEnvs(name, version.String(), SERVICE_BASE)
	if err != nil {
		err = fmt.Errorf("unable to request envs from k8s.io: %v", err)
		return
	}
	for env := range envMap {
		envs = append(envs, env)
	}
	return
}

func (r *Repo) Fabricate(name string, version repointerface.Version, envs []string, dstDir string) (prefabPaths []string, blueprintPaths []string, fileType string, err error) {
	fileType = repointerface.FILETYPE_COMPRESS
	envMap, err := dockerhub.GetEnvs(name, version.String(), SERVICE_BASE)
	if err != nil {
		err = fmt.Errorf("unable to request envs from k8s.io: %v", err)
		return
	}
	for env := range envMap {
		if slices.Contains(envs, env) {
			var prefabPath, blueprintPath string
			prefabPath, blueprintPath, err = fabricate(name, version.String(), env, envMap[env], dstDir)
			if err != nil {
				return
			}
			prefabPaths = append(prefabPaths, prefabPath)
			blueprintPaths = append(blueprintPaths, blueprintPath)
		}
	}
	return
}

func fabricate(name string, version string, env string, digest string, dstDir string) (prefabPath string, blueprintPath string, err error) {
	tmpRootFs, err := os.MkdirTemp("", repointerface.REPO_K8S)
	if err != nil {
		return
	}
	defer os.RemoveAll(tmpRootFs)
	configRaw, err := dockerhub.GetImage(name, digest, tmpRootFs, SERVICE_BASE)
	if err != nil {
		err = fmt.Errorf("error occured when getting image: %v", err)
		return
	}
	var config configFile
	err = json.Unmarshal(configRaw, &config)
	if err != nil {
		err = fmt.Errorf("error parsing image config file: %v", err)
		return
	}

	// Generate Prefab and Blueprint
	blueprint := prefab.NewBlueprint()
	blueprint.Type = repointerface.REPO_K8S
	blueprint.Name = name
	blueprint.Version = version
	blueprint.Environment = env
	blueprint.User = config.Config.User
	blueprint.WorkDir = config.Config.WorkingDir
	blueprint.EnvVar = config.Config.Env
	blueprint.EntryPoint = config.Config.Entrypoint
	blueprint.Command = config.Config.Cmd

	// Set Deploy Context, ignore errors
	blueprint.Context.SetLibCVersion(tmpRootFs)
	blueprint.Context.SetPythonBinPath(tmpRootFs)
	blueprint.Context.SetPythonVersion(tmpRootFs)
	return prefab.Pack(tmpRootFs, dstDir, blueprint)
}

type configFile struct {
	Config struct {
		User       string   `json:"User"`
		Env        []string `json:"Env"`
		Cmd        []string `json:"Cmd"`
		Entrypoint []string `json:"Entrypoint"`
		WorkingDir string   `json:"WorkingDir"`
	} `json:"config"`
}
