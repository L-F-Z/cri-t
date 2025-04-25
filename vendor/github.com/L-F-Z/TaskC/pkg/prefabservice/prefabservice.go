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
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/apt"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/baserepo"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/dockerhub"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/huggingface"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/k8s"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/pypi"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

type PrefabService struct {
	infoStore       *InfoStore
	fileStore       *FileStore
	repos           map[string]repointerface.Repo
	upstream        string
	fabricatePrefab bool
	unpackPrefab    bool
	logging         bool
}

func NewPrefabService(workDir string, upstream string, fabricatePrefab bool, unpackPrefab bool, logging bool, ttl time.Duration) (ps *PrefabService, err error) {
	workDir = filepath.Join(workDir, "PrefabService")
	err = os.MkdirAll(workDir, 0700)
	if err != nil {
		return
	}

	ps = &PrefabService{
		repos: map[string]repointerface.Repo{
			repointerface.REPO_PYPI:        &pypi.Repo{},
			repointerface.REPO_APT:         &apt.Repo{},
			repointerface.REPO_DOCKERHUB:   &dockerhub.Repo{},
			repointerface.REPO_HUGGINGFACE: &huggingface.Repo{},
			repointerface.REPO_K8S:         &k8s.Repo{},
		},
		upstream:        strings.TrimSuffix(upstream, "/"),
		fabricatePrefab: fabricatePrefab,
		unpackPrefab:    unpackPrefab,
		logging:         logging,
	}
	ps.infoStore, err = NewInfoStore(workDir, ttl)
	if err != nil {
		return nil, fmt.Errorf("failed to init infoStore: [%v]", err)
	}
	ps.fileStore, err = NewFileStore(workDir)
	if err != nil {
		return nil, fmt.Errorf("failed to init fileStore: [%v]", err)
	}
	return
}

const NEVER_OUTDATE = time.Duration(math.MaxInt64)
const LONG_ENOUGH = time.Duration(1000000 * time.Hour)

func NewUserService(workDir string, upstream string) (ps *PrefabService, err error) {
	return NewPrefabService(workDir, upstream, false, true, false, NEVER_OUTDATE)
}

func NewProxyService(workDir string, upstream string) (ps *PrefabService, err error) {
	// Proxy Service [ MUST NOT ] use NEVER_OUTDATE as ttl.
	// If you want a stable environment for expriment, consider using LONG_ENGOUGH as ttl,
	// which is over 100 years!
	// We recommend to use 24*time.Duration(time.Hour).
	return NewPrefabService(workDir, upstream, false, false, true, time.Duration(time.Hour))
}

func NewServerService(workDir string) (ps *PrefabService, err error) {
	return NewPrefabService(workDir, "", true, false, true, NEVER_OUTDATE)
}

func (ps *PrefabService) PrefabSelection(specSheet repointerface.SpecSheet) (prefabID string, blueprintID string, err error) {
	if !ps.logging {
		originalOutput := log.Writer()
		log.SetOutput(io.Discard)
		defer log.SetOutput(originalOutput)
	}
	repo, ok := ps.repos[specSheet.Type]
	if !ok {
		repo = &baserepo.Repo{}
	}

	// Try to search on local InfoStore
	log.Println("\tTrying to search specSheet on local Prefab Service")
	// no need to create a dstDir for prefabservice search
	versions, _ := ps.infoStore.GetVersions(specSheet.Type, specSheet.Name)
	log.Printf("\tGot versions %v\n", versions)
	var vers []repointerface.Version
	for _, version := range versions {
		ver, err := ParseAnyVersion(specSheet.Type, version)
		if err != nil {
			log.Printf("\tFailed to parse version %s. ignore: [%v]", version, err)
		}
		vers = append(vers, ver)
	}
	vers = specSheet.Specifier.FilterAndSort(vers)
	for {
		ver, err := repo.SelectVersion(vers)
		// DELETE
		log.Printf("\tSelecting Versions from %+v, Selected %+v", vers, ver)
		if err != nil {
			return "", "", fmt.Errorf("failed to select version: [%v]", err)
		}
		if ver == nil {
			log.Printf("\t\t[Info] No suitable version for %s\n", specSheet.Name)
			break
		}
		envs, _ := ps.infoStore.GetEnvironments(specSheet.Type, specSheet.Name, ver.String())
		env, err := repo.SelectEnv(envs, specSheet.EnvSpec)
		if err != nil {
			return "", "", fmt.Errorf("failed to select env: [%v]", err)
		}
		// DELETE
		log.Printf("\tSelecting Envs from %+v, Selected %+v", envs, env)
		if env == "" {
			vers = slices.DeleteFunc(vers, func(s repointerface.Version) bool { return s.Compare(ver) == 0 })
			continue
		}
		prefabID, blueprintID = ps.infoStore.GetItem(specSheet.Type, specSheet.Name, ver.String(), env)
		if prefabID == "" || blueprintID == "" {
			return "", "", fmt.Errorf("found item, but no ID")
		} else {
			return prefabID, blueprintID, nil
		}
	}

	// then search on upstream Prefab Service
	if ps.upstream != "" {
		prefabID, blueprintID, err = ps.PostUpstreamSpecSheet(specSheet)
		if err != nil {
			return
		}
		if prefabID != "" && blueprintID != "" {
			return
		}
	}

	if !ps.fabricatePrefab {
		return
	}
	// no match in any Prefab Service, fabricate through other repositories
	log.Println("\tTrying to fabricate through ", specSheet.Type)
	dstDir, err := os.MkdirTemp("", "PrefabService")
	if err != nil {
		return
	}
	defer os.RemoveAll(dstDir)
	prefabPaths, blueprintPaths, fileType, err := processSpec(repo, specSheet, dstDir)
	if err != nil {
		return
	}
	if len(prefabPaths) == 0 || len(blueprintPaths) == 0 {
		return
	}
	log.Printf("\tSuccessfully fabricated %s\n", specSheet.Name)
	for i := range len(prefabPaths) {
		// Upload Fabricated prefab
		prefabID, blueprintID, err = ps.HandlePostUpload(specSheet.Type, prefabPaths[i], blueprintPaths[i], fileType)
		if err != nil {
			return
		}
	}
	log.Printf("\tSuccessfully uploaded to Prefab Service %s\n", specSheet.Name)
	return
}

