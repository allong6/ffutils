# 功能方法清单（API）

> ffutils 全部公开 API 的一览，按能力分组；参数与语义细节以各方法的
> godoc 为准。公开 API 增删时本清单与 [README](../README.md) 总览同步
> 更新，版本变更与迁移指引见 [CHANGELOG.md](CHANGELOG.md)。

## 实例与通用执行

| API | 说明 |
|---|---|
| `New() *FFmpeg` | 入口实例；ffmpeg/ffprobe 取自 PATH，可改 `FFmpegPath` / `FFprobePath` / `Dir`（子进程工作目录）/ `Timeout`（单命令超时）字段 |
| `CheckVersion(toolPath)` | 校验 ffmpeg/ffprobe 可用（执行 -version） |
| `RunWithProgress(args, totalSec, onProgress)` | 带进度回调的通用命令执行（回调类型 `ProgressFunc`） |
| `Version()` / `HasXfade()` / `HasAmixNormalize()` | ffmpeg 版本探测与能力标志，能力不足自动走等价/降级路径 |

## 探测

| API | 说明 |
|---|---|
| `Probe(path) (*ProbeResult, error)` | 一次 ffprobe 拿全部信息：Duration / HasAudio / `Video`（`VideoStream`）/ `Audio`（`AudioStream`）宽高、编码器、帧率等 |
| `DetectVolume(path) (VolumeStats, error)` | 平均/峰值音量（dB，`VolumeStats.MeanDb/MaxDb`），只解码音频轨，远快于转码 |
| `DetectHardwareEncoders() []HardwareEncoder` | NVENC/QSV/AMF 硬件编码探测（试编码验证可用性） |

## 转码与格式

| API | 说明 |
|---|---|
| `Transcode(input, opts, output)` | 通用转码（`TranscodeOptions`：编码器/分辨率/帧率/裁剪…，格式由扩展名决定） |
| `TranscodeCommand(input, opts, output)` | 同 Transcode 的命令预览，不执行进程 |
| `Remux(input, output)` | 仅换封装（流拷贝，秒级） |
| `Trim(input, start, end, output)` | 截片段（流拷贝，关键帧对齐） |
| `Mute(input, output)` | 去音轨 |
| `ReplaceAudio(video, audio, loop, output, enc)` | 替换背景音乐（非循环时短配乐静音补齐到视频结尾） |
| `Speed(input, speed, output, enc)` | 整条变速（音画同步） |
| `SpeedRange(input, start, end, speed, output, enc)` | 带时间区间的变速（Since v1.0.0） |
| `SpeedOpts(input, speed, p, output, enc)` | 变速完整版：区间 + 输出宽度/帧率（`PlayOptions`，Since v1.0.0） |
| `ToGIF(input, start, end, w, fps, out)` | 转 GIF（调色板两步法，固定质量参数） |
| `ToGIFWith(input, start, end, GIFOptions, out)` | 完整质量参数版 GIF：宽度/帧率/颜色数/抖动算法/bayer 尺度（Since v1.0.0） |

## 剪辑/合成

| API | 说明 |
|---|---|
| `Concat(paths, output, enc)` | 拼接（同参数流拷贝或重编码） |
| `XfadeConcat(opts, output, enc)` | 转场拼接：30+ 转场（`TransitionType`/`Transition`）、异构素材归一化（含 `Fit` 适配）、音轨选择（`AudioPick`）与时间线对齐 |
| `ComposeGrid(clips, cols, rows, audioTrack, output, enc)` | 分屏/宫格，audioTrack 指定保留哪一路声音 |
| `ComposeGridOpts(clips, GridOptions, output, enc)` | 宫格完整版：时长基准（Shortest）/ 画面适配（Fit）/ 多路声音（`GridAudio`） |
| `ComposeGridAudio(clips, cols, rows, audio, output, enc)` | 宫格 + 多路声音选择（`[]GridAudio` 指定保留路与音量，空 = 不保留） |
| `SplitScreen(left, right, vertical, output, enc)` | 双画面分屏快捷方式（vertical=true 上下，false 左右） |
| `PictureInPicture(main, pip, opts, output, enc)` | 画中画：小窗位置/大小/透明度/播完表现（`PipEndAction`）；主画面与小窗声音各可开关、调音量并混合（`PiPOptions`） |
| `MultiCompose(opts, output)` | 通用合成一次成片：多段预处理（`ClipSpec`：截取/裁剪/旋转/倒放/变速/音量）→ 顺序拼接（带转场，`ComposeLayout`）或宫格 → 叠加小窗层（`OverlayLayer`）→ 整幅水印/字幕 → 音频策略（`AudioSpec`：保留哪些段声音 + 外挂配乐） |
| `MixAudio(video, tracks, output, enc)` | 多路音频混剪（`MixTrack`：每轨截取/起始时间/倍速/音量） |
| `MixBackground(video, bgm, mainVol, bgmVol, loop, output, enc)` | 原声 + 背景音乐两路混音 |

