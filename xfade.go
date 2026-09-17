package ffutils

import (
	"fmt"
	"strings"
)

// Transition 描述两个相邻片段之间的转场。
type Transition struct {
	// Type xfade 转场类型，如 "fade"、"slideleft"、"circleopen" 等；
	// 留空（零值）表示该处不做转场，直接硬切拼接。
	Type string
	// Duration 转场时长（秒），无转场时忽略
	Duration float64
}

// XfadeOptions 多视频转场拼接参数。
type XfadeOptions struct {
	// Clips 输入视频路径，至少 2 个，顺序即拼接顺序
	Clips []string
	// Transitions 每两个相邻片段之间的转场，长度应为 len(Clips)-1；
	// 为 nil 时全部硬切；长度不足时缺省处硬切
	Transitions []Transition

	// Width/Height 输出分辨率；<=0 时自动取所有片段中的最大宽高（取偶）。
	// xfade 要求所有输入分辨率/帧率一致，内部会对每段做 scale/fps 归一化。
	Width  int
	Height int
	// Fps 输出帧率；<=0 时取第一个片段的帧率（不可信时回退 30）
	Fps float64
}

// XfadeConcat 将多个视频带转场地拼接为一个视频，同时把各片段的音频
// 按视频轨对齐后拼接（视频与音频时长不一致时自动补静音）。
// 这是旧版 VideoXfade 与 Transitions 的合并实现。
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
	var audioInputs []string // 参与最终音频 concat 的 label，按顺序

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
		// 归一化链：setpts 重置时间戳起点（转场叠加的前提），fps/scale/setsar
		// 统一帧率分辨率和宽高比，settb 统一时间基避免 xfade offset 计算漂移
		videoChain.WriteString(fmt.Sprintf(
			"[%d:v]setpts=PTS-STARTPTS,fps=%.3f,scale=%d:%d,setsar=1,settb=AVTB[c%d];",
			i, fps, width, height, i))

		trans := transitionAt(opts.Transitions, i)
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
		if pad := audioTarget - audioDur; pad > 0 {
			// 音频比视频轨短：裁到原时长后补静音
			audioChain.WriteString(fmt.Sprintf("[%d:a]atrim=0:%.3f[a%d];", i, audioDur, i))
			audioInputs = append(audioInputs, fmt.Sprintf("[a%d]", i))
			audioChain.WriteString(fmt.Sprintf("aevalsrc=0:d=%.3f[s%d];", pad, i))
			audioInputs = append(audioInputs, fmt.Sprintf("[s%d]", i))
		} else {
			// 音频足够长：直接裁到目标时长（含转场重叠）
			end := audioDur + pad
			if end < 0 {
				end = 0
			}
			audioChain.WriteString(fmt.Sprintf("[%d:a]atrim=0:%.3f[a%d];", i, end, i))
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

	args := append(inputArgs,
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
