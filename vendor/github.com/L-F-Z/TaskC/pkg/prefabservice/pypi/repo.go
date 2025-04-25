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

package pypi

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/L-F-Z/TaskC/internal/cache"
	"github.com/L-F-Z/TaskC/internal/packing"
	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/dcontext"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
	mapset "github.com/deckarep/golang-set/v2"
)

type Repo struct {
	pyVer       string // e.g. "3.10"
	libcVer     string // e.g. "2.36"
	arch        string // e.g. "amd64"
	simpleCache *cache.Cache
}

type whlPackage struct {
	Name    string
	Version string
	Env     string
	Link    string
}

func NameNormalizer(name string) (normalized string) {
	replaced := strings.ReplaceAll(name, "-", "-")
	replaced = strings.ReplaceAll(replaced, "_", "-")
	replaced = strings.ReplaceAll(replaced, ".", "-")
	replaced = strings.ToLower(replaced)
	return replaced
}

func (r *Repo) GetEnvSpec() repointerface.EnvSpec {
	return EnvSpec{
		PyVer:   r.pyVer,
		LibcVer: r.libcVer,
		Arch:    r.arch,
	}
}

func getCache(simpleCache *cache.Cache, name string) ([]whlPackage, error) {
	if simpleCache == nil {
		simpleCache = cache.New(time.Hour, 20*time.Minute)
	}
	pureName, _ := getFeatures(name)
	cached, valid := simpleCache.Get(pureName)
	if valid {
		return cached.([]whlPackage), nil
	}
	candidates, err := getCandidates(pureName)
	if err != nil {
		return nil, err
	}
	simpleCache.Set(pureName, candidates)
	return candidates, nil
}

func (r *Repo) Init(ctx *dcontext.DeployContext) (err error) {
	value1, exists := ctx.Get(dcontext.ARCH_KEY)
	if !exists {
		return fmt.Errorf("unable to get hardware architecture from context: %v", err)
	}
	arch, ok := value1.(string)
	if !ok {
		return fmt.Errorf("context[hardware, architecture] is not a string")
	}
	r.arch = arch

	value2, exists := ctx.Get(dcontext.LIBC_VERSION)
	if !exists {
		return fmt.Errorf("unable to get libc version from context: %v", err)
	}
	libcVer, ok := value2.(string)
	if !ok {
		return fmt.Errorf("context[os, libcVersion] is not a string")
	}
	r.libcVer = libcVer

	value3, exists := ctx.Get(dcontext.PYTHON_VERSION_KEY)
	if !exists {
		return fmt.Errorf("unable to get python version from context: %v", err)
	}
	pyVer, ok := value3.(string)
	if !ok {
		return fmt.Errorf("context[os, pythonVersion] is not a string")
	}
	r.pyVer = pyVer

	return
}

func (r *Repo) GetVersions(name string) (versions []repointerface.Version, err error) {
	candidates, err := getCache(r.simpleCache, name)
	if err != nil {
		return
	}
	rawSet := mapset.NewSet[string]()
	for _, candidate := range candidates {
		rawSet.Add(candidate.Version)
	}
	raws := rawSet.ToSlice()
	for _, raw := range raws {
		ver, err := ParseVersion(raw)
		if err != nil {
			fmt.Printf("failed to parse version %s, ignore: [%v]", raw, err)
			continue
		}
		versions = append(versions, ver)
	}
	return
}

func (r *Repo) GetEnvs(name string, version repointerface.Version) (envs []string, err error) {
	candidates, err := getCache(r.simpleCache, name)
	if err != nil {
		return
	}
	envset := mapset.NewSet[string]()
	for _, candidate := range candidates {
		ver, err := ParseVersion(candidate.Version)
		if err != nil {
			fmt.Printf("failed to parse version %s, ignore: [%v]", candidate.Version, err)
		}
		if ver.Compare(version) == 0 {
			envset.Add(candidate.Env)
		}
	}
	return envset.ToSlice(), nil
}

