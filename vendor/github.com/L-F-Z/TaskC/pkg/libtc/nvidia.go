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
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func addNvidiaGPU(cfg *configs.Config, rootfs string) error {
	devices := []string{"0"}
	capabilities := []string{"compute", "compat32", "graphics", "utility", "video", "display"}
	var args []string
	args = append(args, "--load-kmods")
	args = append(args, "configure")
	args = append(args, fmt.Sprintf("--device=%s", strings.Join(devices, ",")))
	for _, c := range capabilities {
		args = append(args, fmt.Sprintf("--%s", c))
	}

	nvidiaPath, err := exec.LookPath("nvidia-container-cli")
	if err != nil {
		return err
	}

	hook := configs.NewFunctionHook(func(s *specs.State) error {
		args = append(args, "--pid="+strconv.Itoa(s.Pid))
		args = append(args, rootfs)
		cmd := exec.Command(nvidiaPath, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			return fmt.Errorf("nvidia-container-cli execution failed: %w", err)
		}
		return nil
	})

	// TODO: check if there are existing hooks. For simplicity, just override here
	cfg.Hooks[configs.Prestart] = []configs.Hook{hook}

	return nil
}
