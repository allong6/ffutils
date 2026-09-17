package ffutils

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// ExtractFrame 抽取指定时间点的一帧作为图片（封面图等场景）。
// at 为秒；at<=0 取首帧。输出格式由图片扩展名决定（jpg/png）。
func (f *FFmpeg) ExtractFrame(path string, at float64, output string) error {
	args := []string{"-i", path}
	if at > 0 {
		// -ss 放在 -i 前可快速定位；这里放在 -i 后保证帧精确
		args = append(args, "-ss", fmt.Sprintf("%.3f", at))
	}
	args = append(args, "-frames:v", "1", "-y", output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// ExtractCover 抽取适合做封面的帧：时长超过 1s 取第 1 秒处，否则取首帧。
func (f *FFmpeg) ExtractCover(path, output string) error {
	info, err := f.Probe(path)
	if err != nil {
		return err
	}
	at := 0.0
	if info.Duration > 1 {
		at = 1
	}
	return f.ExtractFrame(path, at, output)
}

// AudioExtractOptions 音频抽取参数，零值使用默认。
type AudioExtractOptions struct {
	// Codec 音频编码器，见 AudioCodec 常量组（AudioMP3/AudioAAC/AudioPCM 等），
	// 默认按输出扩展名推断（mp3->AudioMP3, m4a/aac->AudioAAC, wav->AudioPCM）
	Codec AudioCodec
	// Bitrate 音频码率如 "192k"，空则用编码器默认
	Bitrate string
	// SampleRate 采样率，<=0 不指定
	SampleRate int
}

func (o AudioExtractOptions) codecFor(output string) string {
	if o.Codec != "" {
		return string(o.Codec)
	}
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(output), ".")) {
	case "mp3":
		return string(AudioMP3)
	case "m4a", "aac":
		return string(AudioAAC)
	case "wav":
		return string(AudioPCM)
	default:
		return string(AudioMP3)
	}
}

// ExtractAudio 从媒体文件抽取音频流并转码输出。
func (f *FFmpeg) ExtractAudio(path string, opts AudioExtractOptions, output string) error {
	args := []string{"-i", path, "-vn", "-c:a", opts.codecFor(output)}
	if opts.Bitrate != "" {
		args = append(args, "-b:a", opts.Bitrate)
	}
	if opts.SampleRate > 0 {
		args = append(args, "-ar", strconv.Itoa(opts.SampleRate))
	}
	args = append(args, "-y", output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}
