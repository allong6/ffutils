package ffutils

import (
	"fmt"
	"regexp"
	"strconv"
)

// VolumeStats 音量探测结果（volumedetect 滤镜，单位 dB）。
type VolumeStats struct {
	// MeanDb 平均响度（mean_volume），衡量整体音量的常用指标
	MeanDb float64
	// MaxDb 峰值响度（max_volume），超过 0 会削波失真
	MaxDb float64
}

var (
	reMeanVolume = regexp.MustCompile(`mean_volume:\s*(-?[0-9.]+)\s*dB`)
	reMaxVolume  = regexp.MustCompile(`max_volume:\s*(-?[0-9.]+)\s*dB`)
)

// DetectVolume 探测文件音轨的平均/峰值音量（dB）。
// 只解码音频轨（-vn），速度远快于完整转码；无音轨或解析失败时报错。
func (f *FFmpeg) DetectVolume(path string) (VolumeStats, error) {
	out, err := f.run(f.ffmpegBin(), []string{
		"-i", path, "-vn", "-af", "volumedetect", "-f", "null", "-",
	})
	if err != nil {
		return VolumeStats{}, err
	}
	// volumedetect 的统计写在 stderr，run 已合并 stdout/stderr
	vs := VolumeStats{MeanDb: -91, MaxDb: -91}
	meanOK, maxOK := false, false
	if m := reMeanVolume.FindStringSubmatch(out); m != nil {
		if v, perr := strconv.ParseFloat(m[1], 64); perr == nil {
			vs.MeanDb, meanOK = v, true
		}
	}
	if m := reMaxVolume.FindStringSubmatch(out); m != nil {
		if v, perr := strconv.ParseFloat(m[1], 64); perr == nil {
			vs.MaxDb, maxOK = v, true
		}
	}
	if !meanOK && !maxOK {
		return VolumeStats{}, fmt.Errorf("未解析到音量信息（文件可能无音轨）: %s", path)
	}
	return vs, nil
}
