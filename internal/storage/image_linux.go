package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/containers/storage/pkg/unshare"

	"github.com/L-F-Z/cri-t/internal/dbusmgr"
	"github.com/L-F-Z/cri-t/utils"
)

// moveSelfToCgroup moves the current process to a new transient cgroup.
func moveSelfToCgroup(cgroup string) error {
	slice := "system.slice"
	if unshare.IsRootless() {
		slice = "user.slice"
	}

	if cgroup != "" {
		if !strings.Contains(cgroup, ".slice") {
			return fmt.Errorf("invalid systemd cgroup %q", cgroup)
		}
		slice = filepath.Base(cgroup)
	}

	unitName := fmt.Sprintf("crio-pull-image-%d.scope", os.Getpid())

	return utils.RunUnderSystemdScope(dbusmgr.NewDbusConnManager(unshare.IsRootless()), os.Getpid(), slice, unitName)
}
