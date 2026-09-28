package ffutils

import (
	"fmt"
	"strings"
)

// Transition 描述两个相邻片段之间的转场。
type Transition struct {
	// Type xfade 转场类型，见 TransitionType 常量组（Fade/SlideLeft/CircleOpen 等）；
	// 留空（零值）表示该处不做转场，直接硬切拼接。
	Type TransitionType
	// Duration 转场时长（秒），无转场时忽略
	Duration float64
}

// AudioPick 合成/拼接时对某一路输入声音的选择：Index 为输入序号，
// Volume 为音量倍率（1=原样，0.5=减半；<=0 按 1 处理）。
type AudioPick struct {
	Index  int
	Volume float64
}

// XfadeOptions 多视频转场拼接参数。
type XfadeOptions struct {
	// Clips 输入视频路径，至少 2 个，顺序即拼接顺序
	Clips []string
	// Transitions 每两个相邻片段之间的转场，长度应为 len(Clips)-1；
	// 为 nil 时全部硬切；长度不足时缺省处硬切
	Transitions []Transition

	// Audio 指定保留哪些片段的声音及各自音量；nil 表示全部保留（原行为）。
	// 拼接的音频是时间线上先后播放，未选中的片段补静音对齐
	Audio []AudioPick

	// Width/Height 输出分辨率；<=0 时自动取所有片段中的最大宽高（取偶）。
	// xfade 要求所有输入分辨率/帧率一致，内部会对每段做 fps/fit 归一化。
	Width  int
	Height int
	// Fps 输出帧率；<=0 时取第一个片段的帧率（不可信时回退 30）
	Fps float64
	// Fit 片段宽高比与目标分辨率不一致时的适配方式（见 FitMode），
	// 零值 = FitCrop（裁剪填满，不变形）。2026-09 之前固定为拉伸变形，
	// 需要旧观感请显式传 FitStretch。
	Fit FitMode
}

