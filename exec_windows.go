//go:build windows

package ffutils

import (
	"os/exec"
	"syscall"
)

// hideConsole 阻止 GUI 程序拉起控制台子进程（ffmpeg/ffprobe）时闪烁黑窗。
// CREATE_NO_WINDOW：不为子进程分配控制台；HideWindow：万一继承到了也隐藏。
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
