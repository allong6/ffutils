package ffutils

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Watermark 水印图片及其摆放方式。
type Watermark struct {
	// Path 水印图片路径（png 带透明通道最佳）
	Path string
	// Position 预设位置，见 Position 常量组（PosTopLeft 等），默认右下角
	Position Position
	// Margin 距边缘像素，默认 10
	Margin int
}

// overlayExpr 生成 overlay 滤镜参数。
func (w Watermark) overlayExpr() string {
	m := w.Margin
	if m <= 0 {
		m = 10
	}
	var pos string
	switch Position(strings.ToLower(string(w.Position))) {
	case PosTopLeft:
		pos = fmt.Sprintf("%d:%d", m, m)
	case PosTopRight:
		pos = fmt.Sprintf("W-w-%d:%d", m, m)
	case PosBottomLeft:
		pos = fmt.Sprintf("%d:H-h-%d", m, m)
	default: // PosBottomRight
		pos = fmt.Sprintf("W-w-%d:H-h-%d", m, m)
	}
	return pos
}

// FrameWriterOptions 持续推帧生成视频的参数。
type FrameWriterOptions struct {
	// Width/Height/Fps 输出视频参数，均必填
	Width  int
	Height int
	Fps    int

	// Output 输出视频路径
	Output string

	// Watermark 可选水印；nil 表示不加水印
	Watermark *Watermark

	// Encode 输出编码参数，零值用默认（libx264 / ultrafast / yuv420p / aac）
	Encode EncodeOptions
}

// FrameWriter 持续推帧编码器：通过管道向 ffmpeg 写入一帧帧图片
// （JPEG/PNG 等完整编码图，ffmpeg 按内容自动识别），写完后 Close 得到视频。
// 典型用法见 NewFrameWriter 的示例。
type FrameWriter struct {
	cmd  *exec.Cmd
	stdin io.WriteCloser
	done  chan error
}

// NewFrameWriter 启动 ffmpeg 并返回帧写入器。
//
//	w, err := ff.NewFrameWriter(ffutils.FrameWriterOptions{
//	    Width: 1920, Height: 1080, Fps: 30,
//	    Output: "out.mp4",
//	    Watermark: &ffutils.Watermark{Path: "logo.png", Position: "bottomright"},
//	})
//	// 循环生成帧后：
//	w.Write(jpegBytes) // 每次写入完整的一帧图片
//	w.Close()          // 关闭管道并等待编码完成
func (f *FFmpeg) NewFrameWriter(opts FrameWriterOptions) (*FrameWriter, error) {
	if opts.Width <= 0 || opts.Height <= 0 || opts.Fps <= 0 {
		return nil, fmt.Errorf("Width/Height/Fps 必须为正数: %d x %d @ %d", opts.Width, opts.Height, opts.Fps)
	}
	if opts.Output == "" {
		return nil, fmt.Errorf("Output 不能为空")
	}

	args := []string{
		"-f", "image2pipe",
		"-r", fmt.Sprintf("%d", opts.Fps),
		"-s", fmt.Sprintf("%dx%d", opts.Width, opts.Height),
		"-i", "pipe:0",
	}
	if opts.Watermark != nil && opts.Watermark.Path != "" {
		args = append(args,
			"-i", opts.Watermark.Path,
			"-filter_complex", fmt.Sprintf("[0:v][1:v]overlay=%s[v]", opts.Watermark.overlayExpr()),
			"-map", "[v]",
		)
	}
	args = append(args, opts.Encode.outputArgs()...)
	args = append(args, opts.Output)

	cmd := exec.Command(f.ffmpegBin(), args...)
	if f.Dir != "" {
		cmd.Dir = f.Dir
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("创建输入管道失败: %w", err)
	}
	stderr, _ := cmd.StderrPipe()

	// 子进程立即启动，帧数据通过 stdin 管道持续写入；
	// 进程随 writer 生命周期存续，直到 Close/Abort
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("启动 ffmpeg 失败: %w", err)
	}

	w := &FrameWriter{cmd: cmd, stdin: stdin, done: make(chan error, 1)}
	// 后台 goroutine 收集 stderr 并等待进程退出：
	//   - ffmpeg 运行期不断向 stderr 输出进度日志，必须有人读走否则管道写满后 ffmpeg 会阻塞；
	//   - 退出码非 0 时把日志尾部与错误一起送入 done，由 Close() 返回给调用方。
	go func() {
		msg, _ := io.ReadAll(stderr)
		if err := cmd.Wait(); err != nil {
			err = fmt.Errorf("ffmpeg 编码失败: %w\n%s", err, tailLines(string(msg), 15))
		}
		w.done <- err
	}()
	return w, nil
}

// Write 写入一帧（必须是完整编码的一张图片，如 JPEG/PNG 字节）。
// image2pipe 协议按图片边界自动切帧，无需额外分隔符；
// 多次 Write 的顺序即帧序，时长 = 帧数 / Fps。
// 若 ffmpeg 已异常退出，这里会得到 pipe broken 错误——真正的失败原因
// 在 Close() 返回的错误里（含 stderr 日志尾部）。
func (w *FrameWriter) Write(frame []byte) error {
	if _, err := w.stdin.Write(frame); err != nil {
		return fmt.Errorf("写入帧失败（ffmpeg 可能已退出）: %w", err)
	}
	return nil
}

// Close 关闭输入管道并等待编码完成。务必调用，否则视频尾部可能损坏。
func (w *FrameWriter) Close() error {
	if err := w.stdin.Close(); err != nil {
		return err
	}
	return <-w.done
}

// Abort 强制终止编码（出错回滚场景），不等待正常收尾。
func (w *FrameWriter) Abort() {
	_ = w.cmd.Process.Kill()
	<-w.done
}
