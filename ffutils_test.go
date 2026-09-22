package ffutils

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// 测试资源位置：bin/ 下放 ffmpeg.exe、ffprobe.exe，test/ 下放测试素材。
// 产物输出到 test/output/<用例名>/，其中部分需人工查看，见 test/output/REVIEW.md。

var (
	binDir  = findDir("bin")
	testDir = findDir("test")
	outRoot = filepath.Join(testDir, "output")
)

// findDir 从当前包目录向上查找子目录（兼容 go test 的工作目录）。
func findDir(name string) string {
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for i := 0; i < 5; i++ {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
		dir = filepath.Dir(dir)
	}
	return name
}

func testFF(t *testing.T) *FFmpeg {
	t.Helper()
	if _, err := os.Stat(binDir); err != nil {
		t.Skipf("未找到 %s，跳过真实命令测试", binDir)
	}
	return &FFmpeg{
		FFmpegPath:  filepath.Join(binDir, exe("ffmpeg")),
		FFprobePath: filepath.Join(binDir, exe("ffprobe")),
		Timeout:     10 * time.Minute,
	}
}

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// outDir 为当前用例创建独立的输出目录。
func outDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(outRoot, t.Name())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func clip(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(testDir, name)
}

// assertFileExists 断言文件存在且非空，返回路径。
func assertFileExists(t *testing.T, path string) string {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("输出文件不存在 %s: %v", path, err)
	}
	if st.Size() == 0 {
		t.Fatalf("输出文件为空: %s", path)
	}
	return path
}

// assertDuration 断言输出时长与期望值误差在 tolerance 秒内。
func assertDuration(t *testing.T, ff *FFmpeg, path string, want, tolerance float64) {
	t.Helper()
	info, err := ff.Probe(path)
	if err != nil {
		t.Fatalf("探测输出失败: %v", err)
	}
	d := info.Duration - want
	if d < -tolerance || d > tolerance {
		t.Fatalf("输出时长 %.3fs，期望 %.3fs（误差 %.3fs 超过 %.3fs）", info.Duration, want, d, tolerance)
	}
	t.Logf("时长校验通过: %.3fs（期望 %.3fs）", info.Duration, want)
}

// ---------- Probe ----------

func TestProbe(t *testing.T) {
	ff := testFF(t)
	info, err := ff.Probe(clip(t, "1.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasVideo || !info.HasAudio {
		t.Fatalf("1.mp4 应同时含视频和音频流: %+v", info)
	}
	if info.Video.Width != 686 || info.Video.Height != 968 {
		t.Fatalf("分辨率应为 686x968，实际 %dx%d", info.Video.Width, info.Video.Height)
	}
	if info.Video.FrameRate < 50 || info.Video.FrameRate > 80 {
		t.Fatalf("帧率应在 50~80（avg_frame_rate 约 62.8），实际 %.2f", info.Video.FrameRate)
	}
	if info.Duration < 90 || info.Duration > 92 {
		t.Fatalf("总时长应约 91s，实际 %.2f", info.Duration)
	}
	if info.Audio.Duration < 90 || info.Audio.Duration > 91.5 {
		t.Fatalf("音频时长应约 91s，实际 %.2f", info.Audio.Duration)
	}
	t.Logf("probe ok: dur=%.2fs video=%dx%d@%.1f audio=%.2fs",
		info.Duration, info.Video.Width, info.Video.Height, info.Video.FrameRate, info.Audio.Duration)
}

func TestProbe_NoAudioStream(t *testing.T) {
	ff := testFF(t)
	// 先生成一个无音频的视频，再探测
	mute := filepath.Join(outDir(t), "mute.mp4")
	if _, err := ff.run(ff.ffmpegBin(), []string{"-i", clip(t, "2.mp4"), "-an", "-t", "1", "-y", mute}); err != nil {
		t.Fatal(err)
	}
	info, err := ff.Probe(mute)
	if err != nil {
		t.Fatal(err)
	}
	if info.HasAudio || info.Audio != nil {
		t.Fatal("去音视频不应探测到音频流")
	}
	if !info.HasVideo {
		t.Fatal("应有视频流")
	}
}

// ---------- ExtractFrame / ExtractCover ----------

func TestExtractFrame(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "frame.png")
	if err := ff.ExtractFrame(clip(t, "1.mp4"), 1.5, out); err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
}

func TestExtractFrame_FirstFrame(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "frame0.png")
	if err := ff.ExtractFrame(clip(t, "1.mp4"), 0, out); err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
}

func TestExtractCover(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "cover.jpg")
	if err := ff.ExtractCover(clip(t, "1.mp4"), out); err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
}

// ---------- ExtractAudio ----------

func TestExtractAudio_MP3(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "audio.mp3")
	if err := ff.ExtractAudio(clip(t, "1.mp4"), AudioExtractOptions{Bitrate: "128k"}, out); err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasAudio || info.HasVideo {
		t.Fatalf("mp3 应只有音频流: %+v", info)
	}
}

// ---------- Concat ----------

func TestConcat_Copy(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "concat.mp4")
	err := ff.Concat([]string{clip(t, "1.mp4"), clip(t, "2.mp4")}, out, EncodeOptions{VideoCodec: "copy"})
	if err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
	// 91.04 + 10.37 ≈ 101.41
	assertDuration(t, ff, out, 101.41, 0.2)
}

func TestConcat_NeedTwoFiles(t *testing.T) {
	ff := testFF(t)
	err := ff.Concat([]string{clip(t, "1.mp4")}, filepath.Join(outDir(t), "x.mp4"), EncodeOptions{})
	if err == nil {
		t.Fatal("单文件拼接应报错")
	}
}

// ---------- XfadeConcat ----------

func TestXfadeConcat_FadeAndCut(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "xfade.mp4")
	err := ff.XfadeConcat(XfadeOptions{
		Clips: []string{clip(t, "1.mp4"), clip(t, "2.mp4"), clip(t, "3.mp4")},
		Transitions: []Transition{
			{Type: "fade", Duration: 0.5},
			{}, // 第二处硬切
		},
	}, out, EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
	// 91.04 + 10.37 + 58.50 - 0.5(转场重叠) ≈ 159.41
	assertDuration(t, ff, out, 159.41, 0.1)

	// 三段分辨率/编码各不相同，输出应统一为最大宽高（1280x974）
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video.Width != 1280 || info.Video.Height != 974 {
		t.Fatalf("输出应归一化到 1280x968，实际 %dx%d", info.Video.Width, info.Video.Height)
	}
}

func TestXfadeConcat_AllFade(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "xfade_all.mp4")
	err := ff.XfadeConcat(XfadeOptions{
		Clips: []string{clip(t, "2.mp4"), clip(t, "3.mp4")},
		Transitions: []Transition{
			{Type: "slideleft", Duration: 1},
		},
	}, out, EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// 10.37 + 58.50 - 1 ≈ 67.87
	assertDuration(t, ff, out, 67.87, 0.1)
}