func processSpec(repo repointerface.Repo, specSheet repointerface.SpecSheet, dstDir string) (prefabPaths []string, blueprintPaths []string, fileType string, err error) {
	// if Version and Environment is already given, we can directly fabricate the specSheet
	if specSheet.Version != nil && specSheet.Env != "" {
		log.Printf("\t\tAlready given version and environment, directly fabricating [%s] %s\n", specSheet.Version, specSheet.Env)
		return repo.Fabricate(specSheet.Name, specSheet.Version, []string{specSheet.Env}, dstDir)
	}

	// choose appropriate prefab version
	log.Printf("\t\tGetting versions for %s\n", specSheet.Name)
	vers, err := repo.GetVersions(specSheet.Name)
	if err != nil {
		log.Printf("\t\t[Fatal] Unable to get versions for %s\n", specSheet.Name)
		return
	}
	log.Printf("\t\t[Success] Got versions %s\n", sliceDigest(vers))
	vers = specSheet.Specifier.FilterAndSort(vers)
	log.Printf("\t\t[Success] Filetered versions %s\n", sliceDigest(vers))
	for {
		var ver repointerface.Version
		ver, err = repo.SelectVersion(vers)
		if err != nil {
			err = fmt.Errorf("failed to select version: [%v]", err)
			return
		}
		if ver == nil {
			err = fmt.Errorf("no matching version and environment")
			return
		}
		log.Printf("\t\t[Success] Selected version %s\n", ver)

		// choose appropriate prefab environment
		var envs []string
		log.Printf("\t\tGetting environments for version %s\n", ver)
		envs, err = repo.GetEnvs(specSheet.Name, ver)
		if err != nil {
			err = fmt.Errorf("failed to get environments for version %s", ver)
			return
		}
		log.Printf("\t\t[Success] Got environments %s\n", sliceDigestString(envs))

		var env string
		env, err = repo.SelectEnv(envs, specSheet.EnvSpec)
		if err != nil {
			err = fmt.Errorf("failed to select environment for version %s", ver)
			return
		}
		if env == "" {
			log.Printf("\t\t[Info] No suitable environment found for version %s, trying next version...\n", ver)
			vers = slices.DeleteFunc(vers, func(s repointerface.Version) bool { return s.Compare(ver) == 0 })
			continue
		}
		log.Printf("\t\t[Success] Selected environment %s", env)
		return repo.Fabricate(specSheet.Name, ver, []string{env}, dstDir)
	}
}

func (ps *PrefabService) getBlueprintFile(id string) (blueprintPath string, err error) {
	blueprintPath, _, _, err = ps._getFile(id, "", true)
	return
}

func (ps *PrefabService) getPrefabUnpack(id string, targetDir string) (prefabPath string, err error) {
	prefabPath, _, _, err = ps._getFile(id, targetDir, false)
	return
}

func (ps *PrefabService) provideFile(id string) (path string, fileName string, fileType string, err error) {
	return ps._getFile(id, "", true)
}

// When targetDir is empty string, the fetched file is not unpacked
// When targetDir is "/" or other paths, the fetched file is unpacked
func (ps *PrefabService) _getFile(id string, targetDir string, waitFinish bool) (path string, fileName string, fileType string, err error) {
	path = ps.fileStore.genPath(id)
	if utils.PathExists(path) {
		fileType, ok := ps.fileStore.files[id]
		if !ok {
			err = fmt.Errorf("failed to get file type of %s", id)
		}
		return path, fileType.FileName, fileType.FileType, err
	}
	upstreamFile, fileName, fileType, err := ps.GetUpstreamFile(id)
	if err != nil {
		err = fmt.Errorf("failed to request upstream file: [%v]", err)
		return
	}

	fileInfo := FileInfo{
		FileName: fileName,
		FileType: fileType,
	}

	unpack := false
	targetPath := path
	if ps.unpackPrefab && targetDir != "" {
		if fileType == repointerface.FILETYPE_COMPRESS {
			targetPath = filepath.Join(targetPath, targetDir)
			unpack = true
		} else { // FILETYPE_RAW
			targetPath = filepath.Join(targetPath, targetDir, fileName)
		}
	}
	ps.fileStore.AddFile(upstreamFile, targetPath, id, fileInfo, unpack, waitFinish)
	return
}

func sliceDigest(s []repointerface.Version) string {
	if len(s) < 6 {
		return fmt.Sprint(s)
	}
	p := "["
	for i := range 5 {
		p += s[i].String() + ", "
	}
	p += "...(length=" + strconv.Itoa(len(s)) + ")]"
	return p
}

func sliceDigestString(s []string) string {
	if len(s) < 6 {
		return fmt.Sprint(s)
	}
	p := "["
	for i := range 5 {
		p += s[i] + ", "
	}
	p += "...(length=" + strconv.Itoa(len(s)) + ")]"
	return p
}
