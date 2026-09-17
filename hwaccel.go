package ffutils

// hwCandidates 待探测的硬件编码器（Windows 常见三家）。
var hwCandidates = []struct {
	codec VideoCodec
	label string
}{
	{VideoNVENC, "NVIDIA 显卡（NVENC）"},
	{VideoNVENC26, "NVIDIA 显卡（NVENC HEVC）"},
	{VideoQSV, "Intel 核显（QSV）"},
	{VideoAMF, "AMD 显卡（AMF）"},
}

// HardwareEncoder 一个可用的硬件编码器。
type HardwareEncoder struct {
	Codec VideoCodec `json:"codec"`
	Label string     `json:"label"`
}

// DetectHardwareEncoders 探测本机可用的硬件编码器：对每个候选跑一次
// 0.3 秒的 testsrc 试编码，成功即认为可用。返回可用列表（可能为空，
// 表示仅软编码）。总耗时约 1~2 秒，适合在进入专业模式时调用一次并缓存。
func (f *FFmpeg) DetectHardwareEncoders() []HardwareEncoder {
	var out []HardwareEncoder
	for _, c := range hwCandidates {
		args := []string{
			"-f", "lavfi", "-i", "testsrc=duration=0.3:size=320x240:rate=10",
			"-c:v", string(c.codec), "-f", "null", "-",
		}
		if _, err := f.run(f.ffmpegBin(), args); err == nil {
			out = append(out, HardwareEncoder{Codec: c.codec, Label: c.label})
		}
	}
	return out
}
