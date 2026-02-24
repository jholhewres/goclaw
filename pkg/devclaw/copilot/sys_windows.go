//go:build windows

package copilot

import (
	"os/exec"
)

func setSysProcAttr(cmd *exec.Cmd) {}

func killProcGroup(cmd *exec.Cmd) error {
	if cmd.Process != nil {
		return cmd.Process.Kill()
	}
	return nil
}
