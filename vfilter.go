package ffutils

import (
	"fmt"
	"path/filepath"
	"strings"
)

// CropRect 画面区域裁剪矩形（像素，原点左上角）。
type CropRect struct {
	X, Y, W, H int
}

// VideoFilterChain 一组可叠加的画面处理，ApplyVideoFilters 会把启用的
// 项目合成一条滤镜链，一次编码完成全部（相比逐项多次调用：少几遍
// 重编码、画质无损叠加、速度快数倍）。
//
// 处理顺序固定为：旋转 → 裁剪 → 淡入淡出(视频) → 字幕 → 水印。
// 字幕在裁剪之后渲染避免被裁掉；水印最后叠加保证在最上层。
type VideoFilterChain struct {
	Crop      *CropRect    // nil = 不裁剪
	Rotate    Rotation     // "" = 不旋转
	Watermark *Watermark   // nil = 不加水印；Opacity 字段生效（0~1）
	Subtitle  string       // 字幕文件路径（srt/ass），空 = 不加
	Fade      *FadeOptions // nil = 无淡入淡出（仅 VideoIn/VideoOut 参与视频，AudioIn/Out 参与音频）

	// audioFade 由内部在 Probe 后填充（afade 表达式），调用方无需设置
	audioFade string
}

// ApplyVideoFilters 把 VideoFilterChain 中的全部处理一次性应用到视频。
// 音频轨仅受 Fade 的 AudioIn/AudioOut 影响，其余保持原样。
func (f *FFmpeg) ApplyVideoFilters(input string, chain VideoFilterChain, output string, enc EncodeOptions) error {
	if chain.Crop != nil && (chain.Crop.W <= 0 || chain.Crop.H <= 0) {
		return fmt.Errorf("裁剪宽高必须为正数: %dx%d", chain.Crop.W, chain.Crop.H)
	}

	// 视频滤镜链（按固定顺序拼接：裁剪 → 旋转 → 淡入淡出 → 字幕）。
	// 裁剪必须在旋转之前：CropRect 坐标按原始画面定义（调用方按源分辨率
	// 换算），若先旋转再裁剪，宽高互换后坐标全部错位——ffmpeg 对越界 x/y
	// 静默钳制会裁出错误区域，比例较大时直接报错失败。
	var vf []string
	if chain.Crop != nil {
		vf = append(vf, fmt.Sprintf("crop=%d:%d:%d:%d", chain.Crop.W, chain.Crop.H, chain.Crop.X, chain.Crop.Y))
	}
	if chain.Rotate != "" {
		r, err := rotateExpr(chain.Rotate)
		if err != nil {
			return err
		}
		vf = append(vf, r)
	}
	if chain.Fade != nil && (chain.Fade.VideoIn > 0 || chain.Fade.VideoOut > 0) {
		info, err := f.Probe(input)
		if err != nil {
			return err
		}
		if chain.Fade.VideoIn > 0 {
			vf = append(vf, fmt.Sprintf("fade=t=in:st=0:d=%.3f", chain.Fade.VideoIn))
		}
		if chain.Fade.VideoOut > 0 {
			vf = append(vf, fmt.Sprintf("fade=t=out:st=%.3f:d=%.3f", info.Duration-chain.Fade.VideoOut, chain.Fade.VideoOut))
		}
		chain.audioFade = afadeExpr(info.Duration, chain.Fade)
	}
	if chain.Subtitle != "" {
		vf = append(vf, fmt.Sprintf("subtitles='%s'", subtitlePathExpr(chain.Subtitle)))
	}

	var args []string
	if wm := chain.Watermark; wm != nil && wm.Path != "" {
		// 水印需要第二输入，走 filter_complex：
		//   [0:v] 视频链 [v0]；[1:v] (透明度处理) [w]；[v0][w]overlay [v]
		args = append(args, "-i", input, "-i", wm.Path)
		var b strings.Builder
		b.WriteString("[0:v]")
		if len(vf) > 0 {
			b.WriteString(strings.Join(vf, ","))
		}
		b.WriteString("[v0];[1:v]")
		if wm.Opacity > 0 && wm.Opacity < 1 {
			b.WriteString(fmt.Sprintf("format=rgba,colorchannelmixer=aa=%.2f", wm.Opacity))
		}
		b.WriteString(fmt.Sprintf("[w];[v0][w]overlay=%s[v]", wm.overlayExpr()))
		args = append(args, "-filter_complex", b.String(), "-map", "[v]", "-map", "0:a:0?")
	} else {
		args = append(args, "-i", input)
		if len(vf) > 0 {
			args = append(args, "-vf", strings.Join(vf, ","))
		}
		args = append(args, "-map", "0:v:0", "-map", "0:a:0?")
	}

	if chain.audioFade != "" {
		args = append(args, "-af", chain.audioFade)
	}
	args = append(args, enc.outputArgs()...)
	args = append(args, output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// rotateExpr 旋转/翻转转滤镜表达式（与 Rotate 方法一致）。
func rotateExpr(r Rotation) (string, error) {
	switch r {
	case Rot90CW:
		return "transpose=1", nil
	case Rot90CCW:
		return "transpose=2", nil
	case Rot180:
		return "transpose=1,transpose=1", nil
	case FlipH:
		return "hflip", nil
	case FlipV:
		return "vflip", nil
	default:
		return "", fmt.Errorf("未知旋转类型: %q", string(r))
	}
}

// afadeExpr 音频淡入淡出表达式。
func afadeExpr(duration float64, f *FadeOptions) string {
	if f == nil || (f.AudioIn <= 0 && f.AudioOut <= 0) {
		return ""
	}
	var parts []string
	if f.AudioIn > 0 {
		parts = append(parts, fmt.Sprintf("afade=t=in:st=0:d=%.3f", f.AudioIn))
	}
	if f.AudioOut > 0 {
		parts = append(parts, fmt.Sprintf("afade=t=out:st=%.3f:d=%.3f", duration-f.AudioOut, f.AudioOut))
	}
	return strings.Join(parts, ",")
}

// subtitlePathExpr 字幕路径转义（盘符冒号、反斜杠、引号）。
func subtitlePathExpr(p string) string {
	s := strings.ReplaceAll(filepath.ToSlash(p), "\\", "/")
	s = strings.ReplaceAll(s, ":", "\\:")
	return strings.ReplaceAll(s, "'", "\\'")
}
