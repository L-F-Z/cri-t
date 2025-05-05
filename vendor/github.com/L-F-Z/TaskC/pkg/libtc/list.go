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

package libtc

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/runc/libcontainer/utils"
)

// containerState represents the platform agnostic pieces relating to a
// running container's status and state
type containerState struct {
	// Version is the OCI version for the container
	Version string `json:"ociVersion"`
	// ID is the container ID
	ID string `json:"id"`
	// InitProcessPid is the init process id in the parent namespace
	InitProcessPid int `json:"pid"`
	// Status is the current status of the container, running, paused, ...
	Status string `json:"status"`
	// Bundle is the path on the filesystem to the bundle
	Bundle string `json:"bundle"`
	// Rootfs is a path to a directory containing the container's root filesystem.
	Rootfs string `json:"rootfs"`
	// Created is the unix timestamp for the creation time of the container in UTC
	Created time.Time `json:"created"`
	// Annotations is the user defined annotations added to the config.
	Annotations map[string]string `json:"annotations,omitempty"`
	// The owner of the state directory (the owner of the container).
	Owner string `json:"owner"`
}

func ListContainers() ([]containerState, error) {
	list, err := os.ReadDir(CONTAINERS_ROOT)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Ignore non-existing default root directory
			// (no containers created yet).
			return nil, nil
		}
		// Report other errors, including non-existent custom --root.
		return nil, err
	}
	var s []containerState
	for _, item := range list {
		if !item.IsDir() {
			continue
		}
		st, err := item.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// Possible race with runc delete.
				continue
			}
			return nil, err
		}
		// This cast is safe on Linux.
		uid := st.Sys().(*syscall.Stat_t).Uid
		owner := "#" + strconv.Itoa(int(uid))
		u, err := user.LookupId(owner[1:])
		if err == nil {
			owner = u.Username
		}

		status, state, err := GetStatusState(item.Name())
		if err != nil {
			return nil, err
		}
		pid := state.BaseState.InitProcessPid
		if status == libcontainer.Stopped {
			pid = 0
		}
		bundle, annotations := utils.Annotations(state.Config.Labels)
		s = append(s, containerState{
			Version:        state.BaseState.Config.Version,
			ID:             state.BaseState.ID,
			InitProcessPid: pid,
			Status:         status.String(),
			Bundle:         bundle,
			Rootfs:         state.BaseState.Config.Rootfs,
			Created:        state.BaseState.Created,
			Annotations:    annotations,
			Owner:          owner,
		})
	}
	return s, nil
}

func GetStatusState(id string) (status libcontainer.Status, state *libcontainer.State, err error) {
	container, err := libcontainer.Load(CONTAINERS_ROOT, id)
	if err != nil {
		err = fmt.Errorf("failed to load container %s: [%v]", id, err)
		return
	}
	status, err = container.Status()
	if err != nil {
		err = fmt.Errorf("failed to get container %s status: [%v]", id, err)
		return
	}
	state, err = container.State()
	if err != nil {
		err = fmt.Errorf("failed to get container %s state: [%v]", id, err)
	}
	return
}
