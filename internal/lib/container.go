package lib

import (
	"context"
	"errors"
	"fmt"

	"github.com/L-F-Z/cri-t/internal/lib/sandbox"
	"github.com/L-F-Z/cri-t/internal/oci"
	"github.com/L-F-Z/cri-t/internal/registrar"
)

// GetContainerFromShortID gets an oci container matching the specified full or partial id.
func (c *ContainerServer) GetContainerFromShortID(ctx context.Context, cid string) (*oci.Container, error) {
	if cid == "" {
		return nil, errors.New("container ID should not be empty")
	}

	containerID, err := c.ctrIDIndex.Get(cid)
	if err != nil {
		return nil, fmt.Errorf("container with ID starting with %s not found: %w", cid, err)
	}

	ctr := c.GetContainer(ctx, containerID)
	if ctr == nil {
		return nil, fmt.Errorf("specified container not found: %s", containerID)
	}

	if !ctr.Created() {
		return nil, fmt.Errorf("specified container %s is not yet created", containerID)
	}

	return ctr, nil
}

// LookupContainer returns the container with the given name or full or partial id.
func (c *ContainerServer) LookupContainer(ctx context.Context, idOrName string) (*oci.Container, error) {
	if idOrName == "" {
		return nil, errors.New("container ID or name should not be empty")
	}

	ctrID, err := c.ctrNameIndex.Get(idOrName)
	if err != nil {
		if errors.Is(err, registrar.ErrNameNotReserved) {
			ctrID = idOrName
		} else {
			return nil, err
		}
	}

	return c.GetContainerFromShortID(ctx, ctrID)
}

func (c *ContainerServer) getSandboxFromRequest(pid string) (*sandbox.Sandbox, error) {
	if pid == "" {
		return nil, errors.New("pod ID should not be empty")
	}

	podID, err := c.podIDIndex.Get(pid)
	if err != nil {
		return nil, fmt.Errorf("pod with ID starting with %s not found: %w", pid, err)
	}

	sb := c.GetSandbox(podID)
	if sb == nil {
		return nil, fmt.Errorf("specified pod not found: %s", podID)
	}
	return sb, nil
}

// LookupSandbox returns the pod sandbox with the given name or full or partial id.
func (c *ContainerServer) LookupSandbox(idOrName string) (*sandbox.Sandbox, error) {
	if idOrName == "" {
		return nil, errors.New("container ID or name should not be empty")
	}

	podID, err := c.podNameIndex.Get(idOrName)
	if err != nil {
		if errors.Is(err, registrar.ErrNameNotReserved) {
			podID = idOrName
		} else {
			return nil, err
		}
	}

	return c.getSandboxFromRequest(podID)
}
