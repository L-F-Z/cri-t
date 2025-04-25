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

package dockerhub

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
	"github.com/klauspost/compress/zstd"
)

// Reference: https://distribution.github.io/distribution/spec/api/

func getToken(image string, serviceBase string) (string, error) {
	authUrl := utils.CombineURL(serviceBase, "v2") + "/"
	resp, err := http.Get(authUrl)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return "", nil
	}
	if resp.StatusCode != http.StatusUnauthorized {
		return "", fmt.Errorf("expected 401 Unauthorized, got %d", resp.StatusCode)
	}
	authHeader := resp.Header.Get("WWW-Authenticate")
	if authHeader == "" {
		return "", fmt.Errorf("no WWW-Authenticate header found")
	}
	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	var authBase, serviceName string
	for part := range bytes.SplitSeq([]byte(authHeader), []byte{','}) {
		p := string(bytes.TrimSpace(part))
		if strings.HasPrefix(p, "realm=") {
			authBase = strings.Trim(p[len("realm="):], `"`)
		} else if strings.HasPrefix(p, "service=") {
			serviceName = strings.Trim(p[len("service="):], `"`)
		}
	}
	if authBase == "" || serviceName == "" {
		return "", fmt.Errorf("failed to parse auth info from header: %s", authHeader)
	}
	url := fmt.Sprintf("%s?service=%s&scope=repository:%s:pull", authBase, serviceName, image)
	body, _, err := utils.HttpGet(url)
	if err != nil {
		return "", err
	}
	var tokenResponse struct {
		Token string `json:"token"`
	}
	err = json.Unmarshal(body, &tokenResponse)
	if err != nil {
		return "", err
	}
	return tokenResponse.Token, nil
}

func GetTags(name string, serviceBase string) (tags []string, err error) {
	token, err := getToken(name, serviceBase)
	if err != nil {
		err = fmt.Errorf("unable to get dockerhub token: %v", err)
		return
	}

	url := utils.CombineURL(serviceBase, "v2", name, "tags", "list")
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Add("Authorization", "Bearer "+token)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, err
	}
	return result.Tags, nil
}

func GetEnvs(name string, tag string, serviceBase string) (envs map[string]string, err error) {
	envs = make(map[string]string)
	token, err := getToken(name, serviceBase)
	if err != nil {
		return nil, fmt.Errorf("unable to get token: %v", err)
	}

	url := utils.CombineURL(serviceBase, "v2", name, "manifests", tag)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Add("Authorization", "Bearer "+token)
	}
	req.Header.Add("Accept", "application/vnd.oci.image.index.v1+json")
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.list.v2+json")
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	mediaType := resp.Header.Get("Content-Type")
	switch mediaType {
	case "application/vnd.docker.distribution.manifest.list.v2+json", "application/vnd.oci.image.index.v1+json":
		var list struct {
			Manifests []struct {
				Platform struct {
					OS           string `json:"os"`
					Architecture string `json:"architecture"`
					Variant      string `json:"variant,omitempty"`
				} `json:"platform"`
				Digest string `json:"digest"`
			} `json:"manifests"`
		}
		err = json.NewDecoder(resp.Body).Decode(&list)
		if err != nil {
			return
		}
		for _, m := range list.Manifests {
			if m.Platform.OS != "linux" {
				continue
			}
			arch := m.Platform.Architecture
			if m.Platform.Variant != "" {
				arch += "/" + m.Platform.Variant
			}
			envs[arch] = m.Digest
			// envs = append(envs, fmt.Sprintf("%s/%s", m.Platform.OS, arch))
		}
	case "application/vnd.docker.distribution.manifest.v2+json":
		digest := resp.Header.Get("Docker-Content-Digest")
		if digest == "" {
			err = fmt.Errorf("no Docker-Content-Digest header in response")
			return
		}
		var manifest struct {
			Config struct{ Digest string } `json:"config"`
		}
		err = json.NewDecoder(resp.Body).Decode(&manifest)
		if err != nil {
			return
		}
		// fetch config blob to read os/arch
		tmpDir, _ := os.MkdirTemp("", "")
		defer os.RemoveAll(tmpDir)
		err = fetchBlob(serviceBase, token, name, manifest.Config.Digest, tmpDir, "config.json")
		if err != nil {
			return nil, err
		}
		configBytes, _ := os.ReadFile(filepath.Join(tmpDir, "config.json"))

		var cfg struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
			Variant      string `json:"variant,omitempty"`
		}
		err = json.Unmarshal(configBytes, &cfg)
		if err != nil {
			return nil, err
		}
		if cfg.OS != "linux" {
			return
		}
		arch := cfg.Architecture
		if cfg.Variant != "" {
			arch += "/" + cfg.Variant
		}
		envs[arch] = digest
		return
	default:
		err = fmt.Errorf("unexpected manifest mediaType: %s", mediaType)
	}
	return
}

