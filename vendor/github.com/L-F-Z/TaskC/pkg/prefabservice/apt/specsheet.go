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

package apt

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

type EnvSpec struct {
	Arch string `json:"arch"` // e.g. "amd64"
}

func (es EnvSpec) Encode() string {
	return es.Arch
}

func DecodeEnvSpec(s string) (es EnvSpec, err error) {
	es.Arch = strings.TrimSpace(s)
	return
}

func (r *Repo) SelectEnv(envs []string, envSpec repointerface.EnvSpec) (selected string, err error) {
	spec, ok := envSpec.(EnvSpec)
	if !ok {
		return "", errors.New("mismatch envSpec type")
	}
	if slices.Contains(envs, "all") {
		selected = "all"
	}
	if slices.Contains(envs, spec.Arch) {
		selected = spec.Arch
	}
	return
}

func (r *Repo) FilterEnv(envs []string) (selected []string) {
	for _, env := range envs {
		if env == "all" || env == "amd64" || env == "arm64" || env == "i386" {
			selected = append(selected, env)
		}
	}
	return
}

func (r *Repo) SelectVersion(versions []repointerface.Version) (selected repointerface.Version, err error) {
	if len(versions) == 0 {
		return
	}
	vers := make([]Version, 0, len(versions))
	for _, version := range versions {
		if ver, ok := version.(Version); ok {
			vers = append(vers, ver)
		} else {
			return nil, fmt.Errorf("version %v has wrong type", version)
		}
	}
	selected = vers[0]
	return
}
