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
	// Position 预设位置，见 Position 常量组（PosTopLeft 等），默认右下角；
	// Motion 启用时作为轨迹的起始角/锚定角
	Position Position
	// Margin 距边缘像素，默认 10
	Margin int
	// Opacity 不透明度 0~1（0 或 >=1 视为完全不透明）
	Opacity float64
	// TileGap 平铺水印（PosTile）格子的透明间隔（像素），<=0 无间隔。
	// 格子宽度固定为主画面 1/4，间隔从水印内容宽度里扣（水印缩到
	// 格宽-2×间隔后补透明边，网格总尺寸不变）。
	// Since v1.1.0.
	TileGap int
	// ScalePct 水印宽度按主画面宽的百分比缩放（高度按宽高比自适应）：
	// 50 = 水印宽为主画面宽的 50%。0 = 原始尺寸（现状不变）；有效范围
	// 1~100，与 PosTile 互斥（平铺已有固定的 1/4 宽缩放）。
	// 主画面宽指水印叠加时刻的画面宽——多路滤镜链中由库按链序推导
	// （Fit 优先，其次裁剪+旋转换算，最后探测原始宽），调用方无需
	// 自行换算像素。
	// Since v1.2.0.
	ScalePct int
	// Motion 动态轨迹（防盗用动态水印）；nil = 静态水印（现状不变）。
	// 与 PosTile 互斥。ScalePct 可与 Motion 自由组合（缩放在前，轨迹
	// 表达式引用的 w/h 即缩放后的运行时值）。
	// Since v1.2.0.
	Motion *Motion
}

// Motion 水印动态轨迹参数，挂在 Watermark.Motion 上。时间参数一律秒。
// 零值字段按各轨迹的默认规则兜底（见各字段说明）。
// Since v1.2.0.
type Motion struct {
	// Kind 轨迹类型，见 MotionKind 常量组；"" = 静态（等同 Motion 为 nil）
	Kind MotionKind
	// Interval MotionRotate 的换角间隔秒；<=0 默认 10
	Interval float64
	// TX/TY MotionBounce 的水平/垂直单程周期秒（完整往返为 2×周期）；
	// 某轴 <=0 时该轴锚定 Position 不动，两轴同时 <=0 报错（等于静态）
	TX, TY float64
	// Period MotionDrift 完整穿越画面一次的秒数；<=0 默认 10
	Period float64
	// Axis MotionDrift 的漂移轴向，见 DriftAxis 常量组；"" 默认水平
	Axis DriftAxis
}

// overlayExpr 生成 overlay 滤镜的位置参数（静态坐标，不含动态轨迹）。
// 零值默认右下角；PosTile 不是坐标能表达的（需要整条滤镜链，见
// watermarkTileSegments），在只取坐标的调用点（画中画等）直接报错。
func (w Watermark) overlayExpr() (string, error) {
	x, y, err := w.posXY()
	if err != nil {
		return "", err
	}
	return x + ":" + y, nil
}

// posXY 生成 Position 对应的静态坐标表达式对（overlay 运行时变量
// W/H/w/h 组成），供静态摆放与 Motion 轨迹的锚定轴共用。Margin 零值
// 默认 10（与 overlayExpr 一致）。
func (w Watermark) posXY() (x, y string, err error) {
	m := w.Margin
	if m <= 0 {
		m = 10
	}
	switch Position(strings.ToLower(string(w.Position))) {
	case "", PosBottomRight:
		return fmt.Sprintf("W-w-%d", m), fmt.Sprintf("H-h-%d", m), nil
	case PosTopLeft:
		return fmt.Sprintf("%d", m), fmt.Sprintf("%d", m), nil
	case PosTopRight:
		return fmt.Sprintf("W-w-%d", m), fmt.Sprintf("%d", m), nil
	case PosBottomLeft:
		return fmt.Sprintf("%d", m), fmt.Sprintf("H-h-%d", m), nil
	case PosTile:
		return "", "", fmt.Errorf("平铺位置（tile）只用于整幅水印叠加，不支持此处")
	default:
		return "", "", fmt.Errorf("未知水印位置: %q", string(w.Position))
	}
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
	cmd   *exec.Cmd
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
		if err := validateWatermark(opts.Watermark); err != nil {
			return nil, err
		}
		args = append(args, "-i", opts.Watermark.Path)
		var fc string
		if opts.Watermark.Position == PosTile {
			// 推帧画布尺寸已知（就是输出尺寸），平铺水印按其 1/4 宽缩放
			segs, err := watermarkTileSegments("[0:v]", "[1:v]", "[v]",
				tileCellWidth(opts.Width), opts.Watermark.Opacity, opts.Watermark.TileGap)
			if err != nil {
				return nil, err
			}
			fc = strings.Join(segs, ";")
		} else {
			// 画布宽即输出宽；透明度此前在该路径被忽略，自 v1.2.0 起
			// 与其他水印路径一致生效（统一走 watermarkSegments）
			segs, err := watermarkSegments("[0:v]", "[1:v]", "[v]",
				opts.Watermark, opts.Watermark.Opacity, opts.Width)
			if err != nil {
				return nil, err
			}
			fc = strings.Join(segs, ";")
		}
		args = append(args,
			"-filter_complex", fc,
			"-map", "[v]",
		)
	}
	args = append(args, opts.Encode.outputArgs()...)
	args = append(args, opts.Output)

	cmd := exec.Command(f.ffmpegBin(), args...)
	if f.Dir != "" {
		cmd.Dir = f.Dir
	}
	hideConsole(cmd)
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
