package ffutils

import (
	"strconv"
	"strings"
)

// EncodeOptions 描述输出编码参数，零值字段使用默认值。
type EncodeOptions struct {
	// VideoCodec 视频编码器，见 VideoCodec 常量组；默认 VideoH264，
	// 设为 VideoCopy 表示流式拷贝不重编码
	VideoCodec VideoCodec
	// Preset 编码速度预设，见 Preset 常量组；默认 PresetUltrafast
	Preset Preset
	// PixFmt 像素格式，默认 yuv420p
	PixFmt string
	// AudioCodec 音频编码器，见 AudioCodec 常量组；默认 AudioAAC，
	// 设为 AudioCopy 表示不重编码
	AudioCodec AudioCodec
	// AudioRate 音频采样率，默认 44100
	AudioRate int
	// CRF 恒定质量，0 表示不指定
	CRF int
	// ExtraArgs 逃生舱：追加到命令末尾（输出文件之前）的任意参数
	ExtraArgs []string
}

func (o EncodeOptions) videoCodec() VideoCodec {
	return VideoCodec(orDefault(string(o.VideoCodec), string(VideoH264)))
}
func (o EncodeOptions) preset() Preset {
	return Preset(orDefault(string(o.Preset), string(PresetUltrafast)))
}
func (o EncodeOptions) pixFmt() string { return orDefault(o.PixFmt, "yuv420p") }
func (o EncodeOptions) audioRate() int {
	if o.AudioRate > 0 {
		return o.AudioRate
	}
	return 44100
}
func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// outputArgs 生成通用的输出参数段（编码 + 覆盖）。
func (o EncodeOptions) outputArgs() []string {
	args := make([]string, 0, 16)
	if o.VideoCodec == VideoCopy {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, "-c:v", string(o.videoCodec()))
		if o.videoCodec() == VideoH264 {
			args = append(args, "-preset", string(o.preset()))
			if o.CRF > 0 {
				args = append(args, "-crf", strconv.Itoa(o.CRF))
			}
		}
		args = append(args, "-pix_fmt", o.pixFmt())
	}
	if o.AudioCodec == AudioCopy {
		args = append(args, "-c:a", "copy")
	} else {
		args = append(args, "-c:a", orDefault(string(o.AudioCodec), string(AudioAAC)), "-ar", strconv.Itoa(o.audioRate()))
	}
	args = append(args, o.ExtraArgs...)
	args = append(args, "-y")
	return args
}
