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

package baserepo

import (
	"strings"

	"github.com/L-F-Z/TaskC/pkg/dcontext"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

type Version string

func (v Version) String() string {
	return string(v)
}

func (a Version) Compare(other repointerface.Version) (result int) {
	return strings.Compare(a.String(), other.String())
}

type Repo struct{}

func (r *Repo) Init(ctx *dcontext.DeployContext) (err error) {
	return
}
func (r *Repo) NameNormalizer(name string) (normalized string) {
	return name
}
func (r *Repo) GetEnvSpec() (envSpec repointerface.EnvSpec) {
	return
}
func (r *Repo) GetVersions(name string) (versions []repointerface.Version, err error) {
	return
}
func (r *Repo) SelectVersion(versions []repointerface.Version) (selected repointerface.Version, err error) {
	if len(versions) > 0 {
		selected = versions[0]
	}
	return
}
func (r *Repo) GetEnvs(name string, version repointerface.Version) (envs []string, err error) {
	return
}
func (r *Repo) SelectEnv(envs []string, envSpec repointerface.EnvSpec) (selected string, err error) {
	if len(envs) > 0 {
		selected = envs[0]
	}
	return
}
func (r *Repo) FilterEnv(envs []string) (selected []string) {
	return envs
}
func (r *Repo) Fabricate(name string, version repointerface.Version, envs []string, dstDir string) (prefabPaths []string, blueprintPaths []string, fileType string, err error) {
	return
}
