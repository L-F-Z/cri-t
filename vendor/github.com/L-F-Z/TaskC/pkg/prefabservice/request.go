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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/L-F-Z/TaskC/internal/utils"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
)

func (ps *PrefabService) GetUpstreamNames(repository string) (names []string, err error) {
	params := url.Values{}
	params.Add("repo", repository)
	fullURL := fmt.Sprintf("%s/names?%s", ps.upstream, params.Encode())

	resp, err := http.Get(fullURL)
	if err != nil {
		err = fmt.Errorf("failed to post get versions request: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("failure in response: [%s]", string(body))
		return
	}

	var result struct {
		Names []string `json:"names"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		err = fmt.Errorf("unable to unmarshal response: [%v]", err)
		return
	}
	names = result.Names
	return
}

func (ps *PrefabService) GetUpstreamVersions(repository string, name string) (versions []string, err error) {
	params := url.Values{}
	params.Add("repo", repository)
	params.Add("name", name)
	fullURL := fmt.Sprintf("%s/versions?%s", ps.upstream, params.Encode())

	resp, err := http.Get(fullURL)
	if err != nil {
		err = fmt.Errorf("failed to post get versions request: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("failure in response: [%s]", string(body))
		return
	}

	var result struct {
		Versions []string `json:"versions"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		err = fmt.Errorf("unable to unmarshal response: [%v]", err)
		return
	}
	versions = result.Versions
	return
}

func (ps *PrefabService) GetUpstreamEnvironemnts(repository string, name string, version string) (environments []string, err error) {
	params := url.Values{}
	params.Add("repo", repository)
	params.Add("name", name)
	params.Add("version", version)
	fullURL := fmt.Sprintf("%s/environments?%s", ps.upstream, params.Encode())

	resp, err := http.Get(fullURL)
	if err != nil {
		err = fmt.Errorf("failed to post get versions request: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("failure in response: [%s]", string(body))
		return
	}

	var result struct {
		Environments []string `json:"environments"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		err = fmt.Errorf("unable to unmarshal response: [%v]", err)
		return
	}
	environments = result.Environments
	return
}

func (ps *PrefabService) GetUpstreamItem(repository string, name string, version string, environment string) (prefabID string, blueprintID string, err error) {
	params := url.Values{}
	params.Add("repo", repository)
	params.Add("name", name)
	params.Add("version", version)
	params.Add("environment", environment)
	fullURL := fmt.Sprintf("%s/item?%s", ps.upstream, params.Encode())

	resp, err := http.Get(fullURL)
	if err != nil {
		err = fmt.Errorf("failed to post get versions request: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("failure in response: [%s]", string(body))
		return
	}

	var result struct {
		PrefabID    string `json:"prefab-id"`
		BlueprintID string `json:"blueprint-id"`
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		err = fmt.Errorf("unable to unmarshal response: [%v]", err)
		return
	}
	prefabID = result.PrefabID
	blueprintID = result.BlueprintID
	return
}

func (ps *PrefabService) GetUpstreamFile(id string) (file io.ReadCloser, fileName string, fileType string, err error) {
	params := url.Values{}
	params.Add("id", id)
	fullURL := fmt.Sprintf("%s/file?%s", ps.upstream, params.Encode())

	resp, err := http.Get(fullURL)
	if err != nil {
		err = fmt.Errorf("failed to post get file request: %v", err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		err = fmt.Errorf("failure in response: [%s]", string(body))
		return
	}
	fileType = resp.Header.Get("Content-Type")
	contentDisposition := resp.Header.Get("Content-Disposition")
	fileName = utils.ParseFileName(contentDisposition)
	file = resp.Body
	return
}

func (ps *PrefabService) PostUpstreamSpecSheet(specSheet repointerface.SpecSheet) (prefabID string, blueprintID string, err error) {
	return PostSpecSheet(ps.upstream, specSheet)
}

func PostSpecSheet(baseURL string, specSheet repointerface.SpecSheet) (prefabID string, blueprintID string, err error) {
	body, err := specSheet.Encode()
	if err != nil {
		err = fmt.Errorf("failed to marshal specSheet: [%v]", err)
		return
	}

	url := strings.TrimSuffix(baseURL, "/") + "/specsheet"
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		err = fmt.Errorf("failed to send request: [%v]", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("failed response: [%s]", string(body))
		return
	}

	var response struct {
		PrefabID    string `json:"prefab-id"`
		BlueprintID string `json:"blueprint-id"`
	}
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		err = fmt.Errorf("failed to decode response body: [%v]", err)
		return
	}
	return response.PrefabID, response.BlueprintID, nil
}

func (ps *PrefabService) PostUpload(repository string, prefabPath string, blueprintPath string) error {
	prefabFile, err := os.Open(prefabPath)
	if err != nil {
		return fmt.Errorf("unable to open prefab file %s: %v", prefabPath, err)
	}
	defer prefabFile.Close()

	blueprintFile, err := os.Open(blueprintPath)
	if err != nil {
		return fmt.Errorf("unable to open blueprint file %s: %v", blueprintPath, err)
	}
	defer blueprintFile.Close()

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		defer pw.Close()
		err := writer.WriteField("repository", repository)
		if err != nil {
			pw.CloseWithError(fmt.Errorf("failed to write field repository: %v", err))
			return
		}
		prefabHeader := textproto.MIMEHeader{}
		prefabHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="prefab"; filename="%s"`, filepath.Base(prefabPath)))
		prefabHeader.Set("Content-Type", repointerface.FILETYPE_COMPRESS)

		prefabPart, err := writer.CreatePart(prefabHeader)
		if err != nil {
			pw.CloseWithError(fmt.Errorf("failed to create part for prefab: %v", err))
			return
		}
		if _, err = io.Copy(prefabPart, prefabFile); err != nil {
			pw.CloseWithError(fmt.Errorf("failed to copy prefab file: %v", err))
			return
		}

		blueprintHeader := textproto.MIMEHeader{}
		blueprintHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="blueprint"; filename="%s"`, filepath.Base(blueprintPath)))
		blueprintHeader.Set("Content-Type", repointerface.FILETYPE_RAW)

		blueprintPart, err := writer.CreatePart(blueprintHeader)
		if err != nil {
			pw.CloseWithError(fmt.Errorf("failed to create part for blueprint: %v", err))
			return
		}
		if _, err = io.Copy(blueprintPart, blueprintFile); err != nil {
			pw.CloseWithError(fmt.Errorf("failed to copy blueprint file: %v", err))
			return
		}

		if err = writer.Close(); err != nil {
			pw.CloseWithError(fmt.Errorf("failed to close multipart writer: %v", err))
			return
		}
	}()

	url := strings.TrimSuffix(ps.upstream, "/") + "/upload"
	req, err := http.NewRequest("POST", url, pr)
	if err != nil {
		return fmt.Errorf("failed to create POST request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send POST request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed response: [%s]", string(respBody))
	}
	return nil
}
