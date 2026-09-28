package ffutils

import (
	"fmt"
	"strings"
)

// GridAudio 宫格合成时保留的一路声音：Index 为输入序号，Volume 为音量
// 倍率（1=原样，0.5=减半；<=0 按 1 处理）。
type GridAudio struct {
	Index  int
	Volume float64
}

// ComposeGrid 宫格分屏：把多个视频按 cols×rows 网格同屏排列
// （如 2 路左右分屏、4 路四宫格、9 路监控墙）。每个格子统一到
// 所有输入中的最大宽高（宽高比不同的输入默认裁剪填满，见 GridOptions.Fit），
// 不足 cols×rows 的空位用黑块填充。
// audioTrack 指定使用第几个输入的音轨（其余静默），默认 0；传 -1 表示输出无音轨。
func (f *FFmpeg) ComposeGrid(clips []string, cols, rows int, audioTrack int, output string, enc EncodeOptions) error {
	if audioTrack >= len(clips) {
		return fmt.Errorf("audioTrack=%d 超出输入范围（共 %d 个）", audioTrack, len(clips))
	}
	var audio []GridAudio
	if audioTrack >= 0 {
		audio = []GridAudio{{Index: audioTrack}}
	}
	return f.ComposeGridAudio(clips, cols, rows, audio, output, enc)
}

// GridOptions 宫格合成选项（零值均为合理默认）。
type GridOptions struct {
	Cols, Rows int
	Audio      []GridAudio
	// Shortest 时长基准：false（默认）以最长输入为准（短画面定格）；
	// true 以最短输入为准（其余超出部分截断，适合"都只播这么长"）
	Shortest bool
	// Fit 各输入宽高比与格子尺寸不一致时的适配方式（见 FitMode），
	// 零值 = FitCrop（裁剪填满，不变形）。2026-09 之前固定为拉伸变形。
	Fit FitMode
}

