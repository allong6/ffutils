//go:build !windows

package ffutils

import "os/exec"

// hideConsole 非 Windows 平台无控制台闪烁问题，空实现。
func hideConsole(cmd *exec.Cmd) {}
