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

// FitOptions 把画面适配到一个固定尺寸的画布（横竖屏互换、统一成片尺寸）。
type FitOptions struct {
	// Width/Height 目标画布尺寸（像素），必须为正数；编码器通常要求偶数，
	// 传奇数时由调用方自行保证（本结构不自动取偶，避免静默改变用户意图）
	Width, Height int
	// Mode 适配方式，零值 = FitCrop（裁剪填满）
	Mode FitMode
	// Color FitPad 的补边颜色，颜色名或 #RRGGBB；空 = black
	Color string
}

// VideoFilterChain 一组可叠加的画面处理，ApplyVideoFilters 会把启用的
// 项目合成一条滤镜链，一次编码完成全部（相比逐项多次调用：少几遍
// 重编码、画质无损叠加、速度快数倍）。
//
// 处理顺序固定为：裁剪 → 旋转 → 适配画布 → 淡入淡出(视频) → 字幕 → 水印。
// 裁剪在旋转之前（坐标按原始画面定义，先旋转会让宽高互换、坐标全部错位）；
// 适配画布放在旋转之后，这样"先转正再塞进竖屏/横屏画布"能一次做完；
// 字幕在裁剪与适配之后渲染避免被裁掉；水印最后叠加保证在最上层。
type VideoFilterChain struct {
	Crop      *CropRect    // nil = 不裁剪
	Rotate    Rotation     // "" = 不旋转
	Fit       *FitOptions  // nil = 不适配画布（保持裁剪/旋转后的原始尺寸）
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
	if chain.Fit != nil && (chain.Fit.Width <= 0 || chain.Fit.Height <= 0) {
		return fmt.Errorf("适配画布宽高必须为正数: %dx%d", chain.Fit.Width, chain.Fit.Height)
	}

	// 视频滤镜链（按固定顺序拼接：裁剪 → 旋转 → 适配 → 淡入淡出 → 字幕）。
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
	if chain.Fit != nil {
		// setsar=1 必须跟在适配之后：scale 的 force_original_aspect_ratio
		// 只保证等比缩放，不会把像素宽高比元数据写成 1:1（实测 686x968 放大
		// 到 1920x2709 会带上 SAR 309729:309760），不归一会让播放器二次拉伸
		fc, err := fitFilterChain(chain.Fit.Width, chain.Fit.Height, chain.Fit.Mode, chain.Fit.Color)
		if err != nil {
			return err
		}
		vf = append(vf, fc, "setsar=1")
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
		//   [0:v] 视频链 [v0]；半透明时 [1:v] 透明度处理 [w] 再 overlay，
		//   不透明时直接 [v0][1:v]overlay。
		// 注意 label 直连（如 "[1:v][w]"）是非法 filter_graph——此前
		// 纯水印（视频链为空）与 opacity=1（无透明度滤镜）都会拼出
		// 空段，ffmpeg 报 "Filter not found"。
		args = append(args, "-i", input, "-i", wm.Path)
		// filter_graph 分段组装（段间以 ";" 连接；不允许任何空段——
		// label 直连与空前导分号都会让 ffmpeg 报 "No such filter"）
		var parts []string
		main := "[0:v]"
		if len(vf) > 0 {
			parts = append(parts, "[0:v]"+strings.Join(vf, ",")+"[v0]")
			main = "[v0]"
		}
		if wm.Position == PosTile {
			// 平铺格子宽度按主画面 1/4 计，需要先探测
			info, err := f.Probe(input)
			if err != nil {
				return fmt.Errorf("探测主画面失败: %w", err)
			}
			segs, err := watermarkTileSegments(main, "[1:v]", "[v]",
				tileCellWidth(info.Video.Width), wm.Opacity, wm.TileGap)
			if err != nil {
				return err
			}
			parts = append(parts, segs...)
		} else if wm.Opacity > 0 && wm.Opacity < 1 {
			pos, err := wm.overlayExpr()
			if err != nil {
				return err
			}
			parts = append(parts, fmt.Sprintf("[1:v]format=rgba,colorchannelmixer=aa=%.2f[w]", wm.Opacity))
			parts = append(parts, fmt.Sprintf("%s[w]overlay=%s[v]", main, pos))
		} else {
			pos, err := wm.overlayExpr()
			if err != nil {
				return err
			}
			parts = append(parts, fmt.Sprintf("%s[1:v]overlay=%s[v]", main, pos))
		}
		args = append(args, "-filter_complex", strings.Join(parts, ";"), "-map", "[v]", "-map", "0:a:0?")
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

// validateFitMode 校验适配模式是否已定义（零值合法，运行时按默认 FitCrop）。
// FitMode 是库自有语义而非 ffmpeg 词表透传，非法值在这里拦下，
// 不静默回退（静默回退曾掩盖 PosTile 类"值存在但行为错"的问题）。
func validateFitMode(mode FitMode) error {
	switch mode {
	case "", FitStretch, FitCrop, FitPad:
		return nil
	}
	return fmt.Errorf("未知画面适配模式: %q（可选 stretch/crop/pad，见 FitMode 常量组）", string(mode))
}

// fitFilterChain 校验并生成"把任意宽高比的输入适配到 w×h 画布"的滤镜片段，
// 表达式细节见 fitFilterExpr。
func fitFilterChain(w, h int, mode FitMode, color string) (string, error) {
	if err := validateFitMode(mode); err != nil {
		return "", err
	}
	return fitFilterExpr(w, h, mode, color), nil
}

// fitFilterExpr 生成适配滤镜片段（不校验 mode——供已过入口校验的内部
// 链组装函数使用，它们无法中途回错）。
// （不含前导逗号，不含 setsar=1——调用方须自行在链尾补 setsar=1，见下）。
//
//	FitStretch：scale 直接拉到目标尺寸（变形）
//	FitCrop   ：等比放大到铺满画布后居中裁剪（不变形，裁掉溢出部分）
//	FitPad    ：等比缩放到完整可见后居中补边（不变形，留边）
//
// 两个要点（均为实测结论，不是照抄文档）：
//   - force_original_aspect_ratio=increase 只保证"铺满"，等比后的实际尺寸
//     由 ffmpeg 按整数换算得出（686x968 → 1920x1080 得 1920x2709），
//     crop 的默认 x/y 就是居中，因此不必自己写 (iw-ow)/2 表达式；
//   - 等比缩放不会把 SAR 写成 1:1，必须由调用方补 setsar=1，否则部分
//     播放器会按 SAR 二次拉伸。crop/pad 都不会修正 SAR。
func fitFilterExpr(w, h int, mode FitMode, color string) string {
	switch mode {
	case FitStretch:
		return fmt.Sprintf("scale=%d:%d", w, h)
	case FitPad:
		if color == "" {
			color = "black"
		}
		return fmt.Sprintf(
			"scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:%s",
			w, h, w, h, color)
	default: // FitCrop（含零值 ""）
		return fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d", w, h, w, h)
	}
}
