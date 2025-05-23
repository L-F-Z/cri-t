package lib

import (
	"errors"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	selinux "github.com/opencontainers/selinux/go-selinux"

	"github.com/L-F-Z/cri-t/internal/lib/sandbox"
)

func (c *ContainerServer) addSandboxPlatform(sb *sandbox.Sandbox) error {
	context, err := selinux.NewContext(sb.ProcessLabel())
	if err != nil {
		return err
	}
	c.state.processLevels[context["level"]]++
	return nil
}

func (c *ContainerServer) removeSandboxPlatform(sb *sandbox.Sandbox) error {
	processLabel := sb.ProcessLabel()
	context, err := selinux.NewContext(processLabel)
	if err != nil {
		return err
	}
	level := context["level"]
	pl, ok := c.state.processLevels[level]
	if ok {
		c.state.processLevels[level] = pl - 1
		if c.state.processLevels[level] == 0 {
			defer delete(c.state.processLevels, level)
			selinux.ReleaseLabel(processLabel)
		}
	}
	return nil
}

func configNsPath(spec *rspec.Spec, nsType rspec.LinuxNamespaceType) (string, error) {
	for _, ns := range spec.Linux.Namespaces {
		if ns.Type != nsType {
			continue
		}

		if ns.Path == "" {
			return "", errors.New("empty networking namespace")
		}

		return ns.Path, nil
	}

	return "", errors.New("missing networking namespace")
}
