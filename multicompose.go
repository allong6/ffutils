package ffutils

import (
	"fmt"
	"math"
	"strings"
)

// MultiCompose：多视频一次成型的通用合成（复杂场景的总入口/逃生舱）。
// 把"每路预处理 → 排布 → 叠加小窗 → 水印/字幕 → 音频策略"合成为一条
// filter_complex，一次编码输出，避免多遍转码的画质与时间损耗。
//
// 与既有单项 API 的关系：单项需求优先用 Transcode/XfadeConcat/ComposeGrid/
// PictureInPicture/MixAudio 等（简单直接）；MultiCompose 用于它们覆盖不了
// 的组合（如"每段先裁剪变速再拼接，另加两路小窗与配乐"）。

// ComposeLayout 主画面的排布方式。零值 = 顺序拼接。
type ComposeLayout string

const (
	LayoutSequential ComposeLayout = ""      // 顺序首尾拼接（Transitions 可带转场）
	LayoutGrid       ComposeLayout = "grid"  // 网格同屏（分屏/宫格）
)

// ClipSpec 一路输入及其参与组合前的预处理。
// 生效顺序：Trim → Crop → Rotate → Reverse → Speed；视频与音频同步处理。
type ClipSpec struct {
	Path string

	TrimStart float64 // 秒，<=0 从头
	TrimEnd   float64 // 秒（原片时间轴上的绝对结束点），<=0 到结尾
	Crop      *CropRect
	Rotate    Rotation
	// Reverse 倒放（画面+声音）。reverse 滤镜会把整段缓冲在内存，
	// 长片段（几分钟以上）慎用
	Reverse bool
	// Speed 变速倍率，0/1=原速，有效范围 0.5~2.0（超出自动钳制）；
	// 视频 setpts、音频 atempo，音画同步
	Speed float64
	// Volume 该路音量倍率（<=0 或 1=原样）；Mute 优先于 Volume
	Volume float64
	Mute   bool
}

// OverlayLayer 叠加小窗层：排在主画面（拼接/网格结果）之上。
// Overlays 数组顺序即层序，靠后的层在上。
type OverlayLayer struct {
	Clip ClipSpec
	X, Y int // 小窗左上角在成片中的位置（像素）
	W, H int // 目标尺寸；只给其一时另一边等比（-2），都 0=原尺寸
	// Fit W/H 都给定时（固定矩形）源画面的适配方式，零值 = FitCrop。
	// 只给一边时高度/宽度是等比推出来的，天然不变形，Fit 不生效。
	Fit      FitMode
	Opacity  float64 // 0~1，1=不透明
	From, To float64 // 成片时间轴上的显示时间窗（秒），<=0 表示对应边界不限制
}

// AudioSpec 成片音频策略：保留哪些主输入的声音 + 可选外挂配乐。
// Keep 的 Index 对应 Clips（不含叠加层）。Sequential 布局下保留的声音
// 按段时间线先后播放（未保留的段补静音对齐）；Grid 布局下同时播放混合。
type AudioSpec struct {
	Keep      []AudioPick // nil=全部保留
	BGMPath   string      // 外挂配乐，与保留音轨混合；无保留音轨时即纯配乐
	BGMVolume float64     // 配乐音量（<=0 或 1=原样）
	BGMLoop   bool        // 配乐短于成片时循环铺满
}

// MultiComposeOptions 通用合成参数，零值均为合理默认。
type MultiComposeOptions struct {
	Clips []ClipSpec // 主输入，至少 1 路

	Layout      ComposeLayout
	Transitions []Transition // Sequential：相邻段转场（len-1，缺省处硬切）
	Cols, Rows  int          // Grid：行列；只给其一时另一个按数量自动；都 0=单行
	PadColor    string       // Grid：空格填充色（默认 black）

	Overlays  []OverlayLayer // 主画面之上的叠加小窗（任意布局可用）
	Watermark *Watermark     // 整幅水印（叠加层之上）
	Subtitle  string         // 字幕文件（srt/ass，最后渲染）

	Audio AudioSpec

	Width, Height int     // 输出尺寸，0=自动取所有主输入的最大宽高（取偶）
	Fps           float64 // 0=首路帧率
	// Fit 主输入宽高比与目标不一致时的适配方式（见 FitMode），
	// 零值 = FitCrop（裁剪填满，不变形）。2026-09 之前固定为拉伸变形。
	Fit FitMode
	Enc EncodeOptions
}

