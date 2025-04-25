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
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

type EnvSpec struct {
	Arch string `json:"arch"`
}

func (es EnvSpec) Encode() string {
	return es.Arch
}

func DecodeEnvSpec(s string) (es EnvSpec, err error) {
	es.Arch = strings.TrimSpace(s)
	return
}

func (r *Repo) SelectVersion(versions []repointerface.Version) (selected repointerface.Version, err error) {
	if len(versions) == 0 {
		return
	}
	selected = versions[0]
	return
}

func (r *Repo) SelectEnv(envs []string, envSpec repointerface.EnvSpec) (selected string, err error) {
	candidates := make(map[string]*Env)
	for _, env := range envs {
		decoded, err := decodeEnv(env)
		if err != nil {
			fmt.Println("unable to decode env [" + env + "], ignored")
			continue
		}
		candidates[env] = decoded
	}

	// decode spec's architecture info
	spec, ok := envSpec.(EnvSpec)
	if !ok {
		return "", errors.New("mismatch envSpec type")
	}
	architecture, variant := "", ""
	if spec.Arch == "x86_64" {
		architecture = "amd64"
	} else if spec.Arch == "i686" || spec.Arch == "i386" {
		architecture = "386"
	} else if spec.Arch == "aarch64" || spec.Arch == "arm64" {
		architecture, variant = "arm64", "v8"
	} else if ok, vari := _decodeArm(spec.Arch); ok {
		architecture, variant = "arm", "v"+vari
	} else {
		architecture = spec.Arch
	}

	for str, cand := range candidates {
		if cand.Os != "" && cand.Os != "linux" {
			continue
		}
		if architecture != cand.Architecture {
			continue
		}
		if variant != "" && cand.Variant != "" && variant != cand.Variant {
			continue
		}
		selected = str
		return
	}
	return
}

func (r *Repo) FilterEnv(envs []string) (selected []string) {
	return envs
}

func _decodeArm(arch string) (bool, string) {
	re := regexp.MustCompile(`^arm(\d)`)
	matches := re.FindStringSubmatch(arch)
	if len(matches) > 1 {
		return true, string(matches[1])
	}
	return false, ""
}

type Env struct {
	Os           string `json:"os"`           // e.g. "linux"
	Architecture string `json:"architecture"` // e.g. "386", "amd64", "arm64", "arm", "mips64le", "ppc64le", "riscv64", "s390x"
	Variant      string `json:"variant"`      // e.g. ("arm"-)"v5", "v6", "v7" ("arm64"-) "v8"
}

func (p Env) String() string {
	s := p.Architecture
	if p.Variant != "" {
		s += "-" + p.Variant
	}
	if p.Os != "" {
		s += "+" + p.Os
	}
	return s
}

func decodeEnv(s string) (env *Env, err error) {
	env = &Env{}
	osSplit := strings.Split(s, "+")
	if len(osSplit) > 2 {
		err = errors.New("multiple '+' sign, should be at most 1")
		return
	}
	if len(osSplit) == 2 {
		env.Os = osSplit[1]
	}
	archSplit := strings.Split(osSplit[0], "-")
	if len(archSplit) > 2 {
		err = errors.New("multiple '-' sign, should be at most 1")
		return
	}
	if len(archSplit) == 2 {
		env.Variant = archSplit[1]
	}
	env.Architecture = archSplit[0]
	return
}
