package runtimehandlerhooks

import (
	"context"
	"sync"

	"github.com/opencontainers/runtime-tools/generate"

	"github.com/L-F-Z/cri-t/internal/lib/sandbox"
	"github.com/L-F-Z/cri-t/internal/oci"
)

var (
	cpuLoadBalancingAllowedAnywhereOnce sync.Once
	cpuLoadBalancingAllowedAnywhere     bool
)

//nolint:iface // interface duplication is intentional
type RuntimeHandlerHooks interface {
	PreCreate(ctx context.Context, specgen *generate.Generator, s *sandbox.Sandbox, c *oci.Container) error
	PreStart(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
	PreStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
	PostStop(ctx context.Context, c *oci.Container, s *sandbox.Sandbox) error
}

//nolint:iface // interface duplication is intentional
type HighPerformanceHook interface {
	RuntimeHandlerHooks
}
