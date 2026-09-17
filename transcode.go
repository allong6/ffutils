package ffutils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TranscodeOptions 转码/转格式参数，零值字段表示保持原样或使用默认。
type TranscodeOptions struct {
	// TrimStart/TrimEnd 只处理该时间范围内的内容（秒）；TrimEnd<=0 表示到结尾。
	// 转码场景下基于 -ss/-t 放在 -i 之前做快速 seek。
	TrimStart float64
	TrimEnd   float64

	// Width/Height 目标分辨率，<=0 表示保持原始值；两者都指定时可能改变宽高比，
	// 只指定其一时另一边按宽高比自动计算（并取偶）。
	Width  int
	Height int

	// Fps 目标帧率，<=0 保持原始
	Fps float64

	// VideoCodec 视频编码器，见 VideoCodec 常量组；空则按输出扩展名推断
	// （mp4/mkv->libx264, webm->libvpx-vp9），VideoCopy 表示视频流不重编码
	VideoCodec VideoCodec
	// VideoBitrate 目标视频码率（如 "2M"），与 CRF 二选一，同时设置时优先 CRF
	VideoBitrate string
	// CRF 恒定质量（x264/x265 建议 18~28，越小质量越高），0 表示不启用
	CRF int
	// Preset 编码速度预设，见 Preset 常量组；默认 PresetMedium
	// （转码默认比剪辑类更注重画质）
	Preset Preset

	// AudioCodec 音频编码器，见 AudioCodec 常量组；空则默认 aac（webm 为
	// libopus），AudioCopy 表示不重编码
	AudioCodec AudioCodec
	// AudioBitrate 目标音频码率（如 "128k"）
	AudioBitrate string
	// Mute 去掉音轨
	Mute bool

	// ExtraArgs 逃生舱：追加任意 ffmpeg 参数（输出文件之前）
	ExtraArgs []string

	// OnProgress 进度回调（可选）。设置后转码过程会以约 1 次/秒的频率
	// 回调进度（含百分比），GUI/CLI 可用于渲染进度条。回调返回错误会取消转码。
	OnProgress ProgressFunc
}

// defaultCodecFor 根据输出扩展名推断默认编码器。
func defaultCodecFor(output string) (video, audio string) {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(output), ".")) {
	case "webm":
		return "libvpx-vp9", "libopus"
	case "ogv":
		return "libtheora", "libvorbis"
	case "mp4", "m4v", "mov":
		return "libx264", "aac"
	case "mkv", "avi", "flv", "ts":
		return "libx264", "aac"
	case "gif":
		return "gif", ""
	default:
		return "libx264", "aac"
	}
}

// transcodeArgs 依据 opts 构建完整的转码参数（不含 -y 与输出路径）。
// scale 表达式：只指定一边时另一边按宽高比自适应（-2 表示自动计算并保证偶数）。
func (o TranscodeOptions) transcodeArgs(input, output string) []string {
	vcodec, acodec := string(o.VideoCodec), string(o.AudioCodec)
	if vcodec == "" || acodec == "" {
		dv, da := defaultCodecFor(output)
		if vcodec == "" {
			vcodec = dv
		}
		if acodec == "" {
			acodec = da
		}
	}

	var args []string

	// seek 段：放在 -i 前为快速 seek（关键帧定位，转码时再精确解码）
	if o.TrimStart > 0 {
		args = append(args, "-ss", fmt.Sprintf("%.3f", o.TrimStart))
	}
	args = append(args, "-i", input)
	if o.TrimStart > 0 && o.TrimEnd > o.TrimStart {
		args = append(args, "-t", fmt.Sprintf("%.3f", o.TrimEnd-o.TrimStart))
	} else if o.TrimEnd > 0 && o.TrimStart <= 0 {
		args = append(args, "-t", fmt.Sprintf("%.3f", o.TrimEnd))
	}

	// 视频滤镜：缩放 / 帧率
	var vf []string
	if o.Width > 0 && o.Height > 0 {
		vf = append(vf, fmt.Sprintf("scale=%d:%d", o.Width, o.Height))
	} else if o.Width > 0 {
		vf = append(vf, fmt.Sprintf("scale=%d:-2", o.Width))
	} else if o.Height > 0 {
		vf = append(vf, fmt.Sprintf("scale=-2:%d", o.Height))
	}
	if o.Fps > 0 {
		vf = append(vf, fmt.Sprintf("fps=%.3f", o.Fps))
	}
	if len(vf) > 0 && vcodec != "copy" {
		args = append(args, "-vf", strings.Join(vf, ","))
	}

	// 视频编码段
	if vcodec == "copy" {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, "-c:v", vcodec)
		if vcodec == "libx264" || vcodec == "libx265" {
			args = append(args, "-preset", orDefault(string(o.Preset), string(PresetMedium)))
			if o.CRF > 0 {
				args = append(args, "-crf", itoa(o.CRF))
			} else if o.VideoBitrate != "" {
				args = append(args, "-b:v", o.VideoBitrate)
			}
		} else if o.VideoBitrate != "" {
			args = append(args, "-b:v", o.VideoBitrate)
		}
		// webm/mkv 等封装要求，同时保证兼容各播放器
		args = append(args, "-pix_fmt", "yuv420p")
	}

	// 音频段
	switch {
	case o.Mute:
		args = append(args, "-an")
	case acodec == "copy":
		args = append(args, "-c:a", "copy")
	default:
		args = append(args, "-c:a", acodec)
		if o.AudioBitrate != "" {
			args = append(args, "-b:a", o.AudioBitrate)
		}
	}

	args = append(args, o.ExtraArgs...)
	args = append(args, "-y")
	return args
}

