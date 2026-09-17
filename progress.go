package ffutils

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Progress 转码进度快照。Percent 仅在已知总时长时有效（否则恒为 0）。
type Progress struct {
	// OutTimeSec 已输出的内容时长（秒）
	OutTimeSec float64
	// Frame 已编码帧数
	Frame int64
	// Speed 编码速度倍率（1.0 = 实时）
	Speed float64
	// Percent 完成百分比 0~100，总时长未知时为 0
	Percent float64
}

// ProgressFunc 进度回调。在 ffmpeg 输出进度行的线程上被调用，
// 实现方不应在其中做耗时操作。返回 error 会终止并取消编码。
type ProgressFunc func(p Progress) error

// runProgress 与 run 类似，但额外传 -progress pipe:1 以获得机器可读的
// 进度行（key=value，每秒一行），解析后回调 onProgress。
// totalSec>0 时计算 Percent；onProgress 返回错误会杀掉进程并以该错误返回。
func (f *FFmpeg) runProgress(args []string, totalSec float64, onProgress ProgressFunc) (string, error) {
	if onProgress == nil {
		return f.run(f.ffmpegBin(), args)
	}
	full := append([]string{"-nostats", "-progress", "pipe:1"}, args...)
	cmd := exec.Command(f.ffmpegBin(), full...)
	if f.Dir != "" {
		cmd.Dir = f.Dir
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var timer *time.Timer
	var timedOut bool
	if f.Timeout > 0 {
		timer = time.AfterFunc(f.Timeout, func() {
			timedOut = true
			_ = cmd.Process.Kill()
		})
		defer timer.Stop()
	}

	// 进度行是 "key=value" 文本，以 progress=begin/continue/end 作为每块结尾
	parseErr := make(chan error, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64*1024), 64*1024)
		var p Progress
		var kv = map[string]string{}
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" {
				continue
			}
			if i := strings.IndexByte(line, '='); i > 0 {
				kv[line[:i]] = line[i+1:]
			}
			// 每个进度块以 progress=xxx 结束，此时kv已凑齐一帧快照
			if v, ok := kv["progress"]; ok {
				if us, err := strconv.ParseFloat(kv["out_time_us"], 64); err == nil {
					p.OutTimeSec = us / 1e6
				}
				p.Frame, _ = strconv.ParseInt(kv["frame"], 10, 64)
				p.Speed = atofSafe(strings.TrimSuffix(kv["speed"], "x"))
				if totalSec > 0 {
					p.Percent = p.OutTimeSec / totalSec * 100
					if p.Percent > 100 {
						p.Percent = 100
					}
				}
				if err := onProgress(p); err != nil {
					_ = cmd.Process.Kill()
					parseErr <- err
					return
				}
				kv = map[string]string{}
				_ = v
			}
		}
		parseErr <- nil
	}()

	errOut, _ := io.ReadAll(stderr)
	waitErr := cmd.Wait()
	cbErr := <-parseErr
	if cbErr != nil {
		return string(errOut), fmt.Errorf("进度回调取消编码: %w", cbErr)
	}
	if waitErr != nil {
		if timedOut {
			return string(errOut), fmt.Errorf("ffmpeg 执行超时(%s): %w", f.Timeout, waitErr)
		}
		return string(errOut), fmt.Errorf("ffmpeg 执行失败: %w\n%s", waitErr, tailLines(string(errOut), 15))
	}
	return string(errOut), nil
}

// RunWithProgress 高级接口：用完整 ffmpeg 参数执行并获取进度。
// args 为不含 ffmpeg 可执行名的完整参数（输出参数照常以 -y 结尾自己拼），
// totalSec 为成片总时长（用于计算百分比，未知传 0）。
func (f *FFmpeg) RunWithProgress(args []string, totalSec float64, onProgress ProgressFunc) error {
	_, err := f.runProgress(args, totalSec, onProgress)
	return err
}
