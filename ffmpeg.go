package ffutils

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// FFmpeg 是所有操作的入口，封装工具路径与执行配置。
// 通过 New 创建后即可调用 Probe / ExtractFrame / Concat 等方法。
type FFmpeg struct {
	// FFmpegPath ffmpeg 可执行文件路径，默认 "ffmpeg"
	FFmpegPath string
	// FFprobePath ffprobe 可执行文件路径，默认 "ffprobe"
	FFprobePath string
	// Dir 子进程工作目录，空表示继承当前进程
	Dir string
	// Timeout 单次命令超时，<=0 表示不限制
	Timeout time.Duration

	// 版本探测缓存（见 version.go）：能力标志据此选择等价/降级路径
	verOnce  sync.Once
	verMajor int
	verMinor int
	verOK    bool
}

// New 返回使用环境变量中 ffmpeg/ffprobe 的实例。
func New() *FFmpeg {
	return &FFmpeg{FFmpegPath: "ffmpeg", FFprobePath: "ffprobe"}
}

// CheckVersion 校验工具可用（执行 -version），只探测、不删除任何文件。
func (f *FFmpeg) CheckVersion(toolPath string) error {
	cmd := exec.Command(toolPath, "-version")
	hideConsole(cmd)
	if f.Dir != "" {
		cmd.Dir = f.Dir
	}
	_, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("工具 %s 不可用: %w", toolPath, err)
	}
	return nil
}

// run 统一的命令执行入口：
//   - 合并 stdout/stderr，便于失败时还原 ffmpeg 的实际报错；
//   - Timeout 到点杀掉子进程并标注超时（避免僵尸 ffmpeg 卡死上层业务）；
//   - 失败时只带输出末尾 15 行，完整日志对调用方通常是噪音。
func (f *FFmpeg) run(name string, args []string) (string, error) {
	cmd := exec.Command(name, args...)
	hideConsole(cmd)
	if f.Dir != "" {
		cmd.Dir = f.Dir
	}
	// AfterFunc + Kill：exec.Command 没有原生超时支持，用定时器兜底
	var timer *time.Timer
	var timedOut bool
	if f.Timeout > 0 {
		timer = time.AfterFunc(f.Timeout, func() {
			timedOut = true
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		})
		defer timer.Stop()
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		if timedOut {
			return string(out), fmt.Errorf("%s 执行超时(%s): %w", name, f.Timeout, err)
		}
		return string(out), fmt.Errorf("%s 执行失败: %w\n%s", name, err, tailLines(string(out), 15))
	}
	return string(out), nil
}

// tailLines 返回输出末尾 n 行，用于错误信息截断冗长的 ffmpeg 日志。
func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\r\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
