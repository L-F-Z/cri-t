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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ItemInfo struct {
	PrefabID    string `json:"prefab"`
	BlueprintID string `json:"blueprint"`
}
type VersionInfo struct {
	Environments map[string]*ItemInfo `json:"environments"`
	UpdateTime   time.Time            `json:"update"`
}
type NameInfo struct {
	Versions   map[string]*VersionInfo `json:"versions"`
	UpdateTime time.Time               `json:"update"`
}
type RepoInfo struct {
	Names      map[string]*NameInfo `json:"names"`
	UpdateTime time.Time            `json:"update"`
}
type InfoStore struct {
	Repos    map[string]*RepoInfo `json:"repos"`
	ttl      time.Duration
	savePath string
	sync.RWMutex
}

func NewInfoStore(workDir string, ttl time.Duration) (infoStore *InfoStore, err error) {
	infoStore = &InfoStore{
		Repos:    make(map[string]*RepoInfo),
		ttl:      ttl,
		savePath: filepath.Join(workDir, "Info.json"),
	}
	_, err = os.Stat(infoStore.savePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return infoStore, nil
		}
		return nil, fmt.Errorf("failed to stat info file: %w", err)
	}
	data, err := os.ReadFile(infoStore.savePath)
	if err != nil {
		return infoStore, fmt.Errorf("unable to read saved info store data: [%v]", err)
	}
	err = json.Unmarshal(data, &infoStore.Repos)
	if err != nil {
		return infoStore, fmt.Errorf("unable to unmarshal saved info store data: [%v]", err)
	}
	return
}

func (i *InfoStore) saveData() (err error) {
	data, err := json.MarshalIndent(i.Repos, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal info store data: [%v]", err)
	}
	err = os.WriteFile(i.savePath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write info store data file: [%v]", err)
	}
	return
}

func (i *InfoStore) GetNames(repo string) (names []string, outdated bool) {
	outdated = i.ttl != NEVER_OUTDATE
	if repo == "" {
		return
	}
	i.RLock()
	defer i.RUnlock()
	repoInfo, exists := i.Repos[repo]
	if !exists {
		return
	}
	outdated = time.Since(repoInfo.UpdateTime) > i.ttl
	names = make([]string, 0, len(repoInfo.Names))
	for name := range repoInfo.Names {
		names = append(names, name)
	}
	return
}

func (i *InfoStore) SetNames(repo string, names []string) (err error) {
	if repo == "" {
		return fmt.Errorf("repo is empty string")
	}
	i.Lock()
	defer i.Unlock()
	repoInfo, exists := i.Repos[repo]
	if !exists {
		repoInfo = &RepoInfo{Names: make(map[string]*NameInfo)}
		i.Repos[repo] = repoInfo
	}
	repoInfo.UpdateTime = time.Now()
	for _, name := range names {
		_, exists := repoInfo.Names[name]
		if !exists {
			repoInfo.Names[name] = &NameInfo{Versions: make(map[string]*VersionInfo)}
		}
	}
	return i.saveData()
}

func (i *InfoStore) GetVersions(repo string, name string) (versions []string, outdated bool) {
	outdated = i.ttl != NEVER_OUTDATE
	if repo == "" || name == "" {
		return
	}
	i.RLock()
	defer i.RUnlock()
	repoInfo, exists := i.Repos[repo]
	if !exists {
		return
	}
	nameInfo, exists := repoInfo.Names[name]
	if !exists {
		return
	}
	outdated = time.Since(nameInfo.UpdateTime) > i.ttl
	versions = make([]string, 0, len(nameInfo.Versions))
	for ver := range nameInfo.Versions {
		versions = append(versions, ver)
	}
	return
}

func (i *InfoStore) SetVersions(repo string, name string, versions []string) (err error) {
	if repo == "" || name == "" {
		return fmt.Errorf("repo or name is empty string")
	}
	i.Lock()
	defer i.Unlock()
	repoInfo, exists := i.Repos[repo]
	if !exists {
		repoInfo = &RepoInfo{Names: make(map[string]*NameInfo)}
		i.Repos[repo] = repoInfo
	}
	nameInfo, exists := repoInfo.Names[name]
	if !exists {
		nameInfo = &NameInfo{Versions: make(map[string]*VersionInfo)}
		repoInfo.Names[name] = nameInfo
	}
	nameInfo.UpdateTime = time.Now()
	for _, version := range versions {
		_, exists := nameInfo.Versions[version]
		if !exists {
			nameInfo.Versions[version] = &VersionInfo{Environments: make(map[string]*ItemInfo)}
		}
	}
	return i.saveData()
}