// XfadeConcat 将多个视频带转场地拼接为一个视频，同时把各片段的音频
// 按视频轨对齐后拼接（视频与音频时长不一致时自动补静音）。
func (f *FFmpeg) XfadeConcat(opts XfadeOptions, output string, enc EncodeOptions) error {
	n := len(opts.Clips)
	if n < 2 {
		return fmt.Errorf("至少需要 2 个视频才能转场拼接，收到 %d 个", n)
	}

	// 先探测全部片段，一次探测结果同时用于归一化参数与 offset 计算
	infos := make([]*ProbeResult, n)
	for i, path := range opts.Clips {
		info, err := f.Probe(path)
		if err != nil {
			return fmt.Errorf("探测 %s 失败: %w", path, err)
		}
		if !info.HasVideo {
			return fmt.Errorf("%s 没有视频流", path)
		}
		infos[i] = info
	}

	// xfade 的硬性前提：所有输入的分辨率、帧率、像素格式必须一致，
	// 否则滤镜图初始化直接失败（"Picture size 0x0 is invalid"）。
	// 因此每段都归一化到统一目标参数：
	//   - 目标分辨率默认取所有片段的最大宽高并取偶（编码器要求偶数）；
	//   - 目标帧率默认取第一段的平均帧率。
	width, height := opts.Width, opts.Height
	if width <= 0 || height <= 0 {
		for _, info := range infos {
			if info.Video.Width > width {
				width = info.Video.Width
			}
			if info.Video.Height > height {
				height = info.Video.Height
			}
		}
		width &^= 1 // 编码器要求偶数尺寸
		height &^= 1
	}
	fps := opts.Fps
	if fps <= 0 {
		fps = infos[0].Video.FrameRate
		if fps <= 0 || fps > 240 { // r_frame_rate 偶有 90000 之类的虚标值
			fps = 30
		}
	}

	var inputArgs []string
	var videoChain, audioChain strings.Builder
	var audioInputs []string   // 参与最终音频 concat 的 label，按顺序
	var silentPadArgs []string // 无音轨/未选声音输入补的 anullsrc lavfi 输入
	// 注意：lavfi 输入必须循环结束后统一追加（序号 n+len(silentPadArgs)/3 已按此
	// 预分配）；若插在片段 -i 中间，后续片段的输入序号会后移，[i:v] 会指错流

	// label 命名空间：c%d 归一化后的片段视频轨，x%d 转场/拼接的累计结果，
	// 两套命名避免 ffmpeg 滤镜图中 label 复用冲突
	cumulative := 0.0 // 已消费的可视时长（用于计算 xfade offset）
	prevLabel := "c0"
	for i, path := range opts.Clips {
		info := infos[i]
		videoDur := info.Duration
		audioDur := videoDur
		if info.Audio != nil {
			audioDur = info.Audio.Duration
		}

		inputArgs = append(inputArgs, "-i", path)
		// 归一化链：setpts 重置时间戳起点（转场叠加的前提），fps/fit/setsar
		// 统一帧率、分辨率和像素宽高比，settb 统一时间基避免 xfade offset 计算漂移
		videoChain.WriteString(fmt.Sprintf(
			"[%d:v]setpts=PTS-STARTPTS,fps=%.3f,%s,setsar=1,settb=AVTB[c%d];",
			i, fps, fitFilterChain(width, height, opts.Fit, ""), i))

		trans := transitionAt(opts.Transitions, i)
		if trans.Type != "" && trans.Duration > 0 && !f.HasXfade() {
			// ffmpeg <4.3 无 xfade 滤镜：转场降级为硬切（音画对齐逻辑
			// 随之按硬切计算，保持一致）
			trans = Transition{}
		}
		isLast := i == n-1
		nextLabel := fmt.Sprintf("x%d", i+1)

		// 音轨对齐（转场拼接最易出错的部分）。时间线上：
		//   视频轨：clip_i 占 [start_i, start_i + dur_i)，转场时下一段提前
		//           trans.Duration 进入，即 clip_i 的可视时长是 dur_i - trans；
		//   音频轨：直接 concat 顺序拼接，所以每段音轨必须裁/补到与该段
		//           视频可视时长严格相等（audioTarget），否则每过一个转场
		//           音画错位就累积一次，且成片时长会被较长的音轨撑大。
		audioTarget := videoDur
		if !isLast && trans.Type != "" && trans.Duration > 0 {
			audioTarget -= trans.Duration
		}
		// 声音保留选择：Audio 为 nil 时全部保留（原行为）；否则仅列出的
		// 片段出声，未列出的按无音轨处理（补静音对齐时间线）
		keepAudio := opts.Audio == nil
		vol := 1.0
		for _, p := range opts.Audio {
			if p.Index == i {
				keepAudio, vol = true, p.Volume
				break
			}
		}
		if vol <= 0 {
			vol = 1
		}
		vpre := "" // 音量前置滤镜（倍率非 1 时）
		if vol != 1 {
			vpre = fmt.Sprintf("volume=%.2f,", vol)
		}
		if !info.HasAudio || !keepAudio {
			// 无音轨/未选声音的输入：补一条同时长的静音轨，保证音频 concat 链完整
			// （混合"有音/无音"拼接的常见场景，如去音片段参与拼接）
			silentIdx := n + len(silentPadArgs)/3
			silentPadArgs = append(silentPadArgs, "-f", "lavfi", "-i",
				fmt.Sprintf("anullsrc=r=44100:cl=stereo:d=%.3f", audioTarget))
			audioInputs = append(audioInputs, fmt.Sprintf("[%d:a]", silentIdx))
		} else if pad := audioTarget - audioDur; pad > 0 {
			// 音频比视频轨短：裁到原时长后补静音
			audioChain.WriteString(fmt.Sprintf("[%d:a]%satrim=0:%.3f[a%d];", i, vpre, audioDur, i))
			audioInputs = append(audioInputs, fmt.Sprintf("[a%d]", i))
			audioChain.WriteString(fmt.Sprintf("aevalsrc=0:d=%.3f[s%d];", pad, i))
			audioInputs = append(audioInputs, fmt.Sprintf("[s%d]", i))
		} else {
			// 音频足够长：直接裁到目标时长（含转场重叠）
			end := audioDur + pad
			if end < 0 {
				end = 0
			}
			audioChain.WriteString(fmt.Sprintf("[%d:a]%satrim=0:%.3f[a%d];", i, vpre, end, i))
			audioInputs = append(audioInputs, fmt.Sprintf("[a%d]", i))
		}

		if isLast {
			break
		}

		if trans.Type != "" && trans.Duration > 0 {
			// offset = 当前片段结束点 - 转场时长，即转场在当前片段尾部开始
			offset := cumulative + info.Duration - trans.Duration
			videoChain.WriteString(fmt.Sprintf("[%s][c%d]xfade=transition=%s:duration=%.3f:offset=%.3f[%s];",
				prevLabel, i+1, trans.Type, trans.Duration, offset, nextLabel))
			cumulative += info.Duration - trans.Duration
		} else {
			// 硬切：视频 concat
			videoChain.WriteString(fmt.Sprintf("[%s][c%d]concat=n=2:v=1:a=0[%s];", prevLabel, i+1, nextLabel))
			cumulative += info.Duration
		}
		prevLabel = nextLabel
	}

	// 音频统一拼接：[a0][s0][a1][a2]...concat 成单轨 [a]，
	// 静音段 [sN] 与裁短的音轨交错排列，保证时间线与视频轨一一对应
	filter := videoChain.String() + audioChain.String() +
		strings.Join(audioInputs, "") +
		fmt.Sprintf("concat=n=%d:v=0:a=1[a]", len(audioInputs))

	args := append(append(inputArgs, silentPadArgs...),
		"-filter_complex", filter,
		"-map", "["+prevLabel+"]",
		"-map", "[a]",
	)
	args = append(args, enc.outputArgs()...)
	args = append(args, output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

func transitionAt(ts []Transition, i int) Transition {
	if i < len(ts) {
		return ts[i]
	}
	return Transition{}
}
