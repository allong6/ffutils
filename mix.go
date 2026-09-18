package ffutils

import (
	"fmt"
	"strings"
)

// MixTrack 描述一路混入视频的音频轨。
type MixTrack struct {
	// Path 音频文件路径
	Path string
	// TrimIn/TrimOut 截取音频的起止时间（秒），0 表示对应边界不裁
	TrimIn  float64
	TrimOut float64
	// StartAt 该轨在成片中的起始时间（秒），默认 0
	StartAt float64
	// Speed 播放倍速 (0.5~2.0)，默认 1
	Speed float64
	// Volume 音量倍率 (0~1]，默认 1
	Volume float64
}

func (t MixTrack) normalized() MixTrack {
	if t.Speed <= 0 {
		t.Speed = 1
	}
	if t.Volume <= 0 {
		t.Volume = 1
	}
	return t
}

// MixAudio 将多路音频混入视频：各轨按 Trim 截取、按 Speed 变速、
// 延迟到 StartAt 后以 Volume 混音；总时长短于视频时自动补静音，
// 输出时长以视频为准。
func (f *FFmpeg) MixAudio(video string, tracks []MixTrack, output string, enc EncodeOptions) error {
	if len(tracks) == 0 {
		return fmt.Errorf("至少需要一路音频轨")
	}

	info, err := f.Probe(video)
	if err != nil {
		return fmt.Errorf("探测视频失败: %w", err)
	}
	videoDur := info.Duration

	var args []string = []string{"-i", video}
	var filter strings.Builder
	var mixInputs []string

	// amix 旧版（<4.4 无 normalize 选项）默认把每路乘 1/N：预计算总路数
	//（原声 + 各轨 + 可能的补静音），各路音量预乘 N 抵消，保持响度不变
	norm := f.HasAmixNormalize()
	total := len(tracks) + btoi(info.HasAudio)
	mixEndPre := 0.0
	for _, raw := range tracks {
		t := raw.normalized()
		trimEnd := t.TrimOut
		if trimEnd <= 0 {
			trimEnd = 1 << 30
		}
		if end := t.StartAt + (trimEnd-t.TrimIn)/t.Speed; end > mixEndPre {
			mixEndPre = end
		}
	}
	if pad := videoDur - mixEndPre; pad > 0 {
		total++
	}
	boost := 1.0
	if !norm {
		boost = float64(total)
	}

	// 视频原声（如有）作为第 0 路参与混音
	if info.HasAudio {
		if norm {
			mixInputs = append(mixInputs, "[0:a]")
		} else {
			filter.WriteString(fmt.Sprintf("[0:a]volume=%.3f[orig];", boost))
			mixInputs = append(mixInputs, "[orig]")
		}
	}

	mixEnd := 0.0 // 所有音轨（含延迟）结束的最大时刻，用于判断是否需要补静音
	for i, raw := range tracks {
		t := raw.normalized()
		args = append(args, "-i", t.Path)

		// 单轨处理链：atrim 截取片段 -> asetpts 重置时间戳（atrim 后 pts
		// 保留原值，不重置会导致 adelay 计算错误）-> atempo 变速 ->
		// adelay 平移到 StartAt 时刻 -> volume 调节音量。
		// adelay 的单位是毫秒，且需要给足声道数（两路同值覆盖单/双声道）。
		trimEnd := t.TrimOut
		if trimEnd <= 0 {
			trimEnd = 1 << 30 // 未指定则不裁结束
		}
		filter.WriteString(fmt.Sprintf("[%d:a]atrim=%.3f:%.3f,asetpts=PTS-STARTPTS", i+1, t.TrimIn, trimEnd))
		if t.Speed != 1 {
			filter.WriteString(fmt.Sprintf(",atempo=%.3f", t.Speed))
		}
		delayMS := int(t.StartAt * 1000)
		filter.WriteString(fmt.Sprintf(",adelay=%d|%d,volume=%.3f[a%d];", delayMS, delayMS, t.Volume*boost, i+1))
		mixInputs = append(mixInputs, fmt.Sprintf("[a%d]", i+1))

		// 该轨在成片中的结束时刻 = 延迟 + 倍速后的有效时长（变速使时长缩为 1/speed）
		trackDur := (trimEnd - t.TrimIn) / t.Speed
		if end := t.StartAt + trackDur; end > mixEnd {
			mixEnd = end
		}
	}

	// amix 的时长策略是"最长输入决定"，若所有音轨都比视频短，成片音轨会
	// 提前结束；补一段覆盖到视频结尾的静音，保证音轨贯穿全片。
	// normalize=0 保持各轨音量不自动衰减。
	if pad := videoDur - mixEnd; pad > 0 {
		filter.WriteString(fmt.Sprintf("aevalsrc=0:d=%.3f[silent];", pad))
		mixInputs = append(mixInputs, "[silent]")
	}

	filter.WriteString(strings.Join(mixInputs, ""))
	filter.WriteString(fmt.Sprintf("amix=inputs=%d%s[outa]", len(mixInputs), f.amixNormalizeSuffix()))
	// 统一采样率，避免不同源采样率导致的加速/降速问题
	filter.WriteString(";[outa]aformat=sample_rates=44100[outa]")
	filterStr := filter.String()

	args = append(args,
		"-filter_complex", filterStr,
		"-map", "0:v", "-map", "[outa]",
		"-t", fmt.Sprintf("%.3f", videoDur),
	)
	args = append(args, enc.outputArgs()...)
	args = append(args, output)
	_, err = f.run(f.ffmpegBin(), args)
	return err
}
// MixBackground 背景音乐混入：保留视频原声（音量 mainVol）并与 bgm
// （音量 bgmVol，loop=true 时循环铺满）同时播放混合；视频无原声时
// 等价于纯配乐。输出时长以视频为准。与 ReplaceAudio 的区别：后者
// 丢弃原声，本方法混合保留。
func (f *FFmpeg) MixBackground(video, bgm string, mainVol, bgmVol float64, loop bool, output string, enc EncodeOptions) error {
	if mainVol <= 0 {
		mainVol = 1
	}
	if bgmVol <= 0 {
		bgmVol = 1
	}
	info, err := f.Probe(video)
	if err != nil {
		return fmt.Errorf("探测视频失败: %w", err)
	}

	args := []string{"-i", video}
	if loop {
		// demuxer 层循环 + 输出侧 -t 截断，配 amix duration=first
		args = append(args, "-stream_loop", "-1")
	}
	args = append(args, "-i", bgm)

	var filter string
	if info.HasAudio {
		// 旧版 amix（无 normalize）默认每路乘 1/2：两路各预乘 2 抵消
		if !f.HasAmixNormalize() {
			mainVol, bgmVol = mainVol*2, bgmVol*2
		}
		filter = fmt.Sprintf(
			"[0:a]volume=%.3f[ma];[1:a]volume=%.3f[ba];"+
				"[ma][ba]amix=inputs=2:duration=first%s[a]",
			mainVol, bgmVol, f.amixNormalizeSuffix())
	} else {
		filter = fmt.Sprintf("[1:a]volume=%.3f[a]", bgmVol)
	}
	args = append(args,
		"-filter_complex", filter,
		"-map", "0:v", "-map", "[a]",
	)
	if info.Duration > 0 {
		args = append(args, "-t", fmt.Sprintf("%.3f", info.Duration))
	}
	args = append(args, enc.outputArgs()...)
	args = append(args, output)
	_, err = f.run(f.ffmpegBin(), args)
	return err
}