## 画面处理

| API | 说明 |
|---|---|
| `ApplyVideoFilters(input, chain, output, enc)` | 组合滤镜一次编码：裁剪（`CropRect`）→ 旋转 → 适配画布（`FitOptions`）→ 字幕 → 水印 → 淡入淡出（成片整体；自 v1.1.0 起水印/字幕随画面淡出）（`VideoFilterChain`） |
| `Crop(input, x, y, w, h, output, enc)` | 画面裁剪 |
| `Rotate(input, r, output, enc)` | 旋转/翻转（`Rotation`） |
| `AddWatermark(input, wm, opacity, output, enc)` | 图片水印（`Watermark`；`Position` 含四角与平铺 PosTile，平铺可用 `TileGap` 格间透明间隔，Since v1.1.0） |
| `BurnSubtitle(input, subtitle, output, enc)` | 硬字幕（srt/ass） |
| `FadeAV(input, opts, output, enc)` | 音视频淡入淡出（`FadeOptions`） |
| `Reverse(input, output, enc)` | 整条倒放 |
| `ReverseRange(input, start, end, output, enc)` | 带时间区间的倒放（Since v1.0.0） |
| `ReverseOpts(input, p, output, enc)` | 倒放完整版：区间 + 输出宽度/帧率（Since v1.0.0） |
| `SetVolume(input, factor, output)` | 音量倍率调节（视频流拷贝不重编码） |

## 帧与图片

| API | 说明 |
|---|---|
| `ExtractFrame(input, at, output)` | 抽一帧 |
| `ExtractFrames(input, start, end, stepPerSec, pattern)` | 帧序列导出（每秒 stepPerSec 张采样），返回导出张数 |
| `ExtractFramesEveryN(input, start, end, everyN, pattern)` | 帧序列导出（每 everyN 张取 1） |
| `ExtractCover(input, output)` | 封面图（>1s 取第 1 秒） |
| `ExtractAudio(input, opts, output)` | 抽音频（`AudioExtractOptions`，mp3/aac/wav 等按扩展名） |
| `GenerateSpriteSheet(input, opts)` | 雪碧图/快照墙（`SpriteOptions` → `SpriteResult`） |
| `NewFrameWriter(opts)` / `Write(frame)` / `Close()` / `Abort()` | 持续推帧编码器（stdin 管道长生命周期，`FrameWriterOptions`） |

## HLS

| API | 说明 |
|---|---|
| `ToHLS(input, segSeconds, output)` | 切 HLS（m3u8 + ts 分片） |
| `FromHLS(source, output)` | HLS → 单文件（本地路径或 http(s) URL） |

## 枚举与参数类型

| 类型 | 用途 |
|---|---|
| `EncodeOptions` / `TranscodeOptions` | 编码参数（零值即合理默认，`ExtraArgs` 逃生舱） |
| `VideoCodec` / `AudioCodec` / `Preset` / `TransitionType` | **开放枚举**：透传 ffmpeg 词表，未列出的字符串字面量仍可直接赋值，零值 = 按上下文取默认 |
| `Position` / `FitMode` / `Rotation` / `PipEndAction` / `Dither` / `ComposeLayout` | **封闭枚举**：库自有语义，非空未知值在入口报错（不静默回退），零值 = 按上下文取默认 |

两类枚举的策略细节与 `FitMode` 的适配语义见 [README](../README.md)。