// MultiCompose 按参数把多路输入合成为一个视频（一次编码）。
func (f *FFmpeg) MultiCompose(opts MultiComposeOptions, output string) error {
	n := len(opts.Clips)
	if n < 1 {
		return fmt.Errorf("至少需要一路主输入")
	}
	// 归一参数
	for i := range opts.Clips {
		c := &opts.Clips[i]
		if c.Speed <= 0 || math.Abs(c.Speed-1) < 1e-9 {
			c.Speed = 1
		}
		if c.Speed < 0.5 {
			c.Speed = 0.5
		}
		if c.Speed > 2 {
			c.Speed = 2
		}
		if c.Volume <= 0 {
			c.Volume = 1
		}
	}
	for k := range opts.Overlays {
		ov := &opts.Overlays[k]
		s := &ov.Clip
		if s.Speed <= 0 || math.Abs(s.Speed-1) < 1e-9 {
			s.Speed = 1
		}
		if s.Speed < 0.5 {
			s.Speed = 0.5
		}
		if s.Speed > 2 {
			s.Speed = 2
		}
		if s.Volume <= 0 {
			s.Volume = 1
		}
	}

	// 探测主输入，计算预处理后的有效时长
	infos := make([]*ProbeResult, n)
	maxW, maxH, fps := 0, 0, opts.Fps
	for i := range opts.Clips {
		info, err := f.Probe(opts.Clips[i].Path)
		if err != nil {
			return fmt.Errorf("探测 %s 失败: %w", opts.Clips[i].Path, err)
		}
		if !info.HasVideo {
			return fmt.Errorf("%s 没有视频流", opts.Clips[i].Path)
		}
		infos[i] = info
		if info.Video.Width > maxW {
			maxW = info.Video.Width
		}
		if info.Video.Height > maxH {
			maxH = info.Video.Height
		}
		if fps <= 0 {
			fps = info.Video.FrameRate
			if fps <= 0 || fps > 240 {
				fps = 30
			}
		}
	}

	// Grid 行列归一（尺寸计算前）
	cols, rows := opts.Cols, opts.Rows
	if opts.Layout == LayoutGrid {
		switch {
		case cols > 0 && rows > 0:
		case cols > 0:
			rows = (n + cols - 1) / cols
		case rows > 0:
			cols = (n + rows - 1) / rows
		default:
			cols, rows = n, 1
		}
		if n > cols*rows {
			return fmt.Errorf("输入数量 %d 超过网格容量 %d", n, cols*rows)
		}
	}

	// 输出尺寸语义：Width/Height 为整幅输出。
	//   Sequential：自动 = 最大输入宽高；
	//   Grid：自动 = 格子取最大输入宽高（整幅 = 格子 x 行列）；
	//         显式 = 整幅尺寸按行列均分得到格子。
	var W, H, cellW, cellH int
	if opts.Layout == LayoutGrid {
		cellW, cellH = opts.Width/cols, opts.Height/rows
		if cellW <= 0 {
			cellW = maxW
		}
		if cellH <= 0 {
			cellH = maxH
		}
		cellW &^= 1
		cellH &^= 1
		W, H = cellW*cols, cellH*rows
	} else {
		W, H = opts.Width, opts.Height
		if W <= 0 {
			W = maxW
		}
		if H <= 0 {
			H = maxH
		}
		W &^= 1
		H &^= 1
	}

	// 每路预处理后的时长（trim/变速影响；reverse 不变）
	effDur := make([]float64, n)
	for i := range opts.Clips {
		d := infos[i].Duration
		if opts.Clips[i].TrimStart > 0 {
			d -= opts.Clips[i].TrimStart
		}
		if opts.Clips[i].TrimEnd > opts.Clips[i].TrimStart && opts.Clips[i].TrimEnd < d+opts.Clips[i].TrimStart {
			d = opts.Clips[i].TrimEnd - opts.Clips[i].TrimStart
		}
		if d < 0 {
			d = 0
		}
		effDur[i] = d / opts.Clips[i].Speed
	}
	if effDur[0] <= 0 {
		return fmt.Errorf("第一路输入的有效时长为 0（Trim 区间过小）")
	}

	var inputArgs []string
	var chains []string // filtergraph 分段，最后 join
	var lavfiArgs []string
	lavfiIdx := func() int { return n + len(opts.Overlays) + btoi(opts.Watermark != nil) + btoi(opts.Audio.BGMPath != "") + len(lavfiArgs)/3 }

	// ---- 每路主输入：预处理 + 归一化 ----
	// 归一化目标：Sequential=输出全幅；Grid=格子尺寸
	tw, th := W, H
	if opts.Layout == LayoutGrid {
		tw, th = cellW, cellH
	}
	for i := range opts.Clips {
		inputArgs = append(inputArgs, "-i", opts.Clips[i].Path)
		chains = append(chains, clipVideoChain(i, &opts.Clips[i], tw, th, fps, opts.Fit)+fmt.Sprintf("[c%d]", i))
	}

	// ---- 布局 ----
	var vLabel string
	var totalDur float64
	switch opts.Layout {
	case LayoutGrid:
		maxDur := 0.0
		for _, d := range effDur {
			if d > maxDur {
				maxDur = d
			}
		}
		if maxDur <= 0 {
			maxDur = 1
		}
		totalDur = maxDur
		// 空位黑块（color 源限定时长，否则 stack 遇最长输入永不结束）
		padColor := opts.PadColor
		if padColor == "" {
			padColor = "black"
		}
		for i := 0; i < cols*rows-n; i++ {
			idx := lavfiIdx()
			lavfiArgs = append(lavfiArgs, "-f", "lavfi", "-i",
				fmt.Sprintf("color=%s:s=%dx%d:r=%d:d=%.3f", padColor, tw, th, int(fps), maxDur))
			chains = append(chains, fmt.Sprintf(
				"[%d:v]setpts=PTS-STARTPTS,fps=%d,setsar=1[g%d]", idx, int(fps), n+i))
		}
		var rowLabels []string
		for r := 0; r < rows; r++ {
			var cells []string
			for c := 0; c < cols; c++ {
				k := r*cols + c
				if k < n {
					cells = append(cells, fmt.Sprintf("[c%d]", k))
				} else {
					cells = append(cells, fmt.Sprintf("[g%d]", k))
				}
			}
			if cols == 1 {
				rowLabels = append(rowLabels, cells[0])
				continue
			}
			chains = append(chains, strings.Join(cells, "")+fmt.Sprintf("hstack=inputs=%d[r%d]", cols, r))
			rowLabels = append(rowLabels, fmt.Sprintf("[r%d]", r))
		}
		if rows == 1 {
			vLabel = rowLabels[0]
		} else {
			chains = append(chains, strings.Join(rowLabels, "")+fmt.Sprintf("vstack=inputs=%d[v0]", rows))
			vLabel = "[v0]"
		}
	default: // Sequential
		cumulative := 0.0
		prev := "[c0]"
		for i := 0; i < n; i++ {
			vis := effDur[i]
			if i < n-1 {
				if t := transitionAt(opts.Transitions, i); t.Type != "" && t.Duration > 0 {
					vis -= t.Duration
				}
			}
			totalDur += vis
			if i == n-1 {
				break
			}
			tr := transitionAt(opts.Transitions, i)
			next := fmt.Sprintf("[x%d]", i+1)
			if tr.Type != "" && tr.Duration > 0 {
				offset := cumulative + effDur[i] - tr.Duration
				chains = append(chains, fmt.Sprintf(
					"%s[c%d]xfade=transition=%s:duration=%.3f:offset=%.3f%s",
					prev, i+1, tr.Type, tr.Duration, offset, next))
				cumulative += effDur[i] - tr.Duration
			} else {
				chains = append(chains, fmt.Sprintf("%s[c%d]concat=n=2:v=1:a=0%s", prev, i+1, next))
				cumulative += effDur[i]
			}
			prev = next
		}
		vLabel = prev
	}

	// ---- 叠加小窗层 ----
	for k, ov := range opts.Overlays {
		idx := n + k
		inputArgs = append(inputArgs, "-i", ov.Clip.Path)
		chains = append(chains, overlayChain(idx, &ov)+fmt.Sprintf("[o%d]", k))
		enable := ""
		if ov.From > 0 || ov.To > 0 {
			from, to := ov.From, ov.To
			if to <= 0 {
				to = 1 << 20
			}
			enable = fmt.Sprintf(":enable='between(t,%.3f,%.3f)'", from, to)
		}
		next := fmt.Sprintf("[v%d]", k+1)
		chains = append(chains, fmt.Sprintf("%s[o%d]overlay=%d:%d%s%s", vLabel, k, ov.X, ov.Y, enable, next))
		vLabel = next
	}

	// ---- 水印 ----
	if wm := opts.Watermark; wm != nil && wm.Path != "" {
		idx := n + len(opts.Overlays)
		inputArgs = append(inputArgs, "-i", wm.Path)
		var b strings.Builder
		b.WriteString(fmt.Sprintf("[%d:v]", idx))
		if wm.Opacity > 0 && wm.Opacity < 1 {
			b.WriteString(fmt.Sprintf("format=rgba,colorchannelmixer=aa=%.2f", wm.Opacity))
		}
		b.WriteString("[w]")
		chains = append(chains, b.String())
		next := fmt.Sprintf("[v%d]", len(opts.Overlays)+1)
		chains = append(chains, fmt.Sprintf("%s[w]overlay=%s%s", vLabel, wm.overlayExpr(), next))
		vLabel = next
	}

	// ---- 字幕（最后渲染，不会被裁掉） ----
	if opts.Subtitle != "" {
		chains = append(chains, fmt.Sprintf("%ssubtitles='%s'[vfin]", vLabel, subtitlePathExpr(opts.Subtitle)))
		vLabel = "[vfin]"
	}

	// ---- 音频 ----
	// keepAudio[i]：该路是否参与成片声音
	keep := make([]bool, n)
	if opts.Audio.Keep == nil {
		for i := range keep {
			keep[i] = true
		}
	} else {
		for _, p := range opts.Audio.Keep {
			if p.Index >= 0 && p.Index < n {
				keep[p.Index] = true
			}
		}
	}
	hasKept := false
	for i := range keep {
		if keep[i] && infos[i].HasAudio && !opts.Clips[i].Mute {
			hasKept = true
		}
	}

	var audioMap []string // 为空表示 -an
	switch {
	case !hasKept && opts.Audio.BGMPath == "":
		audioMap = nil
	case opts.Layout == LayoutGrid && hasKept:
		// 同屏混音：保留路归一后 amix。旧版（<4.4 无 normalize）各路
		// 音量预乘路数抵消默认的 1/N 衰减
		var inputs []string
		keptTotal := 0
		for i := range opts.Clips {
			if keep[i] && infos[i].HasAudio && !opts.Clips[i].Mute {
				keptTotal++
			}
		}
		boost := 1.0
		if !f.HasAmixNormalize() {
			boost = float64(keptTotal)
		}
		for i := range opts.Clips {
			if !keep[i] || !infos[i].HasAudio || opts.Clips[i].Mute {
				continue
			}
			cs := opts.Clips[i]
			cs.Volume *= boost
			chains = append(chains, clipAudioChain(i, &cs)+fmt.Sprintf("[a%d]", i))
			inputs = append(inputs, fmt.Sprintf("[a%d]", i))
		}
		chains = append(chains, strings.Join(inputs, "")+
			fmt.Sprintf("amix=inputs=%d:duration=longest%s[a]", len(inputs), f.amixNormalizeSuffix()))
		audioMap = []string{"-map", "[a]"}
	default:
		// Sequential 的时间线拼接（Grid 无保留路时也走静音轨垫底）。
		// 每段音轨必须与该段可视时长严格相等（长了裁、短了补静音），
		// 否则每过一个转场音画错位就累积一次
		var inputs []string
		for i := range opts.Clips {
			vis := effDur[i]
			if i < n-1 {
				if t := transitionAt(opts.Transitions, i); t.Type != "" && t.Duration > 0 {
					vis -= t.Duration
				}
			}
			if vis < 0 {
				vis = 0
			}
			if !hasKept || !keep[i] || !infos[i].HasAudio || opts.Clips[i].Mute {
				// 不出声的段：补同时长静音，保证时间线对齐
				idx := lavfiIdx()
				lavfiArgs = append(lavfiArgs, "-f", "lavfi", "-i",
					fmt.Sprintf("anullsrc=r=44100:cl=stereo:d=%.3f", vis))
				inputs = append(inputs, fmt.Sprintf("[%d:a]", idx))
				continue
			}
			// 该路预处理后的音频时长（音轨时长为准，容器时长兜底/封顶）
			c := &opts.Clips[i]
			ad := infos[i].Duration
			if infos[i].Audio != nil && infos[i].Audio.Duration > 0 {
				ad = infos[i].Audio.Duration
				if ad > infos[i].Duration {
					ad = infos[i].Duration
				}
			}
			if c.TrimStart > 0 {
				ad -= c.TrimStart
			}
			if c.TrimEnd > c.TrimStart && c.TrimEnd-c.TrimStart < ad {
				ad = c.TrimEnd - c.TrimStart
			}
			if ad < 0 {
				ad = 0
			}
			ad /= c.Speed
			chain := clipAudioChain(i, c)
			if ad > vis+0.001 {
				chains = append(chains, chain+fmt.Sprintf(",atrim=end=%.3f,asetpts=PTS-STARTPTS[a%d]", vis, i))
				inputs = append(inputs, fmt.Sprintf("[a%d]", i))
			} else {
				chains = append(chains, chain+fmt.Sprintf("[a%d]", i))
				inputs = append(inputs, fmt.Sprintf("[a%d]", i))
				if pad := vis - ad; pad > 0.001 {
					chains = append(chains, fmt.Sprintf("anullsrc=r=44100:cl=stereo:d=%.3f[s%d]", pad, i))
					inputs = append(inputs, fmt.Sprintf("[s%d]", i))
				}
			}
		}
		chains = append(chains, strings.Join(inputs, "")+
			fmt.Sprintf("concat=n=%d:v=0:a=1[a]", len(inputs)))
		audioMap = []string{"-map", "[a]"}
	}

	// ---- 外挂配乐 ----
	if bgm := opts.Audio.BGMPath; bgm != "" {
		bgmIdx := n + len(opts.Overlays) + btoi(opts.Watermark != nil)
		if opts.Audio.BGMLoop {
			inputArgs = append(inputArgs, "-stream_loop", "-1")
		}
		inputArgs = append(inputArgs, "-i", bgm)
		v := opts.Audio.BGMVolume
		if v <= 0 {
			v = 1
		}
		// 旧版 amix（<4.4 无 normalize）默认每路乘 1/2：配乐预乘 2 抵消，
		// 混音分支的基础轨 [a] 同样加 2 倍增益链
		bgmBoost := 1.0
		if !f.HasAmixNormalize() {
			bgmBoost = 2
		}
		chains = append(chains, fmt.Sprintf(
			"[%d:a]aresample=44100,aformat=sample_fmts=fltp:channel_layouts=stereo", bgmIdx))
		if v*bgmBoost != 1 {
			chains[len(chains)-1] += fmt.Sprintf(",volume=%.2f", v*bgmBoost)
		}
		chains[len(chains)-1] += "[bgm]"
		if audioMap == nil {
			// 纯配乐：用成片时长的静音轨做 amix 的 first，保证时长精确
			chains = append(chains, fmt.Sprintf("aevalsrc=0:d=%.3f[sa]", totalDur))
			chains = append(chains, "[sa][bgm]amix=inputs=2:duration=first"+f.amixNormalizeSuffix()+"[a]")
			audioMap = []string{"-map", "[a]"}
		} else if f.HasAmixNormalize() {
			chains = append(chains, "[a][bgm]amix=inputs=2:duration=first:normalize=0[a2]")
			audioMap = []string{"-map", "[a2]"}
		} else {
			chains = append(chains, "[a]volume=2.00[aa]")
			chains = append(chains, "[aa][bgm]amix=inputs=2:duration=first[a2]")
			audioMap = []string{"-map", "[a2]"}
		}
	}

	args := append(inputArgs, lavfiArgs...)
	args = append(args, "-filter_complex", strings.Join(chains, ";"))
	args = append(args, "-map", vLabel)
	if audioMap != nil {
		args = append(args, audioMap...)
	} else {
		args = append(args, "-an")
	}
	args = append(args, opts.Enc.outputArgs()...)
	args = append(args, output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// clipVideoChain 单路视频的预处理+归一化链（不含输出 label）。
// 顺序：trim → crop → rotate → reverse → speed → 归一化(fps/fit/sar/tb)。
func clipVideoChain(idx int, c *ClipSpec, targetW, targetH int, fps float64, fit FitMode) string {
	var parts []string
	if c.TrimStart > 0 || c.TrimEnd > c.TrimStart {
		trim := fmt.Sprintf("trim=start=%.3f", c.TrimStart)
		if c.TrimEnd > c.TrimStart {
			trim += fmt.Sprintf(":end=%.3f", c.TrimEnd)
		}
		parts = append(parts, trim, "setpts=PTS-STARTPTS")
	}
	if c.Crop != nil && c.Crop.W > 0 && c.Crop.H > 0 {
		parts = append(parts, fmt.Sprintf("crop=%d:%d:%d:%d", c.Crop.W, c.Crop.H, c.Crop.X, c.Crop.Y))
	}
	if c.Rotate != "" {
		if r, err := rotateExpr(c.Rotate); err == nil {
			parts = append(parts, r)
		}
	}
	if c.Reverse {
		parts = append(parts, "reverse", "setpts=PTS-STARTPTS")
	}
	if c.Speed != 1 {
		parts = append(parts, fmt.Sprintf("setpts=%.6f*PTS", 1/c.Speed))
	}
	parts = append(parts,
		fmt.Sprintf("fps=%.3f", fps),
		fitFilterChain(targetW, targetH, fit, ""),
		"setsar=1", "settb=AVTB")
	return fmt.Sprintf("[%d:v]%s", idx, strings.Join(parts, ","))
}

// overlayChain 叠加层的预处理链（按层参数缩放/调透明度，不强制归一到输出尺寸）。
func overlayChain(idx int, ov *OverlayLayer) string {
	var parts []string
	c := &ov.Clip
	if c.TrimStart > 0 || c.TrimEnd > c.TrimStart {
		trim := fmt.Sprintf("trim=start=%.3f", c.TrimStart)
		if c.TrimEnd > c.TrimStart {
			trim += fmt.Sprintf(":end=%.3f", c.TrimEnd)
		}
		parts = append(parts, trim, "setpts=PTS-STARTPTS")
	}
	if c.Crop != nil && c.Crop.W > 0 && c.Crop.H > 0 {
		parts = append(parts, fmt.Sprintf("crop=%d:%d:%d:%d", c.Crop.W, c.Crop.H, c.Crop.X, c.Crop.Y))
	}
	if c.Rotate != "" {
		if r, err := rotateExpr(c.Rotate); err == nil {
			parts = append(parts, r)
		}
	}
	if c.Reverse {
		parts = append(parts, "reverse", "setpts=PTS-STARTPTS")
	}
	if c.Speed != 1 {
		parts = append(parts, fmt.Sprintf("setpts=%.6f*PTS", 1/c.Speed))
	}
	switch {
	case ov.W > 0 && ov.H > 0:
		// 固定矩形：宽高都给定时源画面会被塞进这个框，需要适配模式
		parts = append(parts, fitFilterChain(ov.W, ov.H, ov.Fit, ""), "setsar=1")
	case ov.W > 0:
		parts = append(parts, fmt.Sprintf("scale=%d:-2", ov.W))
	case ov.H > 0:
		parts = append(parts, fmt.Sprintf("scale=-2:%d", ov.H))
	}
	if ov.Opacity > 0 && ov.Opacity < 1 {
		parts = append(parts, fmt.Sprintf("format=rgba,colorchannelmixer=aa=%.2f", ov.Opacity))
	}
	return fmt.Sprintf("[%d:v]%s", idx, strings.Join(parts, ","))
}

// clipAudioChain 单路音频的预处理链（不含输出 label）：
// trim → areverse → atempo(变速) → volume → 采样格式归一。
// 段时长对齐（裁长补短）由调用方处理。
func clipAudioChain(idx int, c *ClipSpec) string {
	var parts []string
	if c.TrimStart > 0 || c.TrimEnd > c.TrimStart {
		trim := fmt.Sprintf("atrim=start=%.3f", c.TrimStart)
		if c.TrimEnd > c.TrimStart {
			trim += fmt.Sprintf(":end=%.3f", c.TrimEnd)
		}
		parts = append(parts, trim, "asetpts=PTS-STARTPTS")
	}
	if c.Reverse {
		parts = append(parts, "areverse", "asetpts=PTS-STARTPTS")
	}
	if c.Speed != 1 {
		parts = append(parts, fmt.Sprintf("atempo=%.6f", c.Speed))
	}
	if c.Volume > 0 && c.Volume != 1 {
		parts = append(parts, fmt.Sprintf("volume=%.2f", c.Volume))
	}
	parts = append(parts, "aresample=44100", "aformat=sample_fmts=fltp:channel_layouts=stereo")
	return fmt.Sprintf("[%d:a]%s", idx, strings.Join(parts, ","))
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
