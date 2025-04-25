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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/prefab"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

const VIRTUAL_PKG_VERSION = "~"
const VIRTUAL_PKG_ENVIRONMENT = "all"

type version struct {
	Version string `json:"binary_version"`
}

type architecture struct {
	Architecture string `json:"architecture"`
	Hash         string `json:"hash"`
}

func debianGetVersions(name string) (versions []string, err error) {
	url := "https://snapshot.debian.org/mr/binary/" + name + "/"
	body, status, err := utils.HttpGet(url)
	if err != nil {
		return
	}
	if status != http.StatusOK {
		var virtual bool
		virtual, err = isVirtualPackage(name)
		if err != nil {
			return
		}
		if virtual {
			return []string{VIRTUAL_PKG_VERSION}, nil
		}
		err = fmt.Errorf("received non-200 response status: %d", status)
		return
	}
	var data struct {
		Result []version `json:"result"`
	}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return
	}
	for _, entry := range data.Result {
		versions = append(versions, entry.Version)
	}
	return
}

// get a raw version string that equals the input version
func debianGetRawVersion(name string, version repointerface.Version) (rawVer string, err error) {
	versions, err := debianGetVersions(name)
	if err != nil {
		return
	}
	for _, raw := range versions {
		ver, err := ParseVersion(raw)
		if err != nil {
			continue
		}
		if ver.Compare(version) == 0 {
			rawVer = raw
			break
		}
	}
	if rawVer == "" {
		err = fmt.Errorf("failed to find a version that equals %v", version)
	}
	return
}

func debianGetEnvs(name string, version repointerface.Version) (envs []string, err error) {
	rawVer, err := debianGetRawVersion(name, version)
	if err != nil {
		return
	}
	url := "https://snapshot.debian.org/mr/binary/" + name + "/" + rawVer + "/binfiles"
	body, status, err := utils.HttpGet(url)
	if err != nil {
		return
	}
	if status != http.StatusOK {
		var virtual bool
		virtual, err = isVirtualPackage(name)
		if err != nil {
			return
		}
		if virtual {
			return []string{VIRTUAL_PKG_ENVIRONMENT}, nil
		}
		err = fmt.Errorf("received non-200 response status: %d", status)
		return
	}
	var data struct {
		Result []architecture `json:"result"`
	}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return
	}
	for _, entry := range data.Result {
		envs = append(envs, entry.Architecture)
	}
	return
}

func debianGetPackage(name string, version repointerface.Version, env string) (pkgUrl string, err error) {
	rawVer, err := debianGetRawVersion(name, version)
	if err != nil {
		return
	}
	url := "https://snapshot.debian.org/mr/binary/" + name + "/" + rawVer + "/binfiles"
	body, status, err := utils.HttpGet(url)
	if err != nil {
		return
	}
	if status != http.StatusOK {
		var virtual bool
		virtual, err = isVirtualPackage(name)
		if err != nil {
			return
		}
		if virtual {
			return "virtual", nil
		}
		err = fmt.Errorf("received non-200 response status: %d", status)
		return
	}
	var data struct {
		Result []architecture `json:"result"`
	}
	err = json.Unmarshal(body, &data)
	if err != nil {
		return
	}

	var hash string
	for _, entry := range data.Result {
		if entry.Architecture == env {
			hash = entry.Hash
			break
		}
	}

	if hash == "" {
		err = errors.New("unable to find package " + name + " with required version and arch")
		return
	}

	pkgUrl = "https://snapshot.debian.org/file/" + hash
	return
}

// https://www.debian.org/doc/debian-policy/ch-binary.html#virtual-packages
// https://www.debian.org/doc/debian-policy/ch-relationships.html#virtual-packages-provides

func _getVirtualPackageInfo(name string) (virtual bool, list string, err error) {
	re := regexp.MustCompile(`(?s)<div id="pdeps">.*?</div>`)

	urlStable := "https://packages.debian.org/en/stable/" + name
	body, statusCode, err := utils.HttpGet(urlStable)
	if err != nil {
		err = errors.New("unable to request " + urlStable + " to check if it is a virtual package:" + err.Error())
		return
	}
	if statusCode == http.StatusOK && !strings.Contains(string(body), "<h1>Error</h1>") {
		if strings.Contains(string(body), "<em>virtual package</em>") {
			list = re.FindString(string(body))
			return true, list, nil
		} else {
			return false, "", nil
		}
	}

	// Try to search on unstable channel, e.g. perlapi-5.38.2
	urlUnstable := "https://packages.debian.org/en/unstable/" + name
	body, statusCode, err = utils.HttpGet(urlUnstable)
	if err != nil {
		err = errors.New("unable to request " + urlUnstable + " to check if it is a virtual package:" + err.Error())
		return
	}
	if statusCode == http.StatusOK && !strings.Contains(string(body), "<h1>Error</h1>") {
		if strings.Contains(string(body), "<em>virtual package</em>") {
			list = re.FindString(string(body))
			return true, list, nil
		} else {
			return false, "", nil
		}
	}

	// Try to search on experimental channel, e.g. qtbase-abi-5-15-16
	urlExperimental := "https://packages.debian.org/en/experimental/" + name
	body, statusCode, err = utils.HttpGet(urlExperimental)
	if err != nil {
		err = errors.New("unable to request " + urlExperimental + " to check if it is a virtual package:" + err.Error())
		return
	}
	if statusCode == http.StatusOK && !strings.Contains(string(body), "<h1>Error</h1>") {
		if strings.Contains(string(body), "<em>virtual package</em>") {
			list = re.FindString(string(body))
			return true, list, nil
		} else {
			return false, "", nil
		}
	}
	return false, "", errors.New("unable to find package info on packages.debian.org")
}

func isVirtualPackage(name string) (result bool, err error) {
	result, _, err = _getVirtualPackageInfo(name)
	return
}

func provideVirtualPackage(name string) (provide []*prefab.Prefab, err error) {
	virtual, list, err := _getVirtualPackageInfo(name)
	if err != nil {
		return
	}
	if !virtual {
		err = errors.New("package " + name + "is not a virtual package")
	}
	re := regexp.MustCompile(`<a[^>]*>(.*?)</a>`)
	matches := re.FindAllStringSubmatch(list, -1)
	for _, match := range matches {
		provide = append(provide, &prefab.Prefab{
			SpecType:  repointerface.REPO_APT,
			Name:      match[1],
			Specifier: "any",
		})
	}
	return
}