// Transcode 转码/转格式：支持换编码器、改分辨率、改帧率、裁剪时间范围、
// 换封装格式（由输出扩展名决定）等。最通用的单个输入 -> 单个输出方法。
// 设置 opts.OnProgress 可获取转码进度。
func (f *FFmpeg) Transcode(input string, opts TranscodeOptions, output string) error {
	args := opts.transcodeArgs(input, output)
	args = append(args, output)
	if opts.OnProgress == nil {
		_, err := f.run(f.ffmpegBin(), args)
		return err
	}
	// 进度百分比需要总时长（裁剪时用裁剪后的时长）
	total := 0.0
	if info, err := f.Probe(input); err == nil {
		total = info.Duration - opts.TrimStart
		if opts.TrimEnd > opts.TrimStart {
			total = opts.TrimEnd - opts.TrimStart
		}
	}
	_, err := f.runProgress(args, total, opts.OnProgress)
	return err
}

// Remux 仅更换封装格式（如 mp4 -> mkv），音视频流均不重编码，速度极快。
// 注意：目标封装必须支持源编码格式，否则 ffmpeg 会报错（此时应改用 Transcode）。
func (f *FFmpeg) Remux(input, output string) error {
	return f.Transcode(input, TranscodeOptions{VideoCodec: "copy", AudioCodec: "copy"}, output)
}

// Trim 截取时间范围 [start, end)（秒）为新文件，不重编码（流拷贝，关键帧对齐，
// 起点可能略有偏移；需要帧精确时用 Transcode 并设置 TrimStart/TrimEnd）。
func (f *FFmpeg) Trim(input string, start, end float64, output string) error {
	if start < 0 || end <= start {
		return fmt.Errorf("非法的时间范围 [%.3f, %.3f)", start, end)
	}
	args := []string{"-ss", fmt.Sprintf("%.3f", start), "-i", input}
	if end > 0 {
		args = append(args, "-t", fmt.Sprintf("%.3f", end-start))
	}
	args = append(args, "-c", "copy", "-y", output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// Mute 去掉音轨（不重编码视频）。
func (f *FFmpeg) Mute(input, output string) error {
	return f.Transcode(input, TranscodeOptions{VideoCodec: "copy", Mute: true}, output)
}

// ReplaceAudio 用指定音频替换视频的原音轨（不重编码视频）。
// loopAudio 为 true 时音频短于视频则循环铺满。
func (f *FFmpeg) ReplaceAudio(video, audio string, loopAudio bool, output string, enc EncodeOptions) error {
	var args []string = []string{"-i", video, "-i", audio}
	var filter string
	if loopAudio {
		// -stream_loop 要求放在对应 -i 之前；音频循环大数倍保证覆盖全程
		args = []string{"-i", video, "-stream_loop", "-1", "-i", audio}
		filter = "[1:a]aloop=loop=-1:size=2e+09[a]"
	} else {
		filter = "[1:a]anull[a]"
	}
	args = append(args, "-filter_complex", filter,
		"-map", "0:v", "-map", "[a]", "-shortest")
	// 视频流拷贝，音频用 EncodeOptions 的音频参数
	args = append(args, "-c:v", "copy", "-c:a", orDefault(string(enc.AudioCodec), string(AudioAAC)),
		"-ar", itoa(enc.audioRate()), "-y", output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// Speed 调整整条音视频的播放速度（0.5~2.0 之外的倍率可能需要多级 filter，不建议）。
// 视频用 setpts、音频用 atempo，音画同步保持。
func (f *FFmpeg) Speed(input string, speed float64, output string, enc EncodeOptions) error {
	if speed <= 0 || speed == 1 {
		return fmt.Errorf("倍率必须为正且不等于 1: %v", speed)
	}
	filter := fmt.Sprintf("[0:v]setpts=%.6f*PTS[v];[0:a]atempo=%.6f[a]", 1/speed, speed)
	args := []string{"-i", input, "-filter_complex", filter, "-map", "[v]", "-map", "[a]"}
	if enc.VideoCodec == "copy" {
		// 变速必须重编码
		enc.VideoCodec = ""
	}
	args = append(args, enc.outputArgs()...)
	args = append(args, output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// ToGIF 把视频（片段）转为 GIF。palette 使用调色板两步法，颜色明显优于直接转。
func (f *FFmpeg) ToGIF(input string, start, end float64, width int, fps int, output string) error {
	var seek []string
	if start > 0 {
		seek = append(seek, "-ss", fmt.Sprintf("%.3f", start))
	}
	if end > start && end > 0 {
		seek = append(seek, "-t", fmt.Sprintf("%.3f", end-start))
	}
	vf := fmt.Sprintf("fps=%d", orDefaultInt(fps, 15))
	if width > 0 {
		vf += fmt.Sprintf(",scale=%d:-1:flags=lanczos", width)
	}
	// 第一步：生成调色板
	palette := output + ".palette.png"
	args := append(seek, "-i", input, "-vf", vf+",palettegen", "-y", palette)
	if _, err := f.run(f.ffmpegBin(), args); err != nil {
		return err
	}
	defer func() { _ = os.Remove(palette) }()
	// 第二步：用调色板映射颜色
	args = append([]string{}, seek...)
	args = append(args, "-i", input, "-i", palette,
		"-lavfi", vf+" [x]; [x][1:v] paletteuse", "-y", output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func orDefaultInt(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

// TranscodeCommand 返回 Transcode 将执行的完整 ffmpeg 参数串（不含可执行
// 文件路径本身），供 GUI 做"命令预览"。注意：实际执行时若设置了
// OnProgress，还会附加 -nostats -progress pipe:1，预览串中不含这两项。
func (f *FFmpeg) TranscodeCommand(input string, opts TranscodeOptions, output string) string {
	args := opts.transcodeArgs(input, output)
	args = append(args, output)
	return strings.Join(args, " ")
}