// ComposeGridOpts 带选项的宫格合成（ComposeGrid/ComposeGridAudio 的
// 完整版；后续新选项都加在这里）。
func (f *FFmpeg) ComposeGridOpts(clips []string, o GridOptions, output string, enc EncodeOptions) error {
	cols, rows := o.Cols, o.Rows
	audio := o.Audio
	if cols < 1 || rows < 1 {
		return fmt.Errorf("行列数必须 >=1: %dx%d", cols, rows)
	}
	if len(clips) < 1 {
		return fmt.Errorf("至少需要一个输入")
	}
	if len(clips) > cols*rows {
		return fmt.Errorf("输入数量 %d 超过网格容量 %d", len(clips), cols*rows)
	}
	if err := validateFitMode(o.Fit); err != nil {
		return err
	}

	// 探测全部输入，确定统一的格子尺寸与帧率，以及合成时长基准
	infos := make([]*ProbeResult, len(clips))
	cellW, cellH, fps := 0, 0, 0.0
	outDur := 0.0
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
		if i == 0 || (o.Shortest && info.Duration < outDur) || (!o.Shortest && info.Duration > outDur) {
			outDur = info.Duration
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
	if outDur <= 0 {
		outDur = 1
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
			fmt.Sprintf("color=black:s=%dx%d:r=%d:d=%.3f", cellW, cellH, int(fps), outDur))
	}

	// 滤镜链分段收集，最终 join 成 filtergraph（避免尾部分号等格式问题）。
	// hstack/vstack 的 inputs 至少为 2：单行/单列时直接短路，不经过 stack。
	// 黑块占位输入本身就是 cellW×cellH，过一遍 fit 链等价于恒等变换。
	var chains []string
	total := cols * rows
	for i := 0; i < total; i++ {
		chains = append(chains, fmt.Sprintf(
			"[%d:v]setpts=PTS-STARTPTS,fps=%d,%s,setsar=1[g%d]",
			i, int(fps), fitFilterExpr(cellW, cellH, o.Fit, ""), i))
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

	// 音轨选择链：各路统一采样格式后按 Volume 增益，amix 混成单轨。
	// aresample/aformat 先归一（amix 要求各输入采样率/声道一致）。
	// 旧版 amix（<4.4 无 normalize）默认每路乘 1/N：各路预乘 N 抵消。
	// audio 为空（nil 或显式空切片）= 不保留声音——ComposeGridAudio 的
	// API 契约（NoAudioKept 固化）；调用方需要"默认保留第一路"语义时
	// 自行传入（gui 的 execTask 对未指定 AudioSel 的分屏任务传第一个
	// 有音轨的输入）。
	var amixInputs strings.Builder
	keptTotal := 0
	for _, a := range audio {
		if a.Index >= 0 && a.Index < len(clips) && infos[a.Index].HasAudio {
			keptTotal++
		}
	}
	boost := 1.0
	if !f.HasAmixNormalize() {
		boost = float64(keptTotal)
	}
	kept := 0
	for _, a := range audio {
		if a.Index < 0 || a.Index >= len(clips) || !infos[a.Index].HasAudio {
			continue
		}
		v := a.Volume
		if v <= 0 {
			v = 1
		}
		v *= boost
		chain := fmt.Sprintf("[%d:a]aresample=48000,aformat=sample_fmts=fltp:channel_layouts=stereo", a.Index)
		if v != 1 {
			chain += fmt.Sprintf(",volume=%.2f", v)
		}
		chains = append(chains, chain+fmt.Sprintf("[av%d]", a.Index))
		amixInputs.WriteString(fmt.Sprintf("[av%d]", a.Index))
		kept++
	}
	if kept > 0 {
		chains = append(chains, amixInputs.String()+
			fmt.Sprintf("amix=inputs=%d:duration=longest%s[a]", kept, f.amixNormalizeSuffix()))
	}

	args = append(args, "-filter_complex", strings.Join(chains, ";"), "-map", finalLabel)
	if kept > 0 {
		args = append(args, "-map", "[a]")
	}
	// Shortest：以最短输入为准，超出部分截断（默认以最长为准，短画面定格）
	if o.Shortest {
		args = append(args, "-t", fmt.Sprintf("%.3f", outDur))
	}
	args = append(args, enc.outputArgs()...)
	args = append(args, output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// ComposeGridAudio 同 ComposeGrid，但可保留多路声音并分别调音量
// （各路同时播放、自动混合）。audio 为空时输出无音轨；所选输入无音轨
// 或序号越界时跳过该路（全部无效则输出无音轨）。
// 混合用 amix normalize=0：不做自动衰减，各路实际响度只由 Volume 决定。
func (f *FFmpeg) ComposeGridAudio(clips []string, cols, rows int, audio []GridAudio, output string, enc EncodeOptions) error {
	return f.ComposeGridOpts(clips, GridOptions{Cols: cols, Rows: rows, Audio: audio}, output, enc)
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
	// UsePipAudio 保留小窗声音：与主画面声音混合（解说场景：主画面
	// 游戏声 + 小窗人声，各自可调音量）；主画面无音轨时小窗声音单独
	// 成为音轨；小窗无音轨时静默回退为只保留主画面声音。
	// 2026-09-28 语义变更：此前为"用小窗音轨替换主画面音轨"，混合才是
	// 该场景的正解，替换语义废弃。
	UsePipAudio bool
	// PipVolume 小窗声音音量倍率（1=原样，0.5=减半；<=0 按 1 处理），
	// 仅 UsePipAudio 时生效，默认 1
	PipVolume float64
	// PipEnd 小窗播完后的表现（小窗比主画面短时）：PipEndHold（默认）
	// 定格在最后一帧；PipEndHide 小窗消失、主画面继续。
	// 无论哪种，输出时长始终以主画面为准
	PipEnd PipEndAction
}

// PipEndAction 小窗结束后的表现。
type PipEndAction string

const (
	PipEndHold PipEndAction = "hold" // 定格最后一帧
	PipEndHide PipEndAction = "hide" // 小窗消失，主画面继续
)

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
	pipInfo, err := f.Probe(pip)
	if err != nil {
		return fmt.Errorf("探测小画面失败: %w", err)
	}

	if opts.Scale <= 0 {
		opts.Scale = 0.3
	}
	if opts.Scale > 1 {
		opts.Scale = 1
	}
	switch opts.PipEnd {
	case "", PipEndHold, PipEndHide:
	default:
		return fmt.Errorf("未知小窗结束动作: %q（可选 hold/hide，见 PipEndAction）", string(opts.PipEnd))
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
	pos, perr := wm.overlayExpr()
	if perr != nil {
		return fmt.Errorf("画中画位置不支持: %w", perr)
	}

	// 小画面链：统一时间戳 -> 缩放 -> （可选半透明）
	pipChain := fmt.Sprintf("[1:v]setpts=PTS-STARTPTS,scale=%d:-2", int(pipW))
	if opts.Opacity > 0 && opts.Opacity < 1 {
		pipChain += fmt.Sprintf(",format=rgba,colorchannelmixer=aa=%.2f", opts.Opacity)
	}
	// eof_action 决定小窗播完后的表现；不用 shortest=1（那会让主画面
	// 随小窗提前结束——曾致"小窗停了主画面也停了"）
	eof := "repeat" // hold：小窗定格在最后一帧
	if opts.PipEnd == PipEndHide {
		eof = "pass" // hide：小窗消失，主画面继续
	}
	filter := fmt.Sprintf("%s[p];[0:v][p]overlay=%s:eof_action=%s[v]", pipChain, pos, eof)

	// 小窗声音（可选）：统一采样格式后按音量增益，铺到主画面时长
	// （短则补静音、长则截断）；主画面有声再 amix 混成单轨（duration=first
	// 以主画面为准、normalize=0 不自动衰减——与宫格合成同一套约定，
	// 旧版 amix 每路预乘路数抵消衰减）。小窗无音轨时静默回退为主画面声音。
	var audioFilter string
	if opts.UsePipAudio && pipInfo.HasAudio {
		pv := opts.PipVolume
		if pv <= 0 {
			pv = 1
		}
		boost := 1.0
		if mainInfo.HasAudio && !f.HasAmixNormalize() {
			boost = 2
		}
		paChain := "[1:a]aresample=48000,aformat=sample_fmts=fltp:channel_layouts=stereo"
		if v := pv * boost; v != 1 {
			paChain += fmt.Sprintf(",volume=%.2f", v)
		}
		paChain += ",apad"
		if mainInfo.Duration > 0 {
			paChain += fmt.Sprintf(",atrim=0:%.3f", mainInfo.Duration)
		}
		if mainInfo.HasAudio {
			maChain := "[0:a]aresample=48000,aformat=sample_fmts=fltp:channel_layouts=stereo"
			if boost != 1 {
				maChain += fmt.Sprintf(",volume=%.2f", boost)
			}
			audioFilter = maChain + "[ma];" + paChain + "[pa]" +
				";[ma][pa]amix=inputs=2:duration=first" + f.amixNormalizeSuffix() + "[a]"
		} else {
			audioFilter = paChain + "[a]" // 主画面无音轨：小窗声音单独成为音轨
		}
	}

	args := []string{"-i", main, "-i", pip}
	if audioFilter != "" {
		filter += ";" + audioFilter
	}
	args = append(args, "-filter_complex", filter, "-map", "[v]")
	if audioFilter != "" {
		args = append(args, "-map", "[a]")
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
