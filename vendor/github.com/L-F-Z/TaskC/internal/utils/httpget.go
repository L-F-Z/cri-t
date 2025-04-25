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

package utils

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
)

func HttpGet(url string) (body []byte, statusCode int, err error) {
	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	statusCode = resp.StatusCode
	body, err = io.ReadAll(resp.Body)
	return
}

func HttpGetPanic(url string) string {
	resp, err := http.Get(url)
	if err != nil {
		log.Panicf("failed to request %s: %v\n", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Panicf("failed to request %s: Status Code %d\n", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Panicf("failed to read %s response: %v", url, err)
	}
	return string(body)
}

func ParseFileName(contentDisposition string) string {
	if contentDisposition == "" {
		return ""
	}
	re := regexp.MustCompile(`(?i)filename="([^"]+)"`)
	matches := re.FindStringSubmatch(contentDisposition)
	if len(matches) > 1 {
		return matches[1]
	}
	re = regexp.MustCompile(`(?i)filename\*=UTF-8''([^;]+)`)
	matches = re.FindStringSubmatch(contentDisposition)
	if len(matches) > 1 {
		decoded, err := url.QueryUnescape(matches[1])
		if err == nil {
			return decoded
		}
	}
	return ""
}
