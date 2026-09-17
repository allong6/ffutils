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
	ov := wm.overlayExpr()
	if opacity > 0 && opacity < 1 {
		// 半透明：先给水印图补 alpha 通道并整体调低透明度，再叠加
		ov = fmt.Sprintf("[1:v]format=rgba,colorchannelmixer=aa=%.2f[w];[0:v][w]overlay=%s[v]",
			opacity, wm.overlayExpr())
	} else {
		ov = fmt.Sprintf("[0:v][1:v]overlay=%s[v]", ov)
	}
	args := []string{"-i", input, "-i", wm.Path,
		"-filter_complex", ov, "-map", "[v]", "-map", "0:a:0?"}
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
	filter := "[0:v]reverse[v];[0:a]areverse[a]"
	args := []string{"-i", input, "-filter_complex", filter, "-map", "[v]", "-map", "[a]"}
	args = append(args, enc.outputArgs()...)
	args = append(args, output)
	_, err := f.run(f.ffmpegBin(), args)
	return err
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

// tiledWatermarkChain 生成平铺水印的滤镜段：把水印缩到主画面约 1/4 宽，
// 4×3 平铺成一张大图后整体居中叠加（W/H 为主画面尺寸表达式）。
// opacity 0~1 时先调透明度。
func tiledWatermarkChain(baseLabel string, opacity float64) string {
	alpha := ""
	if opacity > 0 && opacity < 1 {
		alpha = fmt.Sprintf("format=rgba,colorchannelmixer=aa=%.2f,", opacity)
	}
	// scale=W/4 后 tile 4x3，overlay 居中
	return fmt.Sprintf("[1:v]%sscale=W/4:-2,tile=4x3[w];[%s][w]overlay=(W-w)/2:(H-h)/2[v]", alpha, baseLabel)
}