func (r *Repo) Fabricate(name string, version repointerface.Version, envs []string, dstDir string) (prefabPaths []string, blueprintPaths []string, fileType string, err error) {
	fileType = repointerface.FILETYPE_COMPRESS
	var prefabPath, blueprintPath string
	ver, ok := version.(Version)
	if !ok {
		err = fmt.Errorf("given version is not a PyPI Version")
		return
	}

	if isTorchVirtual(name, ver) {
		prefabPath, blueprintPath, err = fabricateTorchVirtual(name, ver.String(), dstDir)
		return []string{prefabPath}, []string{blueprintPath}, repointerface.FILETYPE_COMPRESS, err
	}

	var tmpDownloadDir string
	tmpDownloadDir, err = os.MkdirTemp("", repointerface.REPO_PYPI)
	if err != nil {
		return
	}
	defer os.RemoveAll(tmpDownloadDir)
	var candidates []whlPackage
	candidates, err = getCache(r.simpleCache, name)
	if err != nil {
		return
	}

	var whlPaths, environments []string
	if len(envs) == 1 && isSourceDist(envs[0]) { // build from source
		var filename string
		for _, candidate := range candidates {
			ver, err = ParseVersion(candidate.Version)
			if err != nil {
				fmt.Printf("failed to parse version %s, ignore: [%v]", candidate.Version, err)
			}
			if ver.Compare(version) == 0 || isSourceDist(candidate.Env) {
				filename, err = utils.Download(candidate.Link, tmpDownloadDir, "")
				if err != nil {
					err = fmt.Errorf("error occured while downloading %s: %v", candidate.Link, err.Error())
					return
				}
				break
			}
		}
		if filename == "" {
			err = fmt.Errorf("no source distribution found")
		}
		whlPaths, environments, err = buildSource(filepath.Join(tmpDownloadDir, filename), tmpDownloadDir)
		if err != nil {
			err = fmt.Errorf("error occured while building python source package: %v", err)
			return
		}
	} else { // convert from pre-built wheels
		envs = slices.DeleteFunc(envs, isSourceDist)
		for _, candidate := range candidates {
			ver, err = ParseVersion(candidate.Version)
			if err != nil {
				fmt.Printf("failed to parse version %s, ignore: [%v]", candidate.Version, err)
			}
			if ver.Compare(version) != 0 || !slices.Contains(envs, candidate.Env) {
				continue
			}

			var filename string
			filename, err = utils.Download(candidate.Link, tmpDownloadDir, "")
			if err != nil {
				err = fmt.Errorf("error occured while downloading %s: %v", candidate.Link, err.Error())
				return
			}
			whlPaths = append(whlPaths, filepath.Join(tmpDownloadDir, filename))
			environments = append(environments, candidate.Env)
		}
	}

	for i := range len(whlPaths) {
		prefabPath, blueprintPath, err = Fabricate(whlPaths[i], name, version.String(), environments[i], dstDir)
		if err != nil {
			return
		}
		prefabPaths = append(prefabPaths, prefabPath)
		blueprintPaths = append(blueprintPaths, blueprintPath)
	}
	return
}

func buildSource(sourcePath string, dstDir string) (whlPaths []string, environments []string, err error) {
	workDir, err := os.MkdirTemp(dstDir, "SourceUnpack")
	if err != nil {
		err = fmt.Errorf("unable to create a directory for unpacking source code: [%v]", err)
		return
	}
	defer os.RemoveAll(workDir)
	err = packing.Unpack(sourcePath, workDir, "", "")
	if err != nil {
		err = fmt.Errorf("error occured when unpacking source code: [%v]", err)
		return
	}
	sourceDir, err := findSubdirectory(workDir)
	if err != nil {
		return
	}
	for _, pyVer := range []string{"3.12", "3.11", "3.10", "3.9", "3.8", "3.7", "3.6"} {
		pythonBin := "python" + pyVer
		var wheelDir string
		wheelDir, err = os.MkdirTemp(dstDir, "Wheel")
		if err != nil {
			err = fmt.Errorf("unable to create a directory for storing built wheel file: [%v]", err)
			return
		}
		defer os.RemoveAll(wheelDir)
		cmd := exec.Command(pythonBin, "-m", "build", "--wheel", "--outdir", wheelDir)
		cmd.Dir = sourceDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			err = fmt.Errorf("error occured when building source code: [%v]", err)
			return
		}
		wheelName := getWhlFilename(wheelDir)
		if wheelName == "" {
			err = errors.New("building wheel failed, unable to find a wheel file")
			return
		}
		pattern := `^([^\s-]+?)-([^\s-]*?)(-(\d[^-]*?))?-([^\s-]+?)-([^\s-]+?)-([^\s-]+?)\.whl$`
		pkg_regexp := regexp.MustCompile(pattern)
		match := pkg_regexp.FindStringSubmatch(wheelName)
		if match == nil {
			err = fmt.Errorf("building wheel failed, %s is not a valid wheel filename", wheelName)
			return
		}
		environment := match[5] + "-" + match[6] + "-" + match[7] // pyVers-ABIs-platforms
		srcPath := filepath.Join(wheelDir, wheelName)
		whlPath := filepath.Join(dstDir, wheelName)
		err = os.Rename(srcPath, whlPath)
		if err != nil {
			err = fmt.Errorf("error occured when moving built wheel to dstDir: [%v]", err)
			return
		}
		environments = append(environments, environment)
		whlPaths = append(whlPaths, whlPath)
		if strings.HasSuffix(environment, "none-any") {
			break
		}
	}
	return
}

func getWhlFilename(dir string) (whlName string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".whl") {
			return file.Name()
		}
	}
	return
}

func findSubdirectory(parentDir string) (subDirPath string, err error) {
	file, err := os.Open(parentDir)
	if err != nil {
		return
	}
	defer file.Close()

	contents, err := file.Readdir(-1)
	if err != nil {
		return
	}

	for _, entry := range contents {
		if entry.IsDir() {
			subDirPath = filepath.Join(parentDir, entry.Name())
			return
		}
	}

	err = fmt.Errorf("no subdirectory found in: %s", parentDir)
	return
}
