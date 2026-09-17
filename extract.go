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

// ExtractFrames 导出 [start,end) 时间段内的帧序列图片。
// pattern 为输出文件名模板（如 out/frame_%04d.jpg，格式由扩展名决定）。
// stepPerSec 为采样频率：>0 时每秒取 stepPerSec 帧（如 1 = 每秒 1 张），
// <=0 时导出该时间段内的每一帧（原帧率全量）。
// 返回实际写出的图片数量。
func (f *FFmpeg) ExtractFrames(input string, start, end, stepPerSec float64, pattern string) (int, error) {
	if end > 0 && end <= start {
		return 0, fmt.Errorf("非法时间段 [%.3f, %.3f)", start, end)
	}
	if !strings.Contains(pattern, "%") {
		return 0, fmt.Errorf("输出模板必须包含序号占位符（如 frame_%%04d.jpg）: %s", pattern)
	}

	args := []string{"-i", input}
	if start > 0 {
		// 精确 seek 放 -i 后（帧精确，帧序列场景短片段居多）
		args = append(args, "-ss", fmt.Sprintf("%.3f", start))
	}
	if end > 0 {
		args = append(args, "-t", fmt.Sprintf("%.3f", end-start))
	}
	if stepPerSec > 0 {
		args = append(args, "-vf", fmt.Sprintf("fps=%.3f", stepPerSec))
	}
	// -vsync vfr：按实际帧时间戳写文件，避免重复/丢帧
	args = append(args, "-vsync", "vfr", "-y", pattern)
	if _, err := f.run(f.ffmpegBin(), args); err != nil {
		return 0, err
	}
	return countPattern(pattern), nil
}

// countPattern 统计模板实际落盘的文件数（%04d -> *）。
func countPattern(pattern string) int {
	dir := filepath.Dir(pattern)
	base := filepath.Base(pattern)
	i := strings.Index(base, "%")
	prefix, suffix := base[:i], base[i:]
	for strings.HasPrefix(suffix, "%") {
		suffix = suffix[strings.IndexAny(suffix, "dix")+1:]
	}
	matches, _ := filepath.Glob(filepath.Join(dir, prefix+"*"+suffix))
	n := 0
	for _, m := range matches {
		if m != pattern { // 排除模板字符串自身误当文件
			n++
		}
	}
	return n
}
