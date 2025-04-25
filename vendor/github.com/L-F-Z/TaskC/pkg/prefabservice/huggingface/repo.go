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

package huggingface

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/dcontext"
	"github.com/L-F-Z/TaskC/pkg/prefab"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

const BASEURL string = "https://huggingface.co/api/models/"

type EnvSpec struct {
}

func (es EnvSpec) Encode() string {
	return ""
}

func DecodeEnvSpec(s string) (es EnvSpec, err error) {
	return
}

func NameNormalizer(name string) (normalized string) {
	return name
}

type Version string

func ParseVersion(version string) (ver Version, err error) {
	return Version(version), nil
}

func (v Version) String() string {
	return string(v)
}

func (a Version) Compare(other repointerface.Version) (result int) {
	return strings.Compare(a.String(), other.String())
}

func DecodeSpecifier(specifier string) (c repointerface.Constraint, err error) {
	c.Raw = specifier
	c.RepoType = repointerface.REPO_HUGGINGFACE
	specifier = strings.TrimSpace(specifier)
	if specifier == "any" || specifier == "latest" {
		c.AddRange(nil, nil, false, false)
	} else {
		ver := Version(specifier)
		c.AddRange(ver, ver, true, true)
	}
	return
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

type sibling struct {
	RFilename string `json:"rfilename"`
}
type hfData struct {
	Siblings []sibling `json:"siblings"`
}

func (r *Repo) GetVersions(name string) (versions []repointerface.Version, err error) {
	fullURL := utils.CombineURL(BASEURL, name)
	body, status, err := utils.HttpGet(fullURL)
	if err != nil {
		err = fmt.Errorf("failed to request %s: [%v]", fullURL, err)
		return
	}
	if status != http.StatusOK {
		err = fmt.Errorf("get %d when requesting %s", status, fullURL)
		return
	}
	var data hfData
	err = json.Unmarshal(body, &data)
	if err != nil {
		err = fmt.Errorf("failed to unmarshal HuggingFace response: [%v]", err)
		return
	}
	for _, sib := range data.Siblings {
		versions = append(versions, Version(sib.RFilename))
	}
	return
}
func (r *Repo) SelectVersion(versions []repointerface.Version) (selected repointerface.Version, err error) {
	if len(versions) > 0 {
		selected = versions[0]
	}
	return
}
func (r *Repo) GetEnvs(name string, version repointerface.Version) (envs []string, err error) {
	vers, err := r.GetVersions(name)
	if err != nil {
		return
	}
	for _, ver := range vers {
		if ver.Compare(version) == 0 {
			return []string{"any"}, nil
		}
	}
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
	if !slices.Contains(envs, "any") {
		err = errors.New("no [any] in envs")
		return
	}
	vers, err := r.GetVersions(name)
	if err != nil {
		return
	}
	selected := ""
	for _, ver := range vers {
		if ver.Compare(version) == 0 {
			selected = ver.String()
		}
	}
	if selected == "" {
		err = fmt.Errorf("HunggingFace file %s not found", name+" "+version.String())
	}
	downloadURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", name, selected)
	savedName, err := utils.Download(downloadURL, dstDir, "")
	if err != nil {
		return
	}
	prefabPath := filepath.Join(dstDir, savedName)

	blueprint := prefab.NewBlueprint()
	blueprint.Type = repointerface.REPO_HUGGINGFACE
	blueprint.Name = name
	blueprint.Version = selected
	blueprint.Environment = "any"
	blueprintPath, err := blueprint.Save(dstDir)
	if err != nil {
		err = fmt.Errorf("failed to generate blueprint: [%v]", err)
		return
	}

	return []string{prefabPath}, []string{blueprintPath}, repointerface.FILETYPE_RAW, nil
}