func (i *InfoStore) GetEnvironments(repo string, name string, version string) (environments []string, outdated bool) {
	outdated = i.ttl != NEVER_OUTDATE
	if repo == "" || name == "" || version == "" {
		return
	}
	i.RLock()
	defer i.RUnlock()
	repoInfo, exists := i.Repos[repo]
	if !exists {
		return
	}
	nameInfo, exists := repoInfo.Names[name]
	if !exists {
		return
	}
	versionInfo, exists := nameInfo.Versions[version]
	if !exists {
		return
	}
	outdated = time.Since(versionInfo.UpdateTime) > i.ttl
	environments = make([]string, 0, len(versionInfo.Environments))
	for env := range versionInfo.Environments {
		environments = append(environments, env)
	}
	return
}

func (i *InfoStore) SetEnvironments(repo string, name string, version string, environments []string) (err error) {
	if repo == "" || name == "" || version == "" {
		return fmt.Errorf("repo or name or version is empty string")
	}
	i.Lock()
	defer i.Unlock()
	repoInfo, exists := i.Repos[repo]
	if !exists {
		repoInfo = &RepoInfo{Names: make(map[string]*NameInfo)}
	}
	nameInfo, exists := repoInfo.Names[name]
	if !exists {
		nameInfo = &NameInfo{Versions: make(map[string]*VersionInfo)}
	}
	versionInfo, exists := nameInfo.Versions[version]
	if !exists {
		versionInfo = &VersionInfo{Environments: make(map[string]*ItemInfo)}
	}
	versionInfo.UpdateTime = time.Now()
	for _, environment := range environments {
		_, exists := versionInfo.Environments[environment]
		if !exists {
			versionInfo.Environments[environment] = &ItemInfo{}
		}
	}
	nameInfo.Versions[version] = versionInfo
	repoInfo.Names[name] = nameInfo
	i.Repos[repo] = repoInfo
	err = i.saveData()
	return
}

func (i *InfoStore) GetItem(repo string, name string, version string, environment string) (prefabID, blueprintID string) {
	if repo == "" || name == "" || version == "" || environment == "" {
		return
	}
	i.RLock()
	defer i.RUnlock()
	repoInfo, exists := i.Repos[repo]
	if !exists {
		return
	}
	nameInfo, exists := repoInfo.Names[name]
	if !exists {
		return
	}
	versionInfo, exists := nameInfo.Versions[version]
	if !exists {
		return
	}
	itemInfo, exists := versionInfo.Environments[environment]
	if !exists {
		return
	}
	return itemInfo.PrefabID, itemInfo.BlueprintID
}

func (i *InfoStore) SetItem(repo string, name string, version string, environment string, prefabID string, blueprintID string) (err error) {
	if repo == "" || name == "" || version == "" || environment == "" {
		return errors.New("repo or name or version or environment is empty string")
	}
	i.Lock()
	defer i.Unlock()
	repoInfo, exists := i.Repos[repo]
	if !exists {
		repoInfo = &RepoInfo{Names: make(map[string]*NameInfo)}
		i.Repos[repo] = repoInfo
	}
	nameInfo, exists := repoInfo.Names[name]
	if !exists {
		nameInfo = &NameInfo{Versions: make(map[string]*VersionInfo)}
		repoInfo.Names[name] = nameInfo
	}
	versionInfo, exists := nameInfo.Versions[version]
	if !exists {
		versionInfo = &VersionInfo{Environments: make(map[string]*ItemInfo)}
		nameInfo.Versions[version] = versionInfo
	}
	itemInfo, exists := versionInfo.Environments[environment]
	if exists {
		if itemInfo.PrefabID != "" && itemInfo.PrefabID != prefabID {
			return errors.New("existing item prefab ID mismatch")
		}
		if itemInfo.BlueprintID != "" && itemInfo.BlueprintID != blueprintID {
			return errors.New("existing item blueprint ID mismatch")
		}
	}
	versionInfo.Environments[environment] = &ItemInfo{
		PrefabID:    prefabID,
		BlueprintID: blueprintID,
	}
	return i.saveData()
}

func (i *InfoStore) DeleteItem(repo string, name string, version string, environment string) (err error) {
	if repo == "" || name == "" || version == "" || environment == "" {
		return fmt.Errorf("repo [%s] or name [%s] or version [%s] or environment [%s] is empty string", repo, name, version, environment)
	}
	i.Lock()
	defer i.Unlock()
	repoInfo, exists := i.Repos[repo]
	if !exists {
		return
	}
	nameInfo, exists := repoInfo.Names[name]
	if !exists {
		return
	}
	versionInfo, exists := nameInfo.Versions[version]
	if !exists {
		return
	}
	delete(versionInfo.Environments, environment)
	if len(versionInfo.Environments) == 0 {
		delete(nameInfo.Versions, version)
		if len(nameInfo.Versions) == 0 {
			delete(repoInfo.Names, name)
			if len(repoInfo.Names) == 0 {
				delete(i.Repos, repo)
			}
		}
	}
	err = i.saveData()
	return
}