func TestXfadeConcat_OnlyClips(t *testing.T) {
	ff := testFF(t)
	// 不传 Transitions：全部硬切，等价于带归一化的拼接
	out := filepath.Join(outDir(t), "hardcut.mp4")
	err := ff.XfadeConcat(XfadeOptions{Clips: []string{clip(t, "2.mp4"), clip(t, "3.mp4")}}, out, EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertDuration(t, ff, out, 68.87, 0.1)
}

// ---------- MixAudio ----------

func TestMixAudio_MultiTrack(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "mix.mp4")
	err := ff.MixAudio(clip(t, "1.mp4"), []MixTrack{
		{Path: clip(t, "泉水.mp3"), Volume: 0.3},
		{Path: clip(t, "警笛.mp3"), StartAt: 2.0, Volume: 0.5},
		{Path: clip(t, "锣.mp3"), StartAt: 4.0, Speed: 1.5, TrimIn: 0.5, TrimOut: 3, Volume: 0.6},
	}, out, EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
	// 输出时长以视频轨为准（91.04s），音频较短应被补静音拉齐
	assertDuration(t, ff, out, 91.04, 0.2)
}

func TestMixAudio_NeedTracks(t *testing.T) {
	ff := testFF(t)
	err := ff.MixAudio(clip(t, "1.mp4"), nil, filepath.Join(outDir(t), "x.mp4"), EncodeOptions{})
	if err == nil {
		t.Fatal("空音轨应报错")
	}
}

// ---------- GenerateSpriteSheet ----------

func TestGenerateSpriteSheet(t *testing.T) {
	ff := testFF(t)
	res, err := ff.GenerateSpriteSheet(clip(t, "2.mp4"), SpriteOptions{
		Output:   filepath.Join(outDir(t), "sprite.jpg"),
		FrameMax: 160,
		SheetMax: 2048,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, filepath.Join(outDir(t), "sprite.jpg"))
	if res.FrameWidth <= 0 || res.FrameHeight <= 0 || res.Cols <= 0 || res.Rows <= 0 {
		t.Fatalf("雪碧图布局非法: %+v", res)
	}
	// 2.mp4: 1280x720@30fps, 10.37s。FrameMax=160 -> 缩到 160x90；
	// SheetMax=2048 -> 12 cols x 22 rows = 264 格，311 帧 / interval
	if res.FrameWidth != 160 || res.FrameHeight != 90 {
		t.Fatalf("单帧尺寸应为 160x90，实际 %dx%d", res.FrameWidth, res.FrameHeight)
	}
	t.Logf("sprite ok: %dx%d, %d cols x %d rows, %d frames, interval=%d",
		res.FrameWidth, res.FrameHeight, res.Cols, res.Rows, res.Frames, res.Interval)
}

// ---------- FrameWriter ----------

func TestFrameWriter(t *testing.T) {
	ff := testFF(t)
	// 先抽一帧作为推帧内容和水印图
	frame := filepath.Join(outDir(t), "src_frame.png")
	wm := filepath.Join(outDir(t), "watermark.png")
	if _, err := ff.run(ff.ffmpegBin(), []string{
		"-i", clip(t, "2.mp4"), "-ss", "1", "-frames:v", "1", "-y", frame,
	}); err != nil {
		t.Fatal(err)
	}
	// 生成一张小水印（从同一帧缩放）
	if _, err := ff.run(ff.ffmpegBin(), []string{
		"-i", frame, "-vf", "scale=120:-1", "-y", wm,
	}); err != nil {
		t.Fatal(err)
	}
	frameBytes, err := os.ReadFile(frame)
	if err != nil {
		t.Fatal(err)
	}

	const fps = 5
	out := filepath.Join(outDir(t), "stream.mp4")
	w, err := ff.NewFrameWriter(FrameWriterOptions{
		Width:     1280,
		Height:    720,
		Fps:       fps,
		Output:    out,
		Watermark: &Watermark{Path: wm, Position: "bottomright", Margin: 10},
		Encode:    EncodeOptions{Preset: "medium", CRF: 23},
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := w.Write(frameBytes); err != nil {
			w.Abort()
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	// 10 帧 @ 5fps = 2s
	assertDuration(t, ff, out, 2.0, 0.2)
}

func TestFrameWriter_InvalidOptions(t *testing.T) {
	ff := testFF(t)
	_, err := ff.NewFrameWriter(FrameWriterOptions{Width: 0, Height: 720, Fps: 30, Output: "x.mp4"})
	if err == nil {
		t.Fatal("非法参数应报错")
	}
}

// ---------- CheckVersion ----------

func TestCheckVersion(t *testing.T) {
	ff := testFF(t)
	if err := ff.CheckVersion(ff.ffmpegBin()); err != nil {
		t.Fatalf("ffmpeg 应可用: %v", err)
	}
	if err := ff.CheckVersion(filepath.Join(binDir, "not_exist.exe")); err == nil {
		t.Fatal("不存在的工具应报错")
	}
}

// ---------- Transcode / Remux ----------

func TestTranscode_ResizeAndFps(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "small.mp4")
	err := ff.Transcode(clip(t, "2.mp4"), TranscodeOptions{
		Width: 640, Fps: 24, CRF: 28, TrimEnd: 3,
	}, out)
	if err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video.Width != 640 {
		t.Fatalf("宽度应为 640，实际 %d", info.Video.Width)
	}
	if info.Video.FrameRate < 23 || info.Video.FrameRate > 25 {
		t.Fatalf("帧率应为 24，实际 %.1f", info.Video.FrameRate)
	}
	if info.Duration > 3.5 {
		t.Fatalf("裁剪后应不超过 3s，实际 %.2f", info.Duration)
	}
}

func TestTranscode_ToWebm(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "out.webm")
	err := ff.Transcode(clip(t, "2.mp4"), TranscodeOptions{TrimEnd: 2, VideoBitrate: "500k"}, out)
	if err != nil {
		t.Fatal(err)
	}
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video.Codec != "vp9" {
		t.Fatalf("webm 视频应为 vp9，实际 %s", info.Video.Codec)
	}
	assertDuration(t, ff, out, 2.0, 0.3)
}

func TestRemux(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "remux.mkv")
	if err := ff.Remux(clip(t, "2.mp4"), out); err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
	assertDuration(t, ff, out, 10.37, 0.2)
}

// ---------- Trim / Mute / ReplaceAudio / Speed ----------

func TestTrim(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "trim.mp4")
	if err := ff.Trim(clip(t, "3.mp4"), 10, 15.5, out); err != nil {
		t.Fatal(err)
	}
	// 帧精确重编码：起点/终点都不再有关键帧对齐偏移
	assertDuration(t, ff, out, 5.5, 0.3)
}

func TestMute(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "mute.mp4")
	if err := ff.Mute(clip(t, "2.mp4"), out); err != nil {
		t.Fatal(err)
	}
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.HasAudio {
		t.Fatal("去音后不应有音频流")
	}
}

