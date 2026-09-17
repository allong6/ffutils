# ffutils

通用 ffmpeg/ffprobe Go 工具包，无任何外部依赖。

## 设计要点

- **入口结构体 `FFmpeg`** 承载工具路径、工作目录、超时，命令统一执行、错误统一包装。
- **一次探测拿全部信息**：`Probe()` 一次调用返回容器时长、视频流、音频流，只起一个 ffprobe 进程（JSON 输出解析）。
- **参数结构体化**：转场用 `[]Transition`，混音轨用 `[]MixTrack`，不使用平行数组。
- **编码参数 Options 化**：`EncodeOptions` 零值即合理默认（libx264/ultrafast/yuv420p/aac 44100），`ExtraArgs` 兜底。
- **时间统一为 float64 秒**。
- **错误统一**：所有命令 `CombinedOutput`，失败时带命令输出尾部。

## 用法

```go
ff := ffutils.New() // 或 &ffutils.FFmpeg{FFmpegPath: "C:/ffmpeg/bin/ffmpeg.exe", Timeout: 5 * time.Minute}

// 探测
info, err := ff.Probe("in.mp4") // info.Duration, info.HasAudio, info.Video.Width ...

// 封面 / 抽帧
_ = ff.ExtractCover("in.mp4", "cover.jpg")
_ = ff.ExtractFrame("in.mp4", 3.5, "frame.png")

// 抽音频（格式按扩展名推断）
_ = ff.ExtractAudio("in.mp4", ffutils.AudioExtractOptions{Bitrate: "192k"}, "out.mp3")

// 流式拷贝拼接（要求同编码参数）
_ = ff.Concat([]string{"a.mp4", "b.mp4"}, "out.mp4", ffutils.EncodeOptions{VideoCodec: "copy"})

// 转场拼接（含音轨对齐与自动补静音）
_ = ff.XfadeConcat(ffutils.XfadeOptions{
    Clips: []string{"a.mp4", "b.mp4", "c.mp4"},
    Transitions: []ffutils.Transition{
        {Type: "fade", Duration: 0.5},
        {}, // 硬切
    },
}, "out.mp4", ffutils.EncodeOptions{})

// 多路音频混剪
_ = ff.MixAudio("in.mp4", []ffutils.MixTrack{
    {Path: "bgm.mp3", Volume: 0.3},
    {Path: "voice.mp3", StartAt: 2.0, Speed: 1.5, TrimIn: 1, TrimOut: 10},
}, "out.mp4", ffutils.EncodeOptions{})

// 雪碧图
res, err := ff.GenerateSpriteSheet("in.mp4", ffutils.SpriteOptions{Output: "sprite.jpg"})

// 持续推帧生成视频（如逐帧渲染写盘）
w, _ := ff.NewFrameWriter(ffutils.FrameWriterOptions{
    Width: 1920, Height: 1080, Fps: 30,
    Output: "out.mp4",
    Watermark: &ffutils.Watermark{Path: "logo.png", Position: "bottomright", Margin: 10},
    Encode:   ffutils.EncodeOptions{Preset: "medium", CRF: 23},
})
for i := 0; i < totalFrames; i++ {
    _ = w.Write(renderFrame(i)) // 完整的一帧 JPEG/PNG 字节
}
_ = w.Close() // 必须调用，等待编码收尾

```

注意：`XfadeConcat` 的 offset 由内部逐段探测时长计算，片段很多时会多次调用 ffprobe；
`MixAudio` 输出时长始终以视频轨为准（`-t` 截断）。

## 功能总览（第二批新增：转码/格式类）

```go
// 转码：换编码器 / 改分辨率 / 改帧率 / 裁剪时间范围，输出格式由扩展名决定
_ = ff.Transcode("in.mp4", ff.TranscodeOptions{
    Width: 640, Fps: 24, CRF: 28, TrimStart: 10, TrimEnd: 20,
}, "out.mp4")

// 转码为 webm（自动选 vp9/opus）
_ = ff.Transcode("in.mp4", ff.TranscodeOptions{VideoBitrate: "2M"}, "out.webm")

// 仅换封装（流拷贝，秒级完成）
_ = ff.Remux("in.mp4", "out.mkv")

// 截取片段（流拷贝，起点按关键帧对齐；需帧精确用 Transcode+TrimStart/End）
_ = ff.Trim("in.mp4", 10, 15.5, "clip.mp4")

// 去音轨 / 替换音轨（音频短于视频时 loop=true 循环铺满）
_ = ff.Mute("in.mp4", "out.mp4")
_ = ff.ReplaceAudio("in.mp4", "bgm.mp3", true, "out.mp4", ff.EncodeOptions{})

// 整条变速（音画同步，0.5~2.0）
_ = ff.Speed("in.mp4", 2.0, "out.mp4", ff.EncodeOptions{})

// 转 GIF（调色板两步法，颜色优于直接转）
_ = ff.ToGIF("in.mp4", 1, 3, 320, 10, "clip.gif")
```

## 测试

`go test ./...` 需要在 `bin/` 放 ffmpeg/ffprobe、`test/` 放测试素材（缺省自动 skip）。
产物输出到 `test/output/<用例名>/`，需人工查看的项目见 `test/output/REVIEW.md`。

## 关于后续 TUI

本包刻意保持"纯函数库"定位：不依赖任何 UI/终端库，所有结果类型（ProbeResult、
SpriteResult 等）带 json tag、可直接序列化展示。做 TUI 时建议在本仓库新增独立的
`tui/` 模块引用 ffutils，TUI 层只负责交互与渲染，不混入命令构建逻辑；
FFmpeg 结构体的字段（路径/超时）也可作为 TUI 的设置项直接暴露。
