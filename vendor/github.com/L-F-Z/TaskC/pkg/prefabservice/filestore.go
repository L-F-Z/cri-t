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
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/L-F-Z/TaskC/internal/packing"
	"github.com/L-F-Z/TaskC/pkg/prefabservice/repointerface"
	"github.com/google/uuid"
)

type FileInfo struct {
	FileName string `json:"filename"`
	FileType string `json:"filetype"`
}

type FileStore struct {
	files          map[string]FileInfo
	savePath       string
	downloadStatus map[string]string // "" -> downloading, otherwise it stores the error message
	rootPath       string
	sync.RWMutex
}

const DOWNLOADING string = ""

func NewFileStore(workDir string) (fileStore *FileStore, err error) {
	fileStore = &FileStore{
		files:          make(map[string]FileInfo),
		savePath:       filepath.Join(workDir, "File.json"),
		downloadStatus: make(map[string]string),
		rootPath:       filepath.Join(workDir, "files"),
	}
	err = os.MkdirAll(fileStore.rootPath, os.ModePerm)
	if err != nil {
		err = fmt.Errorf("failed to create files directory: [%v]", err)
		return
	}
	_, err = os.Stat(fileStore.savePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fileStore, nil
		}
		return nil, fmt.Errorf("failed to stat info file: %w", err)
	}
	data, err := os.ReadFile(fileStore.savePath)
	if err != nil {
		return fileStore, fmt.Errorf("unable to read saved info store data: [%v]", err)
	}
	err = json.Unmarshal(data, &fileStore.files)
	if err != nil {
		return fileStore, fmt.Errorf("unable to unmarshal saved info store data: [%v]", err)
	}
	return
}

func (f *FileStore) saveData() (err error) {
	data, err := json.MarshalIndent(f.files, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal file store data: [%v]", err)
	}
	err = os.WriteFile(f.savePath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write info store data file: [%v]", err)
	}
	return
}

func (f *FileStore) genPath(id string) string {
	subDir := id[:2]
	return filepath.Join(f.rootPath, subDir, id)
}

func (f *FileStore) NewFile(path string, fileType string) (id string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	id = uuid.New().String()
	storePath := f.genPath(id)
	err = os.MkdirAll(filepath.Dir(storePath), os.ModePerm)
	if err != nil {
		err = errors.New("failed to create file subdirectory")
		return
	}
	dst, err := os.Create(storePath)
	if err != nil {
		return
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	if err != nil {
		err = errors.New("failed to copy file: " + err.Error())
	}

	var fileName string
	if fileType == repointerface.FILETYPE_RAW {
		fileName = filepath.Base(path)
	}

	f.Lock()
	f.files[id] = FileInfo{
		FileName: fileName,
		FileType: fileType,
	}
	f.saveData()
	f.Unlock()
	return
}

func (f *FileStore) AddFile(file io.ReadCloser, targetPath string, id string, fileInfo FileInfo, unpack bool, waitFinish bool) (err error) {
	tempDir, err := os.MkdirTemp("", "FileStore")
	if err != nil {
		return fmt.Errorf("failed to create temp download directory")
	}
	tempPath := filepath.Join(tempDir, id)

	f.Lock()
	f.downloadStatus[id] = DOWNLOADING
	f.Unlock()

	download := func() {
		defer func() {
			os.RemoveAll(tempDir)
			file.Close()
			f.Lock()
			if err != nil {
				f.downloadStatus[id] = err.Error()
			} else {
				delete(f.downloadStatus, id)
				f.files[id] = fileInfo
				f.saveData()
			}
			f.Unlock()
		}()

		if unpack {
			err = os.MkdirAll(tempPath, os.ModePerm)
			if err != nil {
				return
			}
			err = packing.UnTarGz(file, tempPath)
			if err != nil {
				return
			}
		} else {
			dst, err := os.Create(tempPath)
			if err != nil {
				return
			}
			_, err = io.Copy(dst, file)
			dst.Close()
			if err != nil {
				return
			}
		}
		err = os.MkdirAll(filepath.Dir(targetPath), os.ModePerm)
		if err != nil {
			return
		}
		err = os.Rename(tempPath, targetPath)
	}

	if waitFinish {
		download()
		// check download status
		status, ok := f.downloadStatus[id]
		if ok && status != DOWNLOADING {
			return errors.New(status)
		}
	} else {
		go download()
	}
	return
}

func (f *FileStore) DeleteFile(id string) (err error) {
	path := f.genPath(id)
	err = os.Remove(path)
	if err != nil {
		return errors.New("failed to delete file: " + err.Error())
	}
	f.Lock()
	delete(f.downloadStatus, id)
	f.Unlock()
	return
}

func (f *FileStore) WaitDownload(ids []string) (err error) {
	for len(ids) > 0 {
		wait := make([]string, 0)
		f.RLock()
		for _, id := range ids {
			status, ok := f.downloadStatus[id]
			if !ok {
				continue
			} else if status == DOWNLOADING {
				wait = append(wait, id)
			} else {
				log.Printf("\nError occurred when downloading %s: [%v]\n", id, status)
			}
		}
		f.RUnlock()
		ids = wait
		fmt.Printf("\rWaiting for %d Prefabs to download and unpack        ", len(ids))
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Printf("\r%-60s\r", "")
	return
}
