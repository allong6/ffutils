package ffutils

import (
	"os"
	"runtime"
)

// vaapiRenderNode Linux VAAPI 默认渲染设备节点。
const vaapiRenderNode = "/dev/dri/renderD128"

// vaapiUploadFilter VAAPI 试编码的输出侧硬件帧上传滤镜链。
const vaapiUploadFilter = "format=nv12,hwupload"

// hwCandidate 一条硬件编码探测候选：codec 之外还携带平台相关的
// 探测参数差异（VAAPI 需要前置设备参数与输出侧硬件帧滤镜）。
type hwCandidate struct {
	codec   VideoCodec
	label   string
	preArgs []string // 放在 -i 之前的参数（VAAPI 设备声明）
	filter  string   // 放在 -vf 的输出侧滤镜（VAAPI 硬件帧上传）
}

// linuxVAAPIAvailable 报告 Linux VAAPI 默认渲染设备是否存在。
// 无 /dev/dri（典型如无 GPU 服务器）时 VAAPI 全部候选快速跳过。
func linuxVAAPIAvailable() bool {
	_, err := os.Stat(vaapiRenderNode)
	return err == nil
}

// hwCandidatesByPlatform 按 runtime.GOOS 返回探测候选表。
// Windows：NVIDIA NVENC / Intel QSV / AMD AMF；
// Linux：NVENC / QSV + VAAPI（AMD/Intel 开源驱动默认硬编入口），
// AMF 为 Windows-only 技术故不列入；
// 其他平台：NVENC / QSV 兜底。
func hwCandidatesByPlatform() []hwCandidate {
	amf := hwCandidate{codec: VideoAMF, label: "AMD 显卡（AMF）"}
	nvenc := hwCandidate{codec: VideoNVENC, label: "NVIDIA 显卡（NVENC）"}
	nvencHEVC := hwCandidate{codec: VideoNVENC26, label: "NVIDIA 显卡（NVENC HEVC）"}
	qsv := hwCandidate{codec: VideoQSV, label: "Intel 核显（QSV）"}

	switch runtime.GOOS {
	case "windows":
		return []hwCandidate{nvenc, nvencHEVC, qsv, amf}
	case "linux":
		candidates := []hwCandidate{nvenc, nvencHEVC, qsv}
		if linuxVAAPIAvailable() {
			deviceArgs := []string{"-vaapi_device", vaapiRenderNode}
			candidates = append(candidates,
				hwCandidate{
					codec:   VideoVAAPI,
					label:   "AMD/Intel 显卡（VAAPI H.264）",
					preArgs: deviceArgs,
					filter:  vaapiUploadFilter,
				},
				hwCandidate{
					codec:   VideoVAAPI26,
					label:   "AMD/Intel 显卡（VAAPI HEVC）",
					preArgs: deviceArgs,
					filter:  vaapiUploadFilter,
				},
			)
		}
		return candidates
	default:
		return []hwCandidate{nvenc, nvencHEVC, qsv}
	}
}

// HardwareEncoder 一个可用的硬件编码器。
type HardwareEncoder struct {
	Codec VideoCodec `json:"codec"`
	Label string     `json:"label"`
}

// DetectHardwareEncoders 探测本机可用的硬件编码器：对每个候选跑一次
// 0.3 秒的 testsrc 试编码，成功即认为可用。候选表按平台区分：
// Windows 含 NVENC/NVENC HEVC/QSV/AMF；Linux 含 NVENC/NVENC HEVC/QSV
// 与 VAAPI H.264/HEVC（无 /dev/dri/renderD128 时快速跳过 VAAPI），
// AMF 为 Windows-only 不在 Linux 探测。返回可用列表（可能为空，
// 表示仅软编码）。总耗时约 1~2 秒，适合在进入专业模式时调用一次并缓存。
//
// Since v1.6.0 按平台区分候选表（Linux 补 VAAPI、去 AMF）。
func (f *FFmpeg) DetectHardwareEncoders() []HardwareEncoder {
	var out []HardwareEncoder
	for _, c := range hwCandidatesByPlatform() {
		args := append([]string{}, c.preArgs...)
		args = append(args, "-f", "lavfi", "-i", "testsrc=duration=0.3:size=320x240:rate=10")
		if c.filter != "" {
			args = append(args, "-vf", c.filter)
		}
		args = append(args, "-c:v", string(c.codec), "-f", "null", "-")
		if _, err := f.run(f.ffmpegBin(), args); err == nil {
			out = append(out, HardwareEncoder{Codec: c.codec, Label: c.label})
		}
	}
	return out
}
