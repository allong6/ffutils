package ffutils

import (
	"fmt"
	"strings"
)

// ComposeGrid 宫格分屏：把多个视频按 cols×rows 网格同屏排列
// （如 2 路左右分屏、4 路四宫格、9 路监控墙）。每个格子统一缩放到
// 所有输入中的最大宽高，不足 cols×rows 的空位用黑块填充。
// audioTrack 指定使用第几个输入的音轨（其余静默），默认 0；传 -1 表示输出无音轨。
func (f *FFmpeg) ComposeGrid(clips []string, cols, rows int, audioTrack int, output string, enc EncodeOptions) error {
	if cols < 1 || rows < 1 {
		return fmt.Errorf("行列数必须 >=1: %dx%d", cols, rows)
	}
	if len(clips) < 1 {
		return fmt.Errorf("至少需要一个输入")
	}
	if len(clips) > cols*rows {
		return fmt.Errorf("输入数量 %d 超过网格容量 %d", len(clips), cols*rows)
	}

	// 探测全部输入，确定统一的格子尺寸与帧率，以及占位黑块应持续的时长
	infos := make([]*ProbeResult, len(clips))
	cellW, cellH, fps, maxDur := 0, 0, 0.0, 0.0
	for i, p := range clips {
		info, err := f.Probe(p)
		if err != nil {
			return fmt.Errorf("探测 %s 失败: %w", p, err)
		}
		if !info.HasVideo {
			return fmt.Errorf("%s 没有视频流", p)
		}
		infos[i] = info
		if info.Video.Width > cellW {
			cellW = info.Video.Width
		}
		if info.Video.Height > cellH {
			cellH = info.Video.Height
		}
		if info.Duration > maxDur {
			maxDur = info.Duration
		}
		if fps == 0 {
			fps = info.Video.FrameRate
			if fps <= 0 || fps > 240 {
				fps = 30
			}
		}
	}
	cellW &^= 1
	cellH &^= 1
	if maxDur <= 0 {
		maxDur = 1
	}

	var args []string

	// 实际输入 + 黑块占位输入（lavfi color 源）。
	// color 源必须限定时长（d=），否则 vstack/hstack 等最长输入时永不结束。
	padCount := cols*rows - len(clips)
	for _, p := range clips {
		args = append(args, "-i", p)
	}
	for i := 0; i < padCount; i++ {
		args = append(args, "-f", "lavfi", "-i",
			fmt.Sprintf("color=black:s=%dx%d:r=%d:d=%.3f", cellW, cellH, int(fps), maxDur))
	}

	// 滤镜链分段收集，最终 join 成 filtergraph（避免尾部分号等格式问题）。
	// hstack/vstack 的 inputs 至少为 2：单行/单列时直接短路，不经过 stack。
	var chains []string
	total := cols * rows
	for i := 0; i < total; i++ {
		chains = append(chains, fmt.Sprintf(
			"[%d:v]setpts=PTS-STARTPTS,fps=%d,scale=%d:%d,setsar=1[g%d]",
			i, int(fps), cellW, cellH, i))
	}

	var rowLabels []string
	for r := 0; r < rows; r++ {
		var cells []string
		for c := 0; c < cols; c++ {
			cells = append(cells, fmt.Sprintf("[g%d]", r*cols+c))
		}
		if cols == 1 {
			rowLabels = append(rowLabels, cells[0]) // 单列：格子即行
			continue
		}
		lbl := fmt.Sprintf("[r%d]", r)
		chains = append(chains, strings.Join(cells, "")+fmt.Sprintf("hstack=inputs=%d%s", cols, lbl))
		rowLabels = append(rowLabels, lbl)
	}

	// 最终输出 label
	finalLabel := "[v]"
	if rows == 1 {
		finalLabel = rowLabels[0] // 单行：行输出即最终画面
	} else {
		chains = append(chains, strings.Join(rowLabels, "")+fmt.Sprintf("vstack=inputs=%d[v]", rows))
	}

	args = append(args, "-filter_complex", strings.Join(chains, ";"), "-map", finalLabel)
	if audioTrack >= 0 {
		if audioTrack >= len(clips) {
			return fmt.Errorf("audioTrack=%d 超出输入范围（共 %d 个）", audioTrack, len(clips))
		}
		if infos[audioTrack].HasAudio {
			args = append(args, "-map", fmt.Sprintf("%d:a:0", audioTrack))
		}
	}
	// 各输入时长不同时以最长的为准，短的画面冻结（默认行为），不截断
	args = append(args, enc.outputArgs()...)
	args = append(args, output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// SplitScreen 双画面分屏的快捷方式（左右 1×2 或上下 2×1）。
func (f *FFmpeg) SplitScreen(left, right string, vertical bool, output string, enc EncodeOptions) error {
	if vertical {
		return f.ComposeGrid([]string{left, right}, 1, 2, 0, output, enc)
	}
	return f.ComposeGrid([]string{left, right}, 2, 1, 0, output, enc)
}

// PiPOptions 画中画参数。
type PiPOptions struct {
	// Scale 小画面宽度占主画面宽度的比例（0.05~1.0），默认 0.3
	Scale float64
	// Position 小画面位置，见 Position 常量组，默认右下角
	Position Position
	// Margin 小画面距边缘像素，默认 10
	Margin int
	// Opacity 小画面不透明度 0~1，默认 1（不透明）
	Opacity float64
	// UsePipAudio 用小画面的音轨替换主画面音轨（默认保留主画面声音）
	UsePipAudio bool
}

// PictureInPicture 画中画：主画面全屏，小画面叠加在角落（直播摄像头小窗、
// 素材对比等场景）。输出时长以主画面为准。
func (f *FFmpeg) PictureInPicture(main, pip string, opts PiPOptions, output string, enc EncodeOptions) error {
	mainInfo, err := f.Probe(main)
	if err != nil {
		return fmt.Errorf("探测主画面失败: %w", err)
	}
	if !mainInfo.HasVideo {
		return fmt.Errorf("%s 没有视频流", main)
	}
	if _, err := f.Probe(pip); err != nil {
		return fmt.Errorf("探测小画面失败: %w", err)
	}

	if opts.Scale <= 0 {
		opts.Scale = 0.3
	}
	if opts.Scale > 1 {
		opts.Scale = 1
	}
	m := opts.Margin
	if m <= 0 {
		m = 10
	}
	// 小画面目标尺寸按主画面宽度比例计算（偶数）
	pipW := int(float64(mainInfo.Video.Width) * opts.Scale)
	pipW -= pipW % 2
	if pipW < 2 {
		pipW = 2
	}

	wm := Watermark{Position: opts.Position, Margin: m}
	pos := wm.overlayExpr()

	// 小画面链：统一时间戳 -> 缩放 -> （可选半透明）
	pipChain := fmt.Sprintf("[1:v]setpts=PTS-STARTPTS,scale=%d:-2", int(pipW))
	if opts.Opacity > 0 && opts.Opacity < 1 {
		pipChain += fmt.Sprintf(",format=rgba,colorchannelmixer=aa=%.2f", opts.Opacity)
	}
	filter := fmt.Sprintf("%s[p];[0:v][p]overlay=%s:shortest=1[v]", pipChain, pos)

	args := []string{"-i", main, "-i", pip,
		"-filter_complex", filter, "-map", "[v]"}
	if opts.UsePipAudio {
		args = append(args, "-map", "1:a:0?")
	} else {
		args = append(args, "-map", "0:a:0?")
	}
	// 以主画面时长为准
	if mainInfo.Duration > 0 {
		args = append(args, "-t", fmt.Sprintf("%.3f", mainInfo.Duration))
	}
	args = append(args, enc.outputArgs()...)
	args = append(args, output)
	_, err = f.run(f.ffmpegBin(), args)
	return err
}
