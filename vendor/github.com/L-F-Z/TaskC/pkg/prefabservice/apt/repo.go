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
	"fmt"
	"os"
	"path/filepath"

	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/dcontext"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

// Using https://salsa.debian.org/snapshot-team/snapshot/raw/master/API

type Repo struct {
	arch string
}

func NameNormalizer(name string) (normalized string) {
	return name
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

func (r *Repo) GetEnvSpec() repointerface.EnvSpec {
	return EnvSpec{Arch: r.arch}
}

func (r *Repo) GetVersions(name string) (versions []repointerface.Version, err error) {
	rawVersions, err := debianGetVersions(name)
	if err != nil {
		return
	}
	for _, raw := range rawVersions {
		ver, er := ParseVersion(raw)
		if er != nil {
			fmt.Printf("failed to parse version %s: [%v], ignore", raw, err)
		}
		versions = append(versions, ver)
	}
	return
}

func (r *Repo) GetEnvs(name string, version repointerface.Version) (envs []string, err error) {
	return debianGetEnvs(name, version)
}

func (r *Repo) Fabricate(name string, version repointerface.Version, envs []string, dstDir string) (prefabPaths []string, blueprintPaths []string, fileType string, err error) {
	fileType = repointerface.FILETYPE_COMPRESS
	for _, env := range envs {
		var debUrl, prefabPath, blueprintPath string
		debUrl, err = debianGetPackage(name, version, env)
		if err != nil {
			return
		}
		if debUrl == "virtual" {
			prefabPath, blueprintPath, err = FabricateVirtual(name, dstDir)
			return []string{prefabPath}, []string{blueprintPath}, repointerface.FILETYPE_COMPRESS, nil
		}

		var tmpDownloadDir string
		tmpDownloadDir, err = os.MkdirTemp("", repointerface.REPO_APT)
		if err != nil {
			return
		}
		defer os.RemoveAll(tmpDownloadDir)
		filename := "tmp.deb"
		_, err = utils.Download(debUrl, tmpDownloadDir, filename)
		if err != nil {
			return
		}

		prefabPath, blueprintPath, err = Fabricate(filepath.Join(tmpDownloadDir, filename), dstDir)
		if err != nil {
			return
		}
		prefabPaths = append(prefabPaths, prefabPath)
		blueprintPaths = append(blueprintPaths, blueprintPath)
	}
	return
}