func TestReplaceAudio(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "replaced.mp4")
	if err := ff.ReplaceAudio(clip(t, "2.mp4"), clip(t, "咳嗽.mp3"), true, out, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
	assertDuration(t, ff, out, 10.37, 0.3)

	// 非循环：配乐短于视频时静音填充到视频结尾（曾用 -shortest 把成片
	// 截到配乐长度：10.4s 视频 × 8s 配乐 → 8s 成片）
	out2 := filepath.Join(outDir(t), "replaced_noloop.mp4")
	if err := ff.ReplaceAudio(clip(t, "2.mp4"), clip(t, "泉水.mp3"), false, out2, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	assertDuration(t, ff, out2, 10.37, 0.3)
}

func TestSpeed(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "speed2x.mp4")
	if err := ff.Speed(clip(t, "2.mp4"), 2.0, out, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	// 10.37 / 2 ≈ 5.19
	assertDuration(t, ff, out, 5.19, 0.2)
}

// ---------- ToGIF ----------

func TestToGIF(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "clip.gif")
	if err := ff.ToGIF(clip(t, "2.mp4"), 1, 3, 320, 10, out); err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video.Codec != "gif" {
		t.Fatalf("应为 gif 编码，实际 %s", info.Video.Codec)
	}
	if info.Video.Width != 320 {
		t.Fatalf("宽度应为 320，实际 %d", info.Video.Width)
	}
}

// ---------- 第三批：画面操作 / 字幕 / 淡入淡出 / 倒放 / 音量 / HLS / 进度 ----------