func GetImage(name string, digest string, rootFs string, serviceBase string) (config []byte, err error) {
	token, err := getToken(name, serviceBase)
	if err != nil {
		err = fmt.Errorf("unable to get dockerhub token: %v", err)
		return
	}

	manifest, err := getManifest(serviceBase, token, name, digest)
	if err != nil {
		err = fmt.Errorf("unable to get manifest: %v", err)
		return
	}

	tmpDownloadDir, err := os.MkdirTemp("", repointerface.REPO_DOCKERHUB)
	if err != nil {
		return
	}
	defer os.RemoveAll(tmpDownloadDir)
	for i, layer := range manifest.Layers {
		fmt.Printf("downloading layer %d/%d\n", i+1, len(manifest.Layers))
		layerName := layer.Digest + _extension(layer.MediaType)
		err = fetchBlob(serviceBase, token, name, layer.Digest, tmpDownloadDir, layerName)
		if err != nil {
			err = fmt.Errorf("unable to fetch blob: %v", err)
			return
		}
		err = unpackCompressedLayer(rootFs, filepath.Join(tmpDownloadDir, layerName))
		if err != nil {
			err = fmt.Errorf("unable to unpack layer: %v", err)
			return
		}
	}

	// get Image Config
	err = fetchBlob(serviceBase, token, name, manifest.Config.Digest, tmpDownloadDir, "config.json")
	if err != nil {
		err = fmt.Errorf("unable to fetch config blob: %v", err)
		return
	}
	config, err = os.ReadFile(filepath.Join(tmpDownloadDir, "config.json"))
	if err != nil {
		err = fmt.Errorf("unable to read config file: %v", err)
	}
	return
}

func unpackCompressedLayer(root string, layerPath string) (err error) {
	file, err := os.Open(layerPath)
	if err != nil {
		err = errors.New("unable to open file when unpacking " + layerPath + " error:" + err.Error())
		return
	}
	defer file.Close()

	compress := filepath.Ext(layerPath)
	var decompressed io.Reader
	switch compress {
	case ".gz":
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return err
		}
		defer gzReader.Close()
		decompressed = gzReader
	case ".zst":
		zstReader, err := zstd.NewReader(file)
		if err != nil {
			return err
		}
		defer zstReader.Close()
		decompressed = zstReader
	default:
		decompressed = file
	}
	return unpackLayer(root, decompressed)
}

type Manifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Config        Blob   `json:"config"`
	Layers        []Blob `json:"layers"`
}

type Blob struct {
	MediaType string `json:"mediaType"`
	Digest    string `json:"digest"`
	Size      int    `json:"size"`
}

func getManifest(serviceBase string, token string, image string, digest string) (result Manifest, err error) {
	url := utils.CombineURL(serviceBase, "v2", image, "manifests", digest)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}
	if token != "" {
		req.Header.Add("Authorization", "Bearer "+token)
	}
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v1+json")
	req.Header.Add("Accept", "application/vnd.oci.image.manifest.v1+json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return
	}
	if result.MediaType != "application/vnd.oci.image.manifest.v1+json" &&
		result.MediaType != "application/vnd.docker.distribution.manifest.v2+json" {
		return result, errors.New("Currently not support type " + result.MediaType)
	}
	return
}

func fetchBlob(serviceBase string, token string, image string, digest string, directory string, name string) (err error) {
	url := utils.CombineURL(serviceBase, "v2", image, "blobs", digest)
	header := make(map[string]string)
	if token != "" {
		header["Authorization"] = "Bearer " + token
	}
	_, err = utils.DownloadWithHeader(url, directory, name, header)
	return
}

func _extension(mediaType string) string {
	switch mediaType {
	case "application/vnd.oci.image.config.v1+json":
		return ".json"
	case "application/vnd.oci.image.layer.v1.tar":
		return ".tar"
	case "application/vnd.oci.image.layer.v1.tar+gzip":
		return ".tar.gz"
	case "application/vnd.oci.image.layer.v1.tar+zstd":
		return ".tar.zst"
	case "application/vnd.oci.empty.v1+json":
		return ".json"
	case "application/vnd.docker.container.image.v1+json":
		return ".json"
	case "application/vnd.docker.image.rootfs.diff.tar.gzip":
		return ".tar.gz"
	case "application/vnd.docker.image.rootfs.foreign.diff.tar.gzip":
		return ".tar.gz"
	case "application/vnd.docker.plugin.v1+json":
		return ".json"
	default:
		return ""
	}
}
