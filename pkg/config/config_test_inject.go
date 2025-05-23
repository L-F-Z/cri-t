//go:build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package config

import (
	"github.com/cri-o/ocicni/pkg/ocicni"

	"github.com/L-F-Z/cri-t/internal/config/cnimgr"
	"github.com/L-F-Z/cri-t/internal/config/nsmgr"
)

// SetCNIPlugin sets the network plugin for the Configuration. The function
// errors if a sane shutdown of the initially created network plugin failed.
func (c *Config) SetCNIPlugin(plugin ocicni.CNIPlugin) error {
	if c.cniManager == nil {
		c.cniManager = &cnimgr.CNIManager{}
	}
	return c.cniManager.SetCNIPlugin(plugin)
}

// SetNamespaceManager sets the namespaceManager for the Configuration.
func (c *Config) SetNamespaceManager(nsMgr *nsmgr.NamespaceManager) {
	c.namespaceManager = nsMgr
}
