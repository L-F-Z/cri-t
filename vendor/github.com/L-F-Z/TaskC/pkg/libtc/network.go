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
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	"github.com/opencontainers/runc/libcontainer/configs"
	"github.com/opencontainers/runtime-spec/specs-go"
)

var rawConfig = []byte(`{
	"cniVersion": "1.0.0",
	"name": "mynet",
	"plugins": [
	  {
		"type": "bridge",
		"bridge": "mynet0",
		"isGateway": true,
		"ipMasq": true,
		"ipam": {
		  "type": "host-local",
		  "subnet": "172.20.0.0/24",
		  "routes": [
			{ "dst": "0.0.0.0/0" }
		  ]
		}
	  },
	  {
		"type": "portmap",
		"capabilities": {"portMappings": true}
	  },
	  {
		"type": "firewall"
	  }
	]
  }`)

func addNetwork(spec *specs.Spec, id string, portMapping string) error {
	spec.Mounts = append(spec.Mounts, specs.Mount{
		Destination: "/etc/resolv.conf",
		Type:        "bind",
		Source:      "/etc/resolv.conf",
		Options:     []string{"rbind", "ro"},
	}, specs.Mount{
		Destination: "/etc/hosts",
		Type:        "bind",
		Source:      "/etc/hosts",
		Options:     []string{"rbind", "rw"},
	})

	netNS, err := testutils.NewNS()
	if err != nil {
		return fmt.Errorf("failed to create CNI netNS: %v", err)
	}
	spec.Linux.Namespaces = append(spec.Linux.Namespaces, specs.LinuxNamespace{
		Type: specs.NetworkNamespace,
		Path: netNS.Path(),
	})
	runtimeConfig := &libcni.RuntimeConf{
		ContainerID: id,
		NetNS:       netNS.Path(),
		IfName:      "eth0",
	}
	if portMapping != "" {
		parts := strings.Split(portMapping, ":")
		if len(parts) != 2 {
			return fmt.Errorf("port mapping should be in the format hostPort:containerPort")
		}
		hostPort, err := strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid host port: %v", err)
		}
		containerPort, err := strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("invalid container port: %v", err)
		}
		runtimeConfig.CapabilityArgs = map[string]any{
			"portMappings": []map[string]any{
				{
					"hostPort":      hostPort,
					"containerPort": containerPort,
					"protocol":      "tcp",
				},
			},
		}
	}

	cniConfig := &libcni.CNIConfig{Path: []string{"/opt/cni/bin"}}
	networkConfigList, err := libcni.ConfListFromBytes(rawConfig)
	if err != nil {
		return fmt.Errorf("load CNI config list failed: %v", err)
	}
	result, err := cniConfig.AddNetworkList(context.Background(), networkConfigList, runtimeConfig)
	if err == nil {
		return nil
	}
	if result != nil {
		var buf bytes.Buffer
		result.PrintTo(&buf)
		return fmt.Errorf("%v (Result) %v", err, buf.String())
	}
	return err
}

func delNetwork(config configs.Config, id string) error {
	cniConfig := &libcni.CNIConfig{Path: []string{"/opt/cni/bin"}}
	networkConfigList, err := libcni.ConfListFromBytes(rawConfig)
	if err != nil {
		return fmt.Errorf("load CNI config list failed: %v", err)
	}

	var netNSPath string
	for _, ns := range config.Namespaces {
		if ns.Type == configs.NEWNET {
			netNSPath = ns.Path
		}
	}
	if netNSPath == "" {
		return fmt.Errorf("network namespace path unfound")
	}

	runtimeConfig := &libcni.RuntimeConf{
		ContainerID: id,
		NetNS:       netNSPath,
		IfName:      "eth0",
	}

	err = cniConfig.DelNetworkList(context.Background(), networkConfigList, runtimeConfig)
	if err != nil {
		return fmt.Errorf("failed to delete CNI network: %v", err)
	}

	netNS, err := ns.GetNS(netNSPath)
	if err != nil {
		return fmt.Errorf("failed to get netNS object: %v", err)
	}
	err = testutils.UnmountNS(netNS)
	if err != nil {
		return fmt.Errorf("failed to unmount network namespace: %v", err)
	}
	return netNS.Close()
}
