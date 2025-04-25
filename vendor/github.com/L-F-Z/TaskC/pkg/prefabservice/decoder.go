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

package prefabservice

import (
	"encoding/json"
	"fmt"

	"github.com/L-F-Z/TaskC/pkg/prefabservice/apt"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/baserepo"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/dockerhub"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/huggingface"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/k8s"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/pypi"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

func DecodeSpecSheet(raw []byte) (spec repointerface.SpecSheet, err error) {
	var s repointerface.SpecSheetString
	err = s.Decode(raw)
	if err != nil {
		return
	}
	version, err := ParseAnyVersion(s.Type, s.Version)
	if err != nil {
		return
	}
	specifier, err := DecodeAnySpecifier(s.Type, s.Specifier)
	if err != nil {
		return
	}
	envSpec, err := DecodeAnyEnvSpec(s.Type, s.EnvSpec)
	if err != nil {
		return
	}
	spec = repointerface.SpecSheet{
		Type:      s.Type,
		Name:      NormalizeAnyName(s.Type, s.Name),
		Version:   version,
		Env:       s.Env,
		Specifier: specifier,
		EnvSpec:   envSpec,
	}
	return
}

func NormalizeAnyName(repoType string, name string) string {
	switch repoType {
	case repointerface.REPO_APT:
		return apt.NameNormalizer(name)
	case repointerface.REPO_PYPI:
		return pypi.NameNormalizer(name)
	case repointerface.REPO_DOCKERHUB:
		return dockerhub.NameNormalizer(name)
	case repointerface.REPO_HUGGINGFACE:
		return huggingface.NameNormalizer(name)
	case repointerface.REPO_K8S:
		return k8s.NameNormalizer(name)
	default:
		return name
	}
}

func ParseAnyVersion(repoType string, version string) (repointerface.Version, error) {
	if version == "" {
		return nil, nil
	}
	switch repoType {
	case repointerface.REPO_APT:
		return apt.ParseVersion(version)
	case repointerface.REPO_PYPI:
		return pypi.ParseVersion(version)
	case repointerface.REPO_DOCKERHUB:
		return dockerhub.ParseVersion(version)
	case repointerface.REPO_HUGGINGFACE:
		return huggingface.ParseVersion(version)
	case repointerface.REPO_K8S:
		return k8s.ParseVersion(version)
	default:
		return baserepo.Version(version), nil
	}
}

func DecodeAnySpecifier(repoType string, specifier string) (repointerface.Constraint, error) {
	// first try to UnMarshal
	var dec repointerface.ConstraintString
	err := json.Unmarshal([]byte(specifier), &dec)
	if err == nil {
		c := repointerface.Constraint{
			RepoType: dec.RepoType,
			Ranges:   make([]repointerface.VersionRange, len(dec.Ranges)),
			Raw:      dec.Raw,
		}
		for i, ver := range dec.Ranges {
			lower, err := ParseAnyVersion(c.RepoType, ver.LowerBound)
			if err != nil {
				return repointerface.Constraint{}, fmt.Errorf("failed to decode %s version %s: [%v]", c.RepoType, ver.LowerBound, err)
			}
			upper, err := ParseAnyVersion(c.RepoType, ver.UpperBound)
			if err != nil {
				return repointerface.Constraint{}, fmt.Errorf("failed to decode %s version %s: [%v]", c.RepoType, ver.UpperBound, err)
			}
			c.Ranges[i] = repointerface.VersionRange{
				LowerBound:     lower,
				UpperBound:     upper,
				LowerInclusive: ver.LowerInclusive,
				UpperInclusive: ver.UpperInclusive,
			}
		}
		return c, err
	}
	// Then try to use different decoder
	switch repoType {
	case repointerface.REPO_APT:
		return apt.DecodeSpecifier(specifier)
	case repointerface.REPO_PYPI:
		return pypi.DecodeSpecifier(specifier)
	case repointerface.REPO_DOCKERHUB:
		return dockerhub.DecodeSpecifier(specifier)
	case repointerface.REPO_HUGGINGFACE:
		return huggingface.DecodeSpecifier(specifier)
	case repointerface.REPO_K8S:
		return k8s.DecodeSpecifier(specifier)
	default:
		if specifier == "any" {
			return repointerface.AnyConstraint, nil
		} else {
			return repointerface.SingleVersionConstraint(baserepo.Version(specifier)), nil
		}
	}
}

func DecodeAnyEnvSpec(repoType string, envSpec string) (repointerface.EnvSpec, error) {
	switch repoType {
	case repointerface.REPO_APT:
		return apt.DecodeEnvSpec(envSpec)
	case repointerface.REPO_PYPI:
		return pypi.DecodeEnvSpec(envSpec)
	case repointerface.REPO_DOCKERHUB:
		return dockerhub.DecodeEnvSpec(envSpec)
	case repointerface.REPO_HUGGINGFACE:
		return huggingface.DecodeEnvSpec(envSpec)
	case repointerface.REPO_K8S:
		return k8s.DecodeEnvSpec(envSpec)
	}
	return nil, nil
}
