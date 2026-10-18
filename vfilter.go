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

// CoverOptions 片头封面图参数，首帧覆盖（VideoFilterChain.Cover）与
// 片头插入（PrependCover）共用。封面与主画面宽高比不一致时按 Fit
// 适配（语义与画布适配一致，不引入新概念）。
// Since v1.3.0.
type CoverOptions struct {
	// Path 封面图片路径（jpg/png 等 ffmpeg 可识别格式）
	Path string
	// Fit 适配方式，零值 = FitCrop（裁剪填满）；FitPad 时补 PadColor 边
	Fit FitMode
	// PadColor Fit=pad 的补边颜色，颜色名或 #RRGGBB；空 = black
	PadColor string
}

// validate 校验封面参数（Fit 复用画布适配的枚举校验）。
func (c *CoverOptions) validate() error {
	if c == nil {
		return nil
	}
	if c.Path == "" {
		return fmt.Errorf("封面图片路径不能为空")
	}
	if err := validateFitMode(c.Fit); err != nil {
		return fmt.Errorf("封面适配模式非法: %w", err)
	}
	return nil
}

// VideoFilterChain 一组可叠加的画面处理，ApplyVideoFilters 会把启用的
// 项目合成一条滤镜链，一次编码完成全部（相比逐项多次调用：少几遍
// 重编码、画质无损叠加、速度快数倍）。
//
// 处理顺序固定为：裁剪 → 旋转 → 适配画布 → 字幕 → 水印 → 淡入淡出 →
// 封面覆盖。裁剪在旋转之前（坐标按原始画面定义，先旋转会让宽高互换、
// 坐标全部错位）；适配画布放在旋转之后，这样"先转正再塞进竖屏/横屏
// 画布"能一次做完；字幕在裁剪与适配之后渲染避免被裁掉；水印在水印位
// 叠加保证在最上层；淡入淡出恒在链尾作用于成片整体（水印与字幕随
// 画面一起变暗，见 issue #2）；封面覆盖是真正的最后一步——只替换首帧，
// 不带水印、不受淡入淡出影响（见 issue #4）。
type VideoFilterChain struct {
	Crop      *CropRect     // nil = 不裁剪
	Rotate    Rotation      // "" = 不旋转
	Fit       *FitOptions   // nil = 不适配画布（保持裁剪/旋转后的原始尺寸）
	Watermark *Watermark    // nil = 不加水印；Opacity 字段生效（0~1）
	Subtitle  string        // 字幕文件路径（srt/ass），空 = 不加
	Fade      *FadeOptions  // nil = 无淡入淡出（仅 VideoIn/VideoOut 参与视频，AudioIn/Out 参与音频）
	Cover     *CoverOptions // nil = 不覆盖首帧；Since v1.3.0

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
	if err := validateWatermark(chain.Watermark); err != nil {
		return err
	}
	if err := chain.Cover.validate(); err != nil {
		return err
	}

	// 懒探测：淡入淡出（时长）、平铺水印与 ScalePct（主画面宽）都需要
	// 输入信息，多个条件并存时只探测一次
	var probed *ProbeResult
	probeOnce := func() (*ProbeResult, error) {
		if probed == nil {
			info, err := f.Probe(input)
			if err != nil {
				return nil, fmt.Errorf("探测主画面失败: %w", err)
			}
			probed = info
		}
		return probed, nil
	}

	// 视频滤镜链（按固定顺序拼接：裁剪 → 旋转 → 适配 → 字幕 → 淡入淡出）。
	// 裁剪必须在旋转之前：CropRect 坐标按原始画面定义（调用方按源分辨率
	// 换算），若先旋转再裁剪，宽高互换后坐标全部错位——ffmpeg 对越界 x/y
	// 静默钳制会裁出错误区域，比例较大时直接报错失败。
	// fade 恒在链尾作用于成片整体：水印与字幕在其之前叠加/烧制，淡出时
	// 随画面一起变暗（见 issue #2）。
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
	// fade 表达式单独收集，恒排在链尾（作用于成片整体，见上方说明）。
	var fadeExprs []string
	if chain.Fade != nil && (chain.Fade.VideoIn > 0 || chain.Fade.VideoOut > 0) {
		info, err := probeOnce()
		if err != nil {
			return err
		}
		if chain.Fade.VideoIn > 0 {
			fadeExprs = append(fadeExprs, fmt.Sprintf("fade=t=in:st=0:d=%.3f", chain.Fade.VideoIn))
		}
		if chain.Fade.VideoOut > 0 {
			fadeExprs = append(fadeExprs, fmt.Sprintf("fade=t=out:st=%.3f:d=%.3f", info.Duration-chain.Fade.VideoOut, chain.Fade.VideoOut))
		}
		chain.audioFade = afadeExpr(info.Duration, chain.Fade)
	}
	if chain.Subtitle != "" {
		vf = append(vf, fmt.Sprintf("subtitles='%s'", subtitlePathExpr(chain.Subtitle)))
	}

	var args []string
	hasCover := chain.Cover != nil && chain.Cover.Path != ""
	if wm := chain.Watermark; wm != nil && wm.Path != "" || hasCover {
		// 水印与封面都需要额外输入，走 filter_complex：
		//   [0:v] 视频链 [v0]；水印段；fade 段；封面段——依次推进链尾
		//   label，最后一段输出 [v]。两段都启用时水印图是 1 号输入、
		//   封面图 2 号；无水印时封面图是 1 号。
		// 注意 label 直连（如 "[1:v][w]"）是非法 filter_graph——空段
		// 会让 ffmpeg 报 "Filter not found"（见 v1.0 回归注释）。
		args = append(args, "-i", input)
		coverIdx := 1
		var parts []string
		cur := "[0:v]"
		if len(vf) > 0 {
			parts = append(parts, "[0:v]"+strings.Join(vf, ",")+"[v0]")
			cur = "[v0]"
		}
		if wm := chain.Watermark; wm != nil && wm.Path != "" {
			args = append(args, "-i", wm.Path)
			coverIdx = 2
			// 后面还有 fade 或封面时用中间 label，否则直接输出 [v]
			next := "[v]"
			if len(fadeExprs) > 0 || hasCover {
				next = "[vm]"
			}
			if wm.Position == PosTile {
				// 平铺格子宽度按主画面 1/4 计，需要先探测
				info, err := probeOnce()
				if err != nil {
					return err
				}
				segs, err := watermarkTileSegments(cur, "[1:v]", next,
					tileCellWidth(info.Video.Width), wm.Opacity, wm.TileGap)
				if err != nil {
					return err
				}
				parts = append(parts, segs...)
			} else {
				// ScalePct 的主画面宽按链序推导（见 effectiveMainSize）
				mainW := 0
				if wm.ScalePct > 0 {
					w, _, err := effectiveMainSize(&chain, probeOnce)
					if err != nil {
						return err
					}
					mainW = w
				}
				segs, err := watermarkSegments(cur, "[1:v]", next, wm, wm.Opacity, mainW)
				if err != nil {
					return err
				}
				parts = append(parts, segs...)
			}
			cur = next
		}
		if len(fadeExprs) > 0 {
			// fade 作用于到此时为止的成片整体（水印与其前的字幕都在
			// 其中，见 issue #2）；封面在其后，不受淡入影响
			next := "[v]"
			if hasCover {
				next = "[vf]"
			}
			parts = append(parts, cur+strings.Join(fadeExprs, ",")+next)
			cur = next
		}
		if hasCover {
			// 封面覆盖是链尾最后一步：适配到链后有效画面尺寸后整幅盖在
			// 首帧上（n=0），第 2 帧起恢复原画面——帧号表达式不依赖
			// 帧率探测，变帧率视频天然稳定（enable 的 n 在 ffmpeg 4.4
			// 与 8.x 均可用，实测验证）
			args = append(args, "-i", chain.Cover.Path)
			w, h, err := effectiveMainSize(&chain, probeOnce)
			if err != nil {
				return err
			}
			fc, err := fitFilterChain(w, h, chain.Cover.Fit, chain.Cover.PadColor)
			if err != nil {
				return err
			}
			parts = append(parts, fmt.Sprintf("[%d:v]%s,setsar=1[cv]", coverIdx, fc))
			parts = append(parts, fmt.Sprintf("%s[cv]overlay=x=0:y=0:enable='not(n)'[v]", cur))
			cur = "[v]"
		}
		args = append(args, "-filter_complex", strings.Join(parts, ";"), "-map", "[v]", "-map", "0:a:0?")
	} else {
		args = append(args, "-i", input)
		if len(vf)+len(fadeExprs) > 0 {
			args = append(args, "-vf", strings.Join(append(vf, fadeExprs...), ","))
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

// effectiveMainSize 推导链后（水印缩放/封面适配时刻）的主画面有效尺寸。
// ApplyVideoFilters 的链序是 裁剪→旋转→适配画布：Fit 启用时画布尺寸即
// 有效尺寸；仅裁剪时裁出的区域经 90°/270° 旋转宽高互换（Rot180/翻转
// 不换）；都无则保持原始尺寸（需探测）。这套几何规则只有库知道，
// ScalePct（v1.2.0）与 Cover（v1.3.0）共用，调用方不感知。
// probeOnce 为懒探测（仅无链尺寸变换时调用），错误原样上抛。
func effectiveMainSize(chain *VideoFilterChain, probeOnce func() (*ProbeResult, error)) (w, h int, err error) {
	switch {
	case chain.Fit != nil:
		return chain.Fit.Width, chain.Fit.Height, nil
	case chain.Crop != nil:
		if chain.Rotate == Rot90CW || chain.Rotate == Rot90CCW {
			return chain.Crop.H, chain.Crop.W, nil
		}
		return chain.Crop.W, chain.Crop.H, nil
	default:
		info, err := probeOnce()
		if err != nil {
			return 0, 0, err
		}
		return info.Video.Width, info.Video.Height, nil
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
