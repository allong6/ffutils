package ffutils

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Crop 画面裁剪：从 (x,y) 起取 w×h 区域（像素，原点在左上角）。
// 音频流拷贝不重编码。
func (f *FFmpeg) Crop(input string, x, y, w, h int, output string, enc EncodeOptions) error {
	args := []string{"-i", input,
		"-vf", fmt.Sprintf("crop=%d:%d:%d:%d", w, h, x, y),
		"-map", "0:v:0", "-map", "0:a:0?"}
	args = append(args, enc.outputArgs()...)
	args = append(args, output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// Rotation 旋转 / 翻转方向。
type Rotation string

const (
	Rot90CW  Rotation = "90cw"  // 顺时针 90°
	Rot90CCW Rotation = "90ccw" // 逆时针 90°
	Rot180   Rotation = "180"   // 180°
	FlipH    Rotation = "hflip" // 水平镜像
	FlipV    Rotation = "vflip" // 垂直镜像
)

// Rotate 旋转或翻转画面（音频拷贝）。宽高会随旋转自动互换。
func (f *FFmpeg) Rotate(input string, r Rotation, output string, enc EncodeOptions) error {
	var vf string
	switch r {
	case Rot90CW:
		vf = "transpose=1"
	case Rot90CCW:
		vf = "transpose=2"
	case Rot180:
		vf = "transpose=1,transpose=1"
	case FlipH:
		vf = "hflip"
	case FlipV:
		vf = "vflip"
	default:
		return fmt.Errorf("未知旋转类型: %q", string(r))
	}
	args := []string{"-i", input, "-vf", vf, "-map", "0:v:0", "-map", "0:a:0?"}
	args = append(args, enc.outputArgs()...)
	args = append(args, output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// AddWatermark 给现有视频叠加图片水印（不重编码音频）。
// 透明度 0~1，1 为不透明。
func (f *FFmpeg) AddWatermark(input string, wm Watermark, opacity float64, output string, enc EncodeOptions) error {
	if wm.Path == "" {
		return fmt.Errorf("水印图片路径不能为空")
	}
	var graph string
	if wm.Position == PosTile {
		// 平铺水印的格子宽度按主画面宽度的 1/4 计算需要先探测主画面
		info, err := f.Probe(input)
		if err != nil {
			return fmt.Errorf("探测主画面失败: %w", err)
		}
		graph = strings.Join(watermarkTileSegments("[0:v]", "[1:v]", "[v]",
			tileCellWidth(info.Video.Width), opacity), ";")
	} else {
		pos, err := wm.overlayExpr()
		if err != nil {
			return err
		}
		if opacity > 0 && opacity < 1 {
			// 半透明：先给水印图补 alpha 通道并整体调低透明度，再叠加
			graph = fmt.Sprintf("[1:v]format=rgba,colorchannelmixer=aa=%.2f[w];[0:v][w]overlay=%s[v]",
				opacity, pos)
		} else {
			graph = fmt.Sprintf("[0:v][1:v]overlay=%s[v]", pos)
		}
	}
	args := []string{"-i", input, "-i", wm.Path,
		"-filter_complex", graph, "-map", "[v]", "-map", "0:a:0?"}
	args = append(args, enc.outputArgs()...)
	args = append(args, output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// BurnSubtitle 烧制硬字幕（把字幕渲染进画面，无需播放器支持）。
// 支持 srt/ass 等 ffmpeg 可识别的字幕格式；字体/样式由字幕文件自身决定。
func (f *FFmpeg) BurnSubtitle(input, subtitle string, output string, enc EncodeOptions) error {
	// subtitles 滤镜参数里的路径需要转义：Windows 盘符冒号和反斜杠
	sub := strings.ReplaceAll(filepath.ToSlash(subtitle), "\\", "/")
	sub = strings.ReplaceAll(sub, ":", "\\:")
	sub = strings.ReplaceAll(sub, "'", "\\'")
	args := []string{"-i", input,
		"-vf", fmt.Sprintf("subtitles='%s'", sub),
		"-map", "0:v:0", "-map", "0:a:0?"}
	args = append(args, enc.outputArgs()...)
	args = append(args, output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// FadeOptions 音视频淡入淡出。各字段 0 表示不做。
type FadeOptions struct {
	// VideoIn/VideoOut 视频淡入/淡出时长（秒）；淡出自动从结尾倒数
	VideoIn  float64
	VideoOut float64
	// AudioIn/AudioOut 音频淡入/淡出时长（秒）
	AudioIn  float64
	AudioOut float64
}

// FadeAV 添加音视频淡入淡出（视频 fade 滤镜 + 音频 afade 滤镜）。
func (f *FFmpeg) FadeAV(input string, opts FadeOptions, output string, enc EncodeOptions) error {
	if opts.VideoIn <= 0 && opts.VideoOut <= 0 && opts.AudioIn <= 0 && opts.AudioOut <= 0 {
		return fmt.Errorf("至少指定一项淡入/淡出时长")
	}
	info, err := f.Probe(input)
	if err != nil {
		return err
	}
	dur := info.Duration

	var vf, af []string
	if opts.VideoIn > 0 {
		vf = append(vf, fmt.Sprintf("fade=t=in:st=0:d=%.3f", opts.VideoIn))
	}
	if opts.VideoOut > 0 {
		vf = append(vf, fmt.Sprintf("fade=t=out:st=%.3f:d=%.3f", dur-opts.VideoOut, opts.VideoOut))
	}
	if opts.AudioIn > 0 {
		af = append(af, fmt.Sprintf("afade=t=in:st=0:d=%.3f", opts.AudioIn))
	}
	if opts.AudioOut > 0 {
		af = append(af, fmt.Sprintf("afade=t=out:st=%.3f:d=%.3f", dur-opts.AudioOut, opts.AudioOut))
	}

	args := []string{"-i", input}
	if len(vf) > 0 {
		args = append(args, "-vf", strings.Join(vf, ","))
	}
	if len(af) > 0 {
		args = append(args, "-af", strings.Join(af, ","))
	}
	args = append(args, "-map", "0:v:0", "-map", "0:a:0?")
	args = append(args, enc.outputArgs()...)
	args = append(args, output)
	_, err = f.run(f.ffmpegBin(), args)
	return err
}

// Reverse 倒放（画面与音频同时反向）。注意：reverse 滤镜需要把整段
// 内容载入内存，长片段（>几分钟）可能耗尽内存，建议配合 Trim 先截短。
func (f *FFmpeg) Reverse(input, output string, enc EncodeOptions) error {
	return f.ReverseRange(input, 0, 0, output, enc)
}

// PlayOptions 播放类处理（倒放/变速）的可选参数：时间区间（秒，
// end<=0 到结尾）与输出画面尺寸/帧率（0=保持原始）——播放走独立
// filter_complex 通道，TranscodeOptions 的 Width/Fps 不经过这里，
// 由调用方折入 PlayOptions。
type PlayOptions struct {
	Start, End float64
	Width      int
	Fps        float64
}

// ReverseRange 带时间区间的倒放：先快速 seek 到 start 再处理到 end
// （秒；end<=0 表示到结尾）——长视频倒放内存爆炸的主要缓解手段就是
// 配合区间只倒放片段。无音轨输入（GIF/无声视频）自动只倒放画面。
func (f *FFmpeg) ReverseRange(input string, start, end float64, output string, enc EncodeOptions) error {
	return f.ReverseOpts(input, PlayOptions{Start: start, End: end}, output, enc)
}

// ReverseOpts 倒放完整版：区间 + 宽度/帧率（作为视频链前置滤镜）。
func (f *FFmpeg) ReverseOpts(input string, p PlayOptions, output string, enc EncodeOptions) error {
	filter, maps := playFilters(input, playVfPrefix(p)+"reverse", "areverse", f)
	args := seekArgs(p.Start, p.End)
	args = append(args, "-i", input, "-filter_complex", filter)
	args = append(args, maps...)
	args = append(args, enc.outputArgs()...)
	args = append(args, output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// playVfPrefix 尺寸/帧率前置滤镜（逗号结尾，空参数返回空串）。
func playVfPrefix(p PlayOptions) string {
	var parts []string
	if p.Fps > 0 {
		parts = append(parts, fmt.Sprintf("fps=%.3f", p.Fps))
	}
	if p.Width > 0 {
		parts = append(parts, fmt.Sprintf("scale=%d:-2", p.Width))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ",") + ","
}

// playFilters 构造播放类滤镜（倒放/变速共用）：有音轨走视频+音频双路，
// 无音轨（GIF/无声视频）只处理视频路——滤镜写死 [0:a] 会导致
// "Error binding filtergraph" 失败。
func playFilters(input, videoExpr, audioExpr string, f *FFmpeg) (string, []string) {
	hasAudio := false
	if info, err := f.Probe(input); err == nil {
		hasAudio = info.HasAudio
	}
	if hasAudio {
		return fmt.Sprintf("[0:v]%s[v];[0:a]%s[a]", videoExpr, audioExpr),
			[]string{"-map", "[v]", "-map", "[a]"}
	}
	return fmt.Sprintf("[0:v]%s[v]", videoExpr), []string{"-map", "[v]"}
}

// SetVolume 调整整条音轨音量（视频拷贝不重编码）。
// factor 为倍率：0.5 减半、2 加倍；也可用 dB 表达如 "−6dB"（传字符串需另用 Transcode）。
func (f *FFmpeg) SetVolume(input string, factor float64, output string) error {
	if factor <= 0 {
		return fmt.Errorf("音量倍率必须为正: %v", factor)
	}
	args := []string{"-i", input, "-af", fmt.Sprintf("volume=%.3f", factor),
		"-c:v", "copy", "-y", output}
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// FromHLS 下载/读取 HLS（m3u8）并合成为单个文件。
// source 可以是本地 m3u8 索引路径（相对/绝对）或 http(s) URL；
// 本地播放列表引用的 ts 分片必须与索引文件同目录。
func (f *FFmpeg) FromHLS(source, output string) error {
	args := []string{
		"-protocol_whitelist", "file,http,https,tcp,tls,crypto",
		"-allowed_extensions", "ALL",
		"-i", source,
		"-c", "copy", "-y", output,
	}
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// ToHLS 把视频切分为 HLS（m3u8 + ts 分片）输出，适合网页播放分发。
// segSeconds 为每段时长（秒），output 为 m3u8 索引输出路径。
func (f *FFmpeg) ToHLS(input string, segSeconds int, output string) error {
	if segSeconds <= 0 {
		segSeconds = 10
	}
	args := []string{
		"-i", input,
		"-c", "copy",
		"-hls_time", itoa(segSeconds),
		"-hls_playlist_type", "vod",
		"-hls_segment_filename",
		strings.TrimSuffix(output, filepath.Ext(output)) + "_%03d.ts",
		"-y", output,
	}
	_, err := f.run(f.ffmpegBin(), args)
	return err
}

// tileCellWidth 平铺水印的单格宽度：主画面宽度的 1/4，取偶（编码器友好），
// 至少 2px。
func tileCellWidth(mainW int) int {
	w := mainW / 4
	w &^= 1
	if w < 2 {
		w = 2
	}
	return w
}

// watermarkTileSegments 生成平铺水印的 filter_graph 分段（段间以 ";" 连接）：
// 水印（可选透明度预处理）缩到 cellW 宽后 4×3 平铺成一张大图，整体居中
// 叠加到 mainLabel 画面上，输出 outLabel。
//
// 缩放宽度由调用方用主画面宽度算好传入（数值字面量），不用 scale2ref：
//   - scale 滤镜自身只有 iw/ih 变量，直接写 W/4 取不到主画面宽度；
//   - scale2ref 的双输出形式（引用流随路透传）是 ffmpeg 5.1+ 才有，
//     数值方案对 4.x 也成立。
// 居中坐标用 overlay 表达式 (W-w)/2:(H-h)/2 运行时求解（平铺图高度
// 依水印宽高比而定，调用方无法预知）。
func watermarkTileSegments(mainLabel, wmLabel, outLabel string, cellW int, opacity float64) []string {
	wm := wmLabel
	segs := []string{}
	if opacity > 0 && opacity < 1 {
		segs = append(segs, fmt.Sprintf("%sformat=rgba,colorchannelmixer=aa=%.2f[wa]", wmLabel, opacity))
		wm = "[wa]"
	}
	segs = append(segs, fmt.Sprintf("%sscale=%d:-2,tile=4x3[wt]", wm, cellW))
	segs = append(segs, fmt.Sprintf("%s[wt]overlay=(W-w)/2:(H-h)/2%s", mainLabel, outLabel))
	return segs
}
