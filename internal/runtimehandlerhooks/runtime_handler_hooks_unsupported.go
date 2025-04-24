//go:build !linux

package runtimehandlerhooks

import (
	"context"

	"github.com/L-F-Z/cri-t/internal/log"
	libconfig "github.com/L-F-Z/cri-t/pkg/config"
)

const (
	IrqSmpAffinityProcFile = ""
)

// GetRuntimeHandlerHooks returns RuntimeHandlerHooks implementation by the runtime handler name
func GetRuntimeHandlerHooks(ctx context.Context, config *libconfig.Config, handler string, annotations map[string]string) (RuntimeHandlerHooks, error) {
	ctx, span := log.StartSpan(ctx)
	defer span.End()
	return &DefaultCPULoadBalanceHooks{}, nil
}

// RestoreIrqBalanceConfig restores irqbalance service with original banned cpu mask settings
func RestoreIrqBalanceConfig(ctx context.Context, irqBalanceConfigFile, irqBannedCPUConfigFile, irqSmpAffinityProcFile string) error {
	return nil
}
