package ffutils

import (
	"strconv"
	"strings"
)

// EncodeOptions 描述输出编码参数，零值字段使用默认值。
type EncodeOptions struct {
	// VideoCodec 视频编码器，默认 libx264；设为 "copy" 表示流式拷贝不重编码
	VideoCodec string
	// Preset 编码速度预设，默认 ultrafast
	Preset string
	// PixFmt 像素格式，默认 yuv420p
	PixFmt string
	// AudioCodec 音频编码器，默认 aac；设为 "copy" 表示不重编码
	AudioCodec string
	// AudioRate 音频采样率，默认 44100；<=0 时输出 aformat 转换
	AudioRate int
	// CRF 恒定质量，0 表示不指定
	CRF int
	// ExtraArgs 逃生舱：追加到命令末尾（输出文件之前）的任意参数
	ExtraArgs []string
}

func (o EncodeOptions) videoCodec() string  { return orDefault(o.VideoCodec, "libx264") }
func (o EncodeOptions) preset() string      { return orDefault(o.Preset, "ultrafast") }
func (o EncodeOptions) pixFmt() string      { return orDefault(o.PixFmt, "yuv420p") }
func (o EncodeOptions) audioRate() int      { if o.AudioRate > 0 { return o.AudioRate }; return 44100 }
func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// outputArgs 生成通用的输出参数段（编码 + 覆盖）。
// needVideoAudioFilter 表示是否需要在 filter 中完成 aformat（amix 等场景），
// false 时由这里追加 -ar 参数。
func (o EncodeOptions) outputArgs() []string {
	args := make([]string, 0, 16)
	if o.VideoCodec == "copy" {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, "-c:v", o.videoCodec())
		if o.videoCodec() == "libx264" {
			args = append(args, "-preset", o.preset())
			if o.CRF > 0 {
				args = append(args, "-crf", strconv.Itoa(o.CRF))
			}
		}
		args = append(args, "-pix_fmt", o.pixFmt())
	}
	if o.AudioCodec == "copy" {
		args = append(args, "-c:a", "copy")
	} else {
		args = append(args, "-c:a", orDefault(o.AudioCodec, "aac"), "-ar", strconv.Itoa(o.audioRate()))
	}
	args = append(args, o.ExtraArgs...)
	args = append(args, "-y")
	return args
}