// shortClip 生成一个 3s 的小片段供画面类测试快速使用。
func shortClip(t *testing.T, ff *FFmpeg) string {
	t.Helper()
	out := filepath.Join(outDir(t), "src3s.mp4")
	if _, err := ff.run(ff.ffmpegBin(), []string{"-i", clip(t, "2.mp4"), "-t", "3", "-y", out}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCrop(t *testing.T) {
	ff := testFF(t)
	src := shortClip(t, ff)
	out := filepath.Join(outDir(t), "crop.mp4")
	if err := ff.Crop(src, 100, 50, 640, 360, out, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video.Width != 640 || info.Video.Height != 360 {
		t.Fatalf("裁剪后应为 640x360，实际 %dx%d", info.Video.Width, info.Video.Height)
	}
}

func TestRotate(t *testing.T) {
	ff := testFF(t)
	src := shortClip(t, ff)
	out := filepath.Join(outDir(t), "rot.mp4")
	if err := ff.Rotate(src, Rot90CW, out, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	// 1280x720 旋转 90° 后应变为 720x1280
	if info.Video.Width != 720 || info.Video.Height != 1280 {
		t.Fatalf("旋转后应为 720x1280，实际 %dx%d", info.Video.Width, info.Video.Height)
	}
	if err := ff.Rotate(src, "bad", filepath.Join(outDir(t), "x.mp4"), EncodeOptions{}); err == nil {
		t.Fatal("未知旋转类型应报错")
	}
}

func TestAddWatermark(t *testing.T) {
	ff := testFF(t)
	src := shortClip(t, ff)
	wm := filepath.Join(outDir(t), "wm.png")
	if _, err := ff.run(ff.ffmpegBin(), []string{
		"-f", "lavfi", "-i", "color=red:s=120x60", "-frames:v", "1", "-y", wm,
	}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(outDir(t), "watermarked.mp4")
	if err := ff.AddWatermark(src, Watermark{Path: wm, Position: PosTopRight, Margin: 20}, 0.5, out, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
}

func TestBurnSubtitle(t *testing.T) {
	ff := testFF(t)
	src := shortClip(t, ff)
	srt := filepath.Join(outDir(t), "sub.srt")
	if err := os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:02,500\n测试字幕 hello\n\n2\n00:00:02,500 --> 00:00:03,000\n第二行\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(outDir(t), "subbed.mp4")
	if err := ff.BurnSubtitle(src, srt, out, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
}

func TestFadeAV(t *testing.T) {
	ff := testFF(t)
	src := shortClip(t, ff)
	out := filepath.Join(outDir(t), "faded.mp4")
	if err := ff.FadeAV(src, FadeOptions{VideoIn: 0.5, VideoOut: 0.5, AudioIn: 0.5, AudioOut: 0.5}, out, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
	if err := ff.FadeAV(src, FadeOptions{}, filepath.Join(outDir(t), "x.mp4"), EncodeOptions{}); err == nil {
		t.Fatal("空淡入淡出参数应报错")
	}
}

func TestReverse(t *testing.T) {
	ff := testFF(t)
	src := shortClip(t, ff)
	out := filepath.Join(outDir(t), "reversed.mp4")
	if err := ff.Reverse(src, out, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	assertDuration(t, ff, out, 3.0, 0.3)
}

func TestSetVolume(t *testing.T) {
	ff := testFF(t)
	src := shortClip(t, ff)
	out := filepath.Join(outDir(t), "vol.mp4")
	if err := ff.SetVolume(src, 0.5, out); err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
}

func TestHLS_RoundTrip(t *testing.T) {
	ff := testFF(t)
	src := shortClip(t, ff)
	srcInfo, err := ff.Probe(src)
	if err != nil {
		t.Fatal(err)
	}

	// 切 HLS
	index := filepath.Join(outDir(t), "hls", "index.m3u8")
	if err := os.MkdirAll(filepath.Dir(index), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ff.ToHLS(src, 1, index); err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, index)

	// ffprobe 能直接解析 HLS 索引（等价于播放器可播）：
	// 时长≈源、音视频轨齐全。分片是相对路径，Probe 也要定位到索引目录
	f2 := &FFmpeg{FFmpegPath: ff.FFmpegPath, FFprobePath: ff.FFprobePath,
		Timeout: ff.Timeout, Dir: filepath.Dir(index)}
	hlsInfo, err := f2.Probe("index.m3u8")
	if err != nil {
		t.Fatalf("ffprobe 解析 HLS 索引失败（播放器同样会失败）: %v", err)
	}
	if !hlsInfo.HasVideo || !hlsInfo.HasAudio {
		t.Fatalf("HLS 应含音视频轨: video=%v audio=%v", hlsInfo.HasVideo, hlsInfo.HasAudio)
	}
	if d := hlsInfo.Duration - srcInfo.Duration; d < -0.5 || d > 0.5 {
		t.Fatalf("HLS 总时长 %.2fs 与源 %.2fs 偏差过大", hlsInfo.Duration, srcInfo.Duration)
	}

	// 本地 m3u8 合成回单文件：分片与索引同目录，用 Dir 定位
	out := filepath.Join(outDir(t), "from_hls.mp4")
	if err := f2.FromHLS("index.m3u8", out); err != nil {
		t.Fatal(err)
	}
	assertDuration(t, ff, out, 3.0, 0.3)
	// 回合一致性：分辨率与源相同（内容一致性的机器可判部分）
	outInfo, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if outInfo.Video.Width != srcInfo.Video.Width || outInfo.Video.Height != srcInfo.Video.Height {
		t.Fatalf("合成回的视频分辨率 %dx%d 应与源 %dx%d 一致",
			outInfo.Video.Width, outInfo.Video.Height, srcInfo.Video.Width, srcInfo.Video.Height)
	}
}

func TestTranscodeProgress(t *testing.T) {
	ff := testFF(t)
	var got Progress
	var calls int
	out := filepath.Join(outDir(t), "prog.mp4")
	err := ff.Transcode(clip(t, "2.mp4"), TranscodeOptions{
		TrimEnd: 2,
		OnProgress: func(p Progress) error {
			calls++
			got = p
			return nil
		},
	}, out)
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Fatal("进度回调未被调用")
	}
	if got.Percent < 95 || got.Percent > 100 {
		t.Fatalf("最终进度应接近 100%%，实际 %.1f%%（共 %d 次回调）", got.Percent, calls)
	}
	t.Logf("进度回调 %d 次，最终 %.1f%%, speed=%.2fx", calls, got.Percent, got.Speed)
}

// ---------- 第四批：分屏 / 画中画 ----------

func TestComposeGrid_2x1(t *testing.T) {
	ff := testFF(t)
	a := filepath.Join(outDir(t), "a3s.mp4")
	b := filepath.Join(outDir(t), "b2s.mp4")
	for i, src := range []string{clip(t, "2.mp4"), clip(t, "3.mp4")} {
		d := []string{a, b}[i]
		if _, err := ff.run(ff.ffmpegBin(), []string{"-i", src, "-t", []string{"3", "2"}[i], "-y", d}); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(outDir(t), "grid.mp4")
	if err := ff.ComposeGrid([]string{a, b}, 2, 1, 0, out, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	// 格子取最大宽高 1280x974，2 列并排 -> 2560x974；时长以最长的 3s 为准
	if info.Video.Width != 2560 || info.Video.Height != 974 {
		t.Fatalf("分屏应为 2560x974，实际 %dx%d", info.Video.Width, info.Video.Height)
	}
	if info.Duration < 2.5 || info.Duration > 3.5 {
		t.Fatalf("分屏时长应约 3s，实际 %.2f", info.Duration)
	}
}

func TestComposeGrid_PadBlack(t *testing.T) {
	ff := testFF(t)
	// 2x2 网格只给 3 个输入，右下角应为黑块
	src := filepath.Join(outDir(t), "s2s.mp4")
	if _, err := ff.run(ff.ffmpegBin(), []string{"-i", clip(t, "2.mp4"), "-t", "2", "-y", src}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(outDir(t), "grid4.mp4")
	if err := ff.ComposeGrid([]string{src, src, src}, 2, 2, 0, out, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video.Width != 2560 || info.Video.Height != 1440 {
		t.Fatalf("四宫格应为 2560x1440，实际 %dx%d", info.Video.Width, info.Video.Height)
	}
}

func TestSplitScreen(t *testing.T) {
	ff := testFF(t)
	a := filepath.Join(outDir(t), "a2s.mp4")
	b := filepath.Join(outDir(t), "b2s2.mp4")
	for i, d := range []string{a, b} {
		if _, err := ff.run(ff.ffmpegBin(), []string{"-i", []string{clip(t, "2.mp4"), clip(t, "3.mp4")}[i], "-t", "2", "-y", d}); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(outDir(t), "vsplit.mp4")
	if err := ff.SplitScreen(a, b, true, out, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video.Width != 1280 || info.Video.Height != 1948 {
		t.Fatalf("上下分屏应为 1280x1948，实际 %dx%d", info.Video.Width, info.Video.Height)
	}
}

func TestPictureInPicture(t *testing.T) {
	ff := testFF(t)
	main := filepath.Join(outDir(t), "main3s.mp4")
	pip := filepath.Join(outDir(t), "pip1s.mp4")
	if _, err := ff.run(ff.ffmpegBin(), []string{"-i", clip(t, "2.mp4"), "-t", "3", "-y", main}); err != nil {
		t.Fatal(err)
	}
	if _, err := ff.run(ff.ffmpegBin(), []string{"-i", clip(t, "3.mp4"), "-t", "1", "-y", pip}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(outDir(t), "pip.mp4")
	if err := ff.PictureInPicture(main, pip, PiPOptions{
		Scale:    0.25,
		Position: PosBottomRight,
		Opacity:  0.8,
	}, out, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	// 画面尺寸不变（仍是主画面大小），时长以主画面为准
	assertDuration(t, ff, out, 3.0, 0.3)
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video.Width != 1280 {
		t.Fatalf("画中画输出宽度应保持 1280，实际 %d", info.Video.Width)
	}
}

func TestTranscodeCommand(t *testing.T) {
	ff := testFF(t)
	cmd := ff.TranscodeCommand("in.mp4", TranscodeOptions{Width: 640, CRF: 23, TrimStart: 10, TrimEnd: 20}, "out.mp4")
	// 不执行进程，仅校验参数串关键片段
	for _, want := range []string{"-ss", "10.000", "-i", "in.mp4", "-t", "10.000",
		"scale=640:-2", "-c:v", "libx264", "-crf", "23", "-y", "out.mp4"} {
		if !strings.Contains(cmd, want) {
			t.Fatalf("命令预览缺少 %q:\n%s", want, cmd)
		}
	}
	t.Logf("preview: %s", cmd)
}

// ---------- ApplyVideoFilters 组合滤镜链 ----------

func TestApplyVideoFilters_Combo(t *testing.T) {
	ff := testFF(t)
	src := filepath.Join(outDir(t), "fx_src.mp4")
	if _, err := ff.run(ff.ffmpegBin(), []string{"-i", clip(t, "2.mp4"), "-t", "2", "-y", src}); err != nil {
		t.Fatal(err)
	}
	// 水印图 + 字幕
	wm := filepath.Join(outDir(t), "fx_wm.png")
	if _, err := ff.run(ff.ffmpegBin(), []string{"-f", "lavfi", "-i", "color=red:s=100x50", "-frames:v", "1", "-y", wm}); err != nil {
		t.Fatal(err)
	}
	srt := filepath.Join(outDir(t), "fx.srt")
	os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:02,000\n组合滤镜测试\n"), 0o644)

	out := filepath.Join(outDir(t), "fx_combo.mp4")
	err := ff.ApplyVideoFilters(src, VideoFilterChain{
		Crop:      &CropRect{X: 100, Y: 50, W: 640, H: 360},
		Watermark: &Watermark{Path: wm, Position: PosTopRight, Opacity: 0.6},
		Subtitle:  srt,
		Fade:      &FadeOptions{VideoIn: 0.3, VideoOut: 0.3, AudioIn: 0.3, AudioOut: 0.3},
	}, out, EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video.Width != 640 || info.Video.Height != 360 {
		t.Fatalf("组合裁剪后应 640x360，实际 %dx%d", info.Video.Width, info.Video.Height)
	}
	if !info.HasAudio {
		t.Fatal("应保留音轨")
	}
}

func TestApplyVideoFilters_CropThenRotate(t *testing.T) {
	ff := testFF(t)
	src := filepath.Join(outDir(t), "fx_src2.mp4")
	if _, err := ff.run(ff.ffmpegBin(), []string{"-i", clip(t, "2.mp4"), "-t", "1", "-y", src}); err != nil {
		t.Fatal(err)
	}
	// 裁剪坐标固定按原始画面（1280x720）定义：裁左上 720x640，再右转 90°
	// -> 640x720。旧顺序（先旋转再裁剪）下该坐标会被误解到宽高互换后的
	// 帧上——x/y 越界被 ffmpeg 静默钳制，裁出错误区域甚至报错。
	out := filepath.Join(outDir(t), "fx_rotcrop.mp4")
	err := ff.ApplyVideoFilters(src, VideoFilterChain{
		Rotate: Rot90CW,
		Crop:   &CropRect{X: 0, Y: 0, W: 720, H: 640},
	}, out, EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video.Width != 640 || info.Video.Height != 720 {
		t.Fatalf("裁剪(720x640)再旋转应 640x720，实际 %dx%d", info.Video.Width, info.Video.Height)
	}
}

func TestApplyVideoFilters_None(t *testing.T) {
	ff := testFF(t)
	// 空链 = 纯重编码，应正常工作
	out := filepath.Join(outDir(t), "fx_none.mp4")
	err := ff.ApplyVideoFilters(clip(t, "2.mp4"), VideoFilterChain{}, out, EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
}

func TestApplyVideoFilters_WatermarkAlone(t *testing.T) {
	// 回归：纯水印（视频链为空）与不透明水印（无透明度滤镜）曾拼出
	// "[0:v][v0]"/"[1:v][w]" 的 label 直连空段，ffmpeg 报 Filter not found
	ff := testFF(t)
	wm := filepath.Join(outDir(t), "wm_alone.png")
	if _, err := ff.run(ff.ffmpegBin(), []string{"-f", "lavfi", "-i", "color=blue:s=80x40", "-frames:v", "1", "-y", wm}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		opacity float64
		rot     Rotation
	}{{"alone_opaque", 1, ""}, {"alone_semi", 0.5, ""}, {"rot_opaque", 1, Rot90CW}} {
		out := filepath.Join(outDir(t), "wm_"+tc.name+".mp4")
		err := ff.ApplyVideoFilters(clip(t, "2.mp4"), VideoFilterChain{
			Watermark: &Watermark{Path: wm, Position: PosTopLeft, Opacity: tc.opacity},
			Rotate:    tc.rot,
		}, out, EncodeOptions{})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		assertFileExists(t, out)
		if tc.rot == Rot90CW {
			info, err := ff.Probe(out)
			if err != nil {
				t.Fatal(err)
			}
			if info.Video.Width != 720 || info.Video.Height != 1280 {
				t.Fatalf("%s 旋转后应 720x1280，实际 %dx%d", tc.name, info.Video.Width, info.Video.Height)
			}
		}
	}
}

func TestExtractFrames_AllFrames(t *testing.T) {
	ff := testFF(t)
	dir := filepath.Join(outDir(t), "seq_all")
	os.MkdirAll(dir, 0o755)
	pattern := filepath.Join(dir, "frame_%04d.jpg")
	// 2.mp4 30fps，取 [1,2) 全量帧 ≈ 30 张
	n, err := ff.ExtractFrames(clip(t, "2.mp4"), 1, 2, 0, pattern)
	if err != nil {
		t.Fatal(err)
	}
	if n < 27 || n > 33 {
		t.Fatalf("1s@30fps 应约 30 张，实际 %d", n)
	}
	// 抽查首帧存在且非空
	assertFileExists(t, filepath.Join(dir, "frame_0001.jpg"))
}

func TestExtractFrames_Sampled(t *testing.T) {
	ff := testFF(t)
	dir := filepath.Join(outDir(t), "seq_1fps")
	os.MkdirAll(dir, 0o755)
	pattern := filepath.Join(dir, "f_%03d.png")
	// [0,3) 每秒 1 帧 → 3 张
	n, err := ff.ExtractFrames(clip(t, "2.mp4"), 0, 3, 1, pattern)
	if err != nil {
		t.Fatal(err)
	}
	if n < 3 || n > 4 {
		t.Fatalf("每秒 1 帧取 3s 应约 3 张，实际 %d", n)
	}
	if _, err := ff.ExtractFrames(clip(t, "2.mp4"), 2, 1, 0, pattern); err == nil {
		t.Fatal("起止倒置应报错")
	}
	if _, err := ff.ExtractFrames(clip(t, "2.mp4"), 0, 1, 0, "no_seq.jpg"); err == nil {
		t.Fatal("缺序号占位应报错")
	}
}

func TestWatermarkTile(t *testing.T) {
	ff := testFF(t)
	src := filepath.Join(outDir(t), "tile_src.mp4")
	if _, err := ff.run(ff.ffmpegBin(), []string{"-i", clip(t, "2.mp4"), "-t", "2", "-y", src}); err != nil {
		t.Fatal(err)
	}
	wm := filepath.Join(outDir(t), "tile_wm.png")
	if _, err := ff.run(ff.ffmpegBin(), []string{"-f", "lavfi", "-i", "color=red:s=100x50", "-frames:v", "1", "-y", wm}); err != nil {
		t.Fatal(err)
	}
	// AddWatermark 平铺
	out := filepath.Join(outDir(t), "tile1.mp4")
	if err := ff.AddWatermark(src, Watermark{Path: wm, Position: PosTile}, 0.5, out, EncodeOptions{}); err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out)
	// 组合链平铺（与淡入淡出叠加）
	out2 := filepath.Join(outDir(t), "tile2.mp4")
	err := ff.ApplyVideoFilters(src, VideoFilterChain{
		Watermark: &Watermark{Path: wm, Position: PosTile, Opacity: 0.4},
		Fade:      &FadeOptions{VideoIn: 0.3, VideoOut: 0.3, AudioIn: 0.3, AudioOut: 0.3},
	}, out2, EncodeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, out2)
}

func TestDetectHardwareEncoders(t *testing.T) {
	ff := testFF(t)
	got := ff.DetectHardwareEncoders()
	// 本机未必有硬件编码器：只断言"探测不报错、返回的都是合法候选"
	valid := map[string]bool{"h264_nvenc": true, "hevc_nvenc": true, "h264_qsv": true, "h264_amf": true}
	for _, h := range got {
		if !valid[string(h.Codec)] {
			t.Fatalf("返回了未知编码器: %s", h.Codec)
		}
		t.Logf("可用硬件编码器: %s (%s)", h.Codec, h.Label)
	}
	if len(got) == 0 {
		t.Log("本机无可用硬件编码器（仅软编码）")
	}
}

func TestXfadeConcatMixedAudio(t *testing.T) {
	// 有音轨 + 无音轨混合拼接（GUI 真实场景发现的回归）：
	// 去音片段参与拼接必须补静音轨而不是失败
	ff := testFF(t)
	mute := filepath.Join(outDir(t), "mixmute.mp4")
	if err := ff.Mute(clip(t, "2.mp4"), mute); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(outDir(t), "mix_concat.mp4")
	err := ff.XfadeConcat(XfadeOptions{
		Clips:       []string{clip(t, "2.mp4"), mute},
		Transitions: []Transition{{Type: Fade, Duration: 0.5}},
	}, out, EncodeOptions{})
	if err != nil {
		t.Fatalf("混合音轨拼接应成功: %v", err)
	}
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasAudio {
		t.Fatal("补静音后成片应有音轨")
	}
	// 10.37 + 10.37 - 0.5 ≈ 20.24
	assertDuration(t, ff, out, 20.24, 0.3)
}

// ---------- 帧序列降采样 / 音量探测 / 合成声音选择 ----------

func TestExtractFrames_EveryN(t *testing.T) {
	ff := testFF(t)
	dir := outDir(t)
	for _, sub := range []string{"full", "half", "quarter"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	full, err := ff.ExtractFrames(clip(t, "1.mp4"), 0, 0, 0, filepath.Join(dir, "full", "f_%04d.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	half, err := ff.ExtractFramesEveryN(clip(t, "1.mp4"), 0, 0, 2, filepath.Join(dir, "half", "f_%04d.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	quarter, err := ff.ExtractFramesEveryN(clip(t, "1.mp4"), 0, 0, 4, filepath.Join(dir, "quarter", "f_%04d.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("全量 %d · 1/2 %d · 1/4 %d", full, half, quarter)
	// 取模降采样：1/2 约为全量的一半（±2 帧容差），1/4 更少
	if half == 0 || half > full {
		t.Fatalf("1/2 帧数量异常: full=%d half=%d", full, half)
	}
	if half*2 < full-2 {
		t.Fatalf("1/2 帧数量偏少: full=%d half=%d", full, half)
	}
	if quarter > half {
		t.Fatalf("1/4 帧不应多于 1/2: half=%d quarter=%d", half, quarter)
	}
}

func TestDetectVolume(t *testing.T) {
	ff := testFF(t)
	vs, err := ff.DetectVolume(clip(t, "1.mp4"))
	if err != nil {
		t.Fatalf("音量探测失败: %v", err)
	}
	t.Logf("mean=%.1f dB max=%.1f dB", vs.MeanDb, vs.MaxDb)
	if vs.MeanDb > 0 || vs.MeanDb < -60 {
		t.Fatalf("平均响度应在 -60~0 dB 区间: %.1f", vs.MeanDb)
	}
	if vs.MaxDb < vs.MeanDb {
		t.Fatalf("峰值不应低于均值: mean=%.1f max=%.1f", vs.MeanDb, vs.MaxDb)
	}
	// 无音轨文件应报错而不是给出假数据
	mute := filepath.Join(outDir(t), "mute.mp4")
	if err := ff.Mute(clip(t, "1.mp4"), mute); err != nil {
		t.Fatal(err)
	}
	if _, err := ff.DetectVolume(mute); err == nil {
		t.Fatal("无音轨文件应返回错误")
	}
}

func TestComposeGrid_MultiAudio(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "grid2audio.mp4")
	err := ff.ComposeGridAudio([]string{clip(t, "1.mp4"), clip(t, "2.mp4")}, 2, 1,
		[]GridAudio{{Index: 0}, {Index: 1, Volume: 0.5}}, out, EncodeOptions{})
	if err != nil {
		t.Fatalf("多音轨宫格应成功: %v", err)
	}
	assertFileExists(t, out)
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasAudio {
		t.Fatal("多音轨合成结果应有音轨")
	}
	// 1.mp4 更长，画面取最长输入（音频 amix duration=longest 不截断）
	assertDuration(t, ff, out, info.Duration, 0.5)
}

func TestComposeGrid_NoAudioKept(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "grid_noaudio.mp4")
	err := ff.ComposeGridAudio([]string{clip(t, "1.mp4"), clip(t, "2.mp4")}, 2, 1,
		nil, out, EncodeOptions{})
	if err != nil {
		t.Fatalf("不保留声音的宫格应成功: %v", err)
	}
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.HasAudio {
		t.Fatal("audio 为空时输出不应有音轨")
	}
}

func TestXfadeConcat_AudioSelect(t *testing.T) {
	ff := testFF(t)
	out := filepath.Join(outDir(t), "concat_sel.mp4")
	// 只保留第二段声音且降为 0.5 倍；第一段补静音
	err := ff.XfadeConcat(XfadeOptions{
		Clips: []string{clip(t, "1.mp4"), clip(t, "2.mp4")},
		Audio: []AudioPick{{Index: 1, Volume: 0.5}},
	}, out, EncodeOptions{})
	if err != nil {
		t.Fatalf("音轨选择拼接应成功: %v", err)
	}
	assertFileExists(t, out)
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasAudio {
		t.Fatal("选择了第二段声音，成片应有音轨")
	}
	// 硬切：时长 = 两段之和
	assertDuration(t, ff, out, info.Duration, 0.5)
}

func TestMixBackground(t *testing.T) {
	ff := testFF(t)
	dir := outDir(t)
	// 有原声视频 + 短音乐循环铺满，原声 0.5 倍、配乐 1 倍
	out := filepath.Join(dir, "mixbgm.mp4")
	err := ff.MixBackground(clip(t, "2.mp4"), clip(t, "锣.mp3"), 0.5, 1, true, out, EncodeOptions{})
	if err != nil {
		t.Fatalf("背景音乐混入应成功: %v", err)
	}
	assertFileExists(t, out)
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasAudio {
		t.Fatal("混入后应有音轨")
	}
	// 时长以视频为准（2.mp4 ≈ 10.37s，锣.mp3 很短靠循环铺满）
	assertDuration(t, ff, out, 10.37, 0.5)

	// 无原声视频 + 配乐（等价纯配乐，验证无原声分支）
	mute := filepath.Join(dir, "mute.mp4")
	if err := ff.Mute(clip(t, "2.mp4"), mute); err != nil {
		t.Fatal(err)
	}
	out2 := filepath.Join(dir, "mutebgm.mp4")
	if err := ff.MixBackground(mute, clip(t, "锣.mp3"), 1, 0.5, true, out2, EncodeOptions{}); err != nil {
		t.Fatalf("无原声配乐应成功: %v", err)
	}
	info2, err := ff.Probe(out2)
	if err != nil {
		t.Fatal(err)
	}
	if !info2.HasAudio {
		t.Fatal("无原声视频配乐后应有音轨")
	}
}

func TestExtractAudioRange(t *testing.T) {
	ff := testFF(t)
	dir := outDir(t)
	out := filepath.Join(dir, "cut.mp3")
	// 抽取 2~4.5 秒的音频段
	if err := ff.ExtractAudio(clip(t, "2.mp4"), AudioExtractOptions{Start: 2, End: 4.5}, out); err != nil {
		t.Fatalf("时间段音频抽取应成功: %v", err)
	}
	assertFileExists(t, out)
	assertDuration(t, ff, out, 2.5, 0.3)
	// wav 格式 + 全长（零值回退原行为）
	out2 := filepath.Join(dir, "full.wav")
	if err := ff.ExtractAudio(clip(t, "2.mp4"), AudioExtractOptions{}, out2); err != nil {
		t.Fatalf("全长音频抽取应成功: %v", err)
	}
	info, err := ff.Probe(out2)
	if err != nil {
		t.Fatal(err)
	}
	if info.Audio == nil || info.Audio.Codec != "pcm_s16le" && info.Audio.Codec != "pcm_s16f" {
		t.Logf("wav 编码: %s", info.Audio.Codec)
	}
}

// ---------- MultiCompose（通用多视频合成） ----------

func TestMultiCompose_SequentialPreprocess(t *testing.T) {
	ff := testFF(t)
	dir := outDir(t)
	out := filepath.Join(dir, "mc_seq.mp4")
	// A：2.mp4(10.37s) 取前 3s 变速 2x → 1.5s；B：3.mp4 取前 2s；
	// fade 0.5s → 总时长 1.5+2-0.5=3.0s；输出归一到最大宽高 1280x974
	err := ff.MultiCompose(MultiComposeOptions{
		Clips: []ClipSpec{
			{Path: clip(t, "2.mp4"), TrimEnd: 3, Speed: 2, Volume: 0.5},
			{Path: clip(t, "3.mp4"), TrimEnd: 2},
		},
		Transitions: []Transition{{Type: Fade, Duration: 0.5}},
	}, out)
	if err != nil {
		t.Fatalf("顺序合成应成功: %v", err)
	}
	assertFileExists(t, out)
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video.Width != 1280 || info.Video.Height != 974 {
		t.Fatalf("输出应归一到 1280x974: %dx%d", info.Video.Width, info.Video.Height)
	}
	assertDuration(t, ff, out, 3.0, 0.4)
	if !info.HasAudio {
		t.Fatal("两路都保留声音，成片应有音轨")
	}
}

func TestMultiCompose_GridOverlay(t *testing.T) {
	ff := testFF(t)
	dir := outDir(t)
	out := filepath.Join(dir, "mc_grid.mp4")
	// 分屏 2x1：A 静音、B 保留；之上叠一个小窗（2.mp4 前 1s，320x180，
	// 半透明）——覆盖 grid+mute+overlay 组合路径
	err := ff.MultiCompose(MultiComposeOptions{
		Layout: LayoutGrid, Cols: 2, Rows: 1,
		Clips: []ClipSpec{
			{Path: clip(t, "2.mp4"), Mute: true},
			{Path: clip(t, "3.mp4")},
		},
		Overlays: []OverlayLayer{
			{Clip: ClipSpec{Path: clip(t, "2.mp4"), TrimEnd: 1},
				X: 50, Y: 50, W: 320, H: 180, Opacity: 0.5},
		},
	}, out)
	if err != nil {
		t.Fatalf("网格+叠加应成功: %v", err)
	}
	assertFileExists(t, out)
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	// 格子=1280x974，2 列 → 2560x974
	if info.Video.Width != 2560 || info.Video.Height != 974 {
		t.Fatalf("分屏输出应为 2560x974: %dx%d", info.Video.Width, info.Video.Height)
	}
	if !info.HasAudio {
		t.Fatal("第二路保留声音，成片应有音轨")
	}
}

func TestMultiCompose_BGMLoopOnly(t *testing.T) {
	ff := testFF(t)
	dir := outDir(t)
	out := filepath.Join(dir, "mc_bgm.mp4")
	// 全静音 + 循环配乐：时长以视频为准（2.mp4 ≈ 10.37s）
	err := ff.MultiCompose(MultiComposeOptions{
		Clips: []ClipSpec{{Path: clip(t, "2.mp4"), Mute: true}},
		Audio: AudioSpec{Keep: []AudioPick{}, BGMPath: clip(t, "锣.mp3"), BGMLoop: true, BGMVolume: 0.6},
	}, out)
	if err != nil {
		t.Fatalf("纯配乐合成应成功: %v", err)
	}
	assertFileExists(t, out)
	info, err := ff.Probe(out)
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasAudio {
		t.Fatal("应有配乐音轨")
	}
	assertDuration(t, ff, out, 10.37, 0.5)
}

func TestMultiCompose_ReverseCropRotate(t *testing.T) {
	ff := testFF(t)
	dir := outDir(t)
	out := filepath.Join(dir, "mc_rev.mp4")
	// 单输入预处理组合：trim 前 2s → 裁 300x400 → 旋转 90° → 倒放
	err := ff.MultiCompose(MultiComposeOptions{
		Clips: []ClipSpec{{
			Path:    clip(t, "3.mp4"),
			TrimEnd: 2,
			Crop:    &CropRect{W: 300, H: 400},
			Rotate:  Rot90CW,
			Reverse: true,
		}},
	}, out)
	if err != nil {
		t.Fatalf("预处理组合应成功: %v", err)
	}
	assertFileExists(t, out)
	assertDuration(t, ff, out, 2.0, 0.4)
}

// ---------- 版本探测与旧版兼容 ----------

func TestFFmpegVersion(t *testing.T) {
	ff := testFF(t)
	maj, min, ok := ff.Version()
	if !ok {
		t.Fatal("版本探测失败")
	}
	t.Logf("ffmpeg %d.%d", maj, min)
	if maj < 4 || (maj == 4 && min < 4) {
		t.Skipf("测试机 ffmpeg %d.%d 低于 4.4，能力标志按旧版处理", maj, min)
	}
	if !ff.HasXfade() || !ff.HasAmixNormalize() {
		t.Fatal("4.4+ 应支持 xfade 与 amix normalize")
	}
}

func TestLegacyCompat(t *testing.T) {
	// 注入旧版本号（4.2）：所有兼容分支走旧路径，命令在 4.4 上同样合法
	ff := testFF(t)
	dir := outDir(t)

	// 1) 旧版 amix 等价路径：混入配乐（两路预乘 2）
	ff.setVersionForTest(4, 2)
	out := filepath.Join(dir, "legacy_bgm.mp4")
	if err := ff.MixBackground(clip(t, "2.mp4"), clip(t, "锣.mp3"), 0.5, 0.8, true, out, EncodeOptions{}); err != nil {
		t.Fatalf("旧版混音路径应成功: %v", err)
	}
	assertFileExists(t, out)
	assertDuration(t, ff, out, 10.37, 0.5)

	// 2) 旧版多轨混音：MixAudio（含原声共 2 路，各预乘 2）
	out2 := filepath.Join(dir, "legacy_mix.mp4")
	err := ff.MixAudio(clip(t, "2.mp4"), []MixTrack{{Path: clip(t, "锣.mp3"), Volume: 0.5}}, out2, EncodeOptions{})
	if err != nil {
		t.Fatalf("旧版多轨混音应成功: %v", err)
	}
	assertDuration(t, ff, out2, 10.37, 0.5)

	// 3) 旧版宫格多音轨
	out3 := filepath.Join(dir, "legacy_grid.mp4")
	if err := ff.ComposeGridAudio([]string{clip(t, "1.mp4"), clip(t, "2.mp4")}, 2, 1,
		[]GridAudio{{Index: 0}, {Index: 1, Volume: 0.5}}, out3, EncodeOptions{}); err != nil {
		t.Fatalf("旧版宫格混音应成功: %v", err)
	}

	// 4) 旧版 xfade 降级：转场退化为硬切，时长=两段之和（2.mp4x2）
	out4 := filepath.Join(dir, "legacy_concat.mp4")
	err = ff.XfadeConcat(XfadeOptions{
		Clips:       []string{clip(t, "2.mp4"), clip(t, "2.mp4")},
		Transitions: []Transition{{Type: Fade, Duration: 0.5}},
	}, out4, EncodeOptions{})
	if err != nil {
		t.Fatalf("旧版拼接（降级硬切）应成功: %v", err)
	}
	assertDuration(t, ff, out4, 10.37*2, 0.5)
}

func TestPiPMainOutlivesPip(t *testing.T) {
	ff := testFF(t)
	dir := outDir(t)
	// 小窗比主画面短：主画面继续播（hold 定格 / hide 消失），不随小窗停止
	pip := filepath.Join(dir, "short.mp4")
	if err := ff.Transcode(clip(t, "2.mp4"), TranscodeOptions{TrimEnd: 2}, pip); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		opt  PiPOptions
	}{{"hold", PiPOptions{Scale: 0.25, PipEnd: PipEndHold}},
		{"hide", PiPOptions{Scale: 0.25, PipEnd: PipEndHide}}} {
		out := filepath.Join(dir, "pip_"+tc.name+".mp4")
		if err := ff.PictureInPicture(clip(t, "2.mp4"), pip, tc.opt, out, EncodeOptions{}); err != nil {
			t.Fatalf("%s 应成功: %v", tc.name, err)
		}
		// 输出时长以主画面为准（2.mp4 ≈ 10.37s），而非小窗的 2s
		assertDuration(t, ff, out, 10.37, 0.5)
	}
}

func TestGridShortest(t *testing.T) {
	ff := testFF(t)
	dir := outDir(t)
	// 一短一长：Shortest 应按短的（3.mp4 截 2s）截断
	short := filepath.Join(dir, "short.mp4")
	if err := ff.Transcode(clip(t, "3.mp4"), TranscodeOptions{TrimEnd: 2}, short); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "grid_short.mp4")
	err := ff.ComposeGridOpts([]string{clip(t, "2.mp4"), short},
		GridOptions{Cols: 2, Rows: 1, Shortest: true}, out, EncodeOptions{})
	if err != nil {
		t.Fatalf("Shortest 分屏应成功: %v", err)
	}
	assertDuration(t, ff, out, 2.0, 0.5)
}

func TestToGIFSize(t *testing.T) {
	ff := testFF(t)
	dir := outDir(t)
	out := filepath.Join(dir, "gif_opt.gif")
	if err := ff.ToGIF(clip(t, "2.mp4"), 1, 3, 320, 10, out); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("优化后 GIF 体积: %d KB（同参数旧实现约 947KB）", st.Size()/1024)
	if st.Size() > 850*1024 { // 原 ~947KB，优化后应明显更小
		t.Fatalf("GIF 体积应明显小于旧实现: %d KB", st.Size()/1024)
	}
}

func TestToGIFWith(t *testing.T) {
	ff := testFF(t)
	dir := outDir(t)
	src := clip(t, "2.mp4")

	// 低配参数：窄/低帧率/少色应明显小于高配参数
	hi := filepath.Join(dir, "gifw_hi.gif")
	lo := filepath.Join(dir, "gifw_lo.gif")
	if err := ff.ToGIFWith(src, 1, 3, GIFOptions{Width: 640, Fps: 12, MaxColors: 192}, hi); err != nil {
		t.Fatal(err)
	}
	if err := ff.ToGIFWith(src, 1, 3, GIFOptions{Width: 400, Fps: 8, MaxColors: 64}, lo); err != nil {
		t.Fatal(err)
	}
	assertFileExists(t, hi)
	assertFileExists(t, lo)
	info, err := ff.Probe(lo)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video.Codec != "gif" || info.Video.Width != 400 {
		t.Fatalf("低配输出应为 gif/400 宽，实际 %s/%d", info.Video.Codec, info.Video.Width)
	}
	hiSz, err := fileSize(hi)
	if err != nil {
		t.Fatal(err)
	}
	loSz, err := fileSize(lo)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("高配 %d KB vs 低配 %d KB", hiSz/1024, loSz/1024)
	if loSz >= hiSz {
		t.Fatalf("低配(%d)应小于高配(%d)", loSz, hiSz)
	}

	// 零值全默认（640/10/128/bayer5）也应成功
	def := filepath.Join(dir, "gifw_default.gif")
	if err := ff.ToGIFWith(src, 0, 0, GIFOptions{}, def); err != nil {
		t.Fatal(err)
	}
	info, err = ff.Probe(def)
	if err != nil {
		t.Fatal(err)
	}
	if info.Video.Width != 640 {
		t.Fatalf("默认宽度应为 640，实际 %d", info.Video.Width)
	}
}

func fileSize(p string) (int64, error) {
	st, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}
