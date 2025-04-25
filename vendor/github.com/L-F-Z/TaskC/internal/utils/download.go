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
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cavaliergopher/grab/v3"
)

func cleanupOnExit(tempFilePath string, stopChan chan struct{}) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-c:
			// fmt.Println("\nDetected exit signal, cleaning up...")
			if _, err := os.Stat(tempFilePath); !os.IsNotExist(err) {
				// fmt.Printf("Deleting incomplete download file: %s\n", tempFilePath)
				os.Remove(tempFilePath)
			}
			os.Exit(1)
		case <-stopChan:
			return
		}
	}()
}

func Download(rawurl string, directory string, filename string) (savedname string, err error) {
	retry := 2
	savedname, err = download(rawurl, directory, filename, false, nil)
	for err != nil && retry > 0 {
		// fmt.Println("error occured, retry: ", err)
		savedname, err = download(rawurl, directory, filename, false, nil)
		retry--
	}
	return
}

func DownloadWithHeader(rawurl string, directory string, filename string, header map[string]string) (savedname string, err error) {
	retry := 2
	savedname, err = download(rawurl, directory, filename, false, header)
	for err != nil && retry > 0 {
		// fmt.Println("error occured, retry: ", err)
		savedname, err = download(rawurl, directory, filename, false, header)
		retry--
	}
	return
}

func DownloadDisabledTLS(rawurl string, directory string, filename string) (savedname string, err error) {
	retry := 2
	savedname, err = download(rawurl, directory, filename, true, nil)
	for err != nil && retry > 0 {
		// fmt.Println("error occured, retry: ", err)
		savedname, err = download(rawurl, directory, filename, true, nil)
		retry--
	}
	return
}

// Download file from [url] to [directory]. If [filename] is emply, the name will be guessed from [url], and returned.
func download(rawurl string, directory string, filename string, disableTLS bool, header map[string]string) (savedname string, err error) {
	parsedURL, err := url.Parse(rawurl)
	if err != nil {
		return
	}
	// remove url fragments, e.g. #sha256=3vb23...
	parsedURL.Fragment = ""
	filePath := parsedURL.Path
	savedname = filename
	if savedname == "" {
		savedname = filepath.Base(filePath)
	}

	path := filepath.Join(directory, savedname)
	if PathExists(path) {
		return
	}

	tempPath := filepath.Join(directory, savedname+".tmp")
	stopChan := make(chan struct{})
	defer close(stopChan)
	cleanupOnExit(tempPath, stopChan)

	// create client
	client := grab.NewClient()
	if disableTLS {
		noTLSclient := &http.Client{
			Transport: &http.Transport{
				Proxy:           http.ProxyFromEnvironment,
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
		client.HTTPClient = noTLSclient
	}

	req, err := grab.NewRequest(tempPath, rawurl)
	if err != nil {
		return
	}
	// add header
	if len(header) > 0 {
		for key := range header {
			req.HTTPRequest.Header.Set(key, header[key])
		}
	}

	// start download
	// fmt.Printf("Downloading %v...\n", req.URL())
	resp := client.Do(req)
	// fmt.Printf("  %v\n", resp.HTTPResponse.Status)

	// start UI loop
	t := time.NewTicker(500 * time.Millisecond)
	skipUI := 6 // no UI in the first 3 seconds
	defer t.Stop()

Loop:
	for {
		select {
		case <-t.C:
			if skipUI <= 0 {
				fmt.Printf("\r  transferred %v / %v kB (%.2f%%)",
					resp.BytesComplete()/1024,
					resp.Size()/1024,
					100*resp.Progress())
			} else {
				skipUI--
			}

		case <-resp.Done:
			// download is complete
			// fmt.Println("\nDownload Complete")
			if resp.Err() == nil {
				err = os.Rename(tempPath, path)
				if err != nil {
					// fmt.Printf("Failed to rename temporary file: %v\n", err)
					return
				}
			} else {
				os.Remove(tempPath)
			}
			break Loop
		}
	}
	if resp.Err() != nil {
		return "", resp.Err()
	}
	return
}
