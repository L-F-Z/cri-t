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
	"fmt"
	"net/url"
	"strings"
)

func CombineURL(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	firstPart := strings.TrimRight(parts[0], "/")
	var trimmedParts []string
	for _, part := range parts[1:] {
		trimmed := strings.Trim(part, "/")
		trimmedParts = append(trimmedParts, trimmed)
	}
	return firstPart + "/" + strings.Join(trimmedParts, "/")
}

func URLtoFilename(url string) (filename string) {
	filename = strings.TrimSuffix(url, "/")
	filename = strings.TrimPrefix(filename, "https://")
	filename = strings.TrimPrefix(filename, "http://")
	filename = strings.ReplaceAll(filename, "/", "#")
	return
}

func IsURL(str string) bool {
	u, err := url.Parse(str)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func UnescapeURL(encodedURL string) (string, error) {
	encodedURL = strings.ReplaceAll(encodedURL, "%2B", "+")
	parsedURL, err := url.Parse(encodedURL)
	if err != nil {
		return "", fmt.Errorf("unable to decode URL: %v", err)
	}

	unescapedPath, err := url.PathUnescape(parsedURL.Path)
	if err != nil {
		return "", fmt.Errorf("unable to decode URL path: %v", err)
	}
	parsedURL.Path = unescapedPath

	unescapedQuery, err := url.QueryUnescape(parsedURL.RawQuery)
	if err != nil {
		return "", fmt.Errorf("unable to decode URL query: %v", err)
	}
	parsedURL.RawQuery = unescapedQuery

	unescapedFragment, err := url.QueryUnescape(parsedURL.Fragment)
	if err != nil {
		return "", fmt.Errorf("unable to decode URL fragment: %v", err)
	}
	parsedURL.Fragment = unescapedFragment

	unescapedURL := parsedURL.Scheme + "://" + parsedURL.Host + unescapedPath
	if unescapedQuery != "" {
		unescapedURL += "?" + unescapedQuery
	}
	if unescapedFragment != "" {
		unescapedURL += "#" + unescapedFragment
	}
	return unescapedURL, nil
}
