# ffutils

<p align="center">
  <img src="docs/logo.svg" width="160" alt="ffutils logo">
</p>

**零外部依赖的 ffmpeg/ffprobe Go 工具包**：探测/转码/拼接/转场/混音/水印/
字幕/淡入淡出/分屏/画中画/抽帧/HLS/推帧/组合滤镜/硬件编码探测，
130+ 集成测试覆盖全部公开 API。基于它构建的桌面应用见
[FFBox](https://github.com/allong6/ffbox)。

> 2026-09-29 从原 Ffmpeg_utils 仓库独立成库（v1.0.0），历史完整保留；
> 各版本变更与迁移指引见 [docs/CHANGELOG.md](docs/CHANGELOG.md)。

## 安装

```bash
go get github.com/allong6/ffutils
```

要求：本机可用的 ffmpeg/ffprobe（PATH 可见；也可在 `New()` 返回的实例上
设置 `FFmpegPath` / `FFprobePath` / `Dir` 字段显式指定）。

## 快速开始

```go
ff := ffutils.New()
info, _ := ff.Probe("in.mp4") // Duration, HasAudio, Video.Width, Audio.Codec ...
_ = ff.Transcode("in.mp4", ffutils.TranscodeOptions{Width: 1280}, "out.mp4")
```

```bash
# 库测试（需在库根放 bin/ffmpeg.exe + test/ 素材，均不入库，缺失自动 skip）
go test ./...
```

## API 总览

完整方法清单见 [docs/API.md](docs/API.md)（含参数类型与版本标注），以下为能力分组速览。

### 探测

```go
ff := ffutils.New()
info, _ := ff.Probe("in.mp4") // Duration, HasAudio, Video.Width, Audio.Codec ...
```

### 转码/格式

| 方法 | 用途 |
|---|---|
| `Transcode(input, opts, output)` | 通用转码：编码器/分辨率/帧率/裁剪，格式由扩展名决定 |
| `Remux(input, output)` | 仅换封装（流拷贝，秒级） |
| `Trim(input, start, end, output)` | 截片段（流拷贝，关键帧对齐） |
| `Speed(input, 2.0, output, enc)` | 整条变速（音画同步） |
| `ToGIF(input, start, end, w, fps, out)` | 转 GIF（调色板两步法） |
| `TranscodeCommand(input, opts, output)` | 命令预览（不执行进程） |

### 剪辑/合成

| 方法 | 用途 |
|---|---|
| `Concat(paths, output, enc)` | 拼接（同参数流拷贝或重编码） |
| `XfadeConcat(XfadeOptions, output, enc)` | 转场拼接：30+ 转场/异构归一化（含画面适配）/音轨对齐 |
| `ComposeGrid(clips, cols, rows, audio, output, enc)` | 分屏/宫格（`ComposeGridOpts` 可配画面适配） |
| `PictureInPicture(main, pip, opts, output, enc)` | 画中画（小窗位置/大小/透明度/播完表现；主画面与小窗声音各可开关、调音量并混合） |
| `MultiCompose(opts, output)` | 通用合成：多段预处理（截取/裁剪/旋转/倒放/变速）→ 拼接（带转场）或宫格 → 叠加小窗 → 水印/字幕 → 配乐策略，一次成片 |
| `SplitScreen(left, right, vertical, output, enc)` | 双画面分屏快捷方式（左右/上下） |
| `MixAudio(video, []MixTrack, output, enc)` | 多路音频混剪 |
| `MixBackground(video, bgm, mainVol, bgmVol, loop, output, enc)` | 原声 + 背景音乐混音 |

### 画面处理

| 方法 | 用途 |
|---|---|
| `ApplyVideoFilters(input, VideoFilterChain, output, enc)` | 组合滤镜：裁剪→旋转→适配画布→淡入淡出→字幕→水印一次编码 |
| `Crop(input, x, y, w, h, output, enc)` | 画面裁剪 |
| `Rotate(input, Rotation, output, enc)` | 旋转/翻转 |
| `AddWatermark(input, Watermark, opacity, output, enc)` | 图片水印（含平铺） |
| `BurnSubtitle(input, subtitle, output, enc)` | 硬字幕 |
| `FadeAV(input, FadeOptions, output, enc)` | 音视频淡入淡出 |
| `Reverse(input, output, enc)` | 倒放 |
| `SetVolume(input, factor, output)` | 音量调节 |

### 画面适配（FitMode）

输入宽高比与目标不一致时的处理方式。凡是把画面强制塞进某个固定 W×H 矩形的地方
都认这个枚举（`VideoFilterChain.Fit` / `XfadeOptions.Fit` / `GridOptions.Fit` /
`MultiComposeOptions.Fit` / `OverlayLayer.Fit`）：

| 值 | 效果 | 代价 |
|---|---|---|
| `FitStretch` | 拉伸填满，不保持比例 | 变形 |
| `FitCrop`（零值默认） | 等比放大到铺满画布后居中裁剪 | 裁掉超出画布的部分 |
| `FitPad` | 等比缩放到完整可见后居中补边 | 留边（默认黑） |

`FitOptions{Width, Height, Mode, Color}` 配 `VideoFilterChain.Fit` 用，可"先转正
再塞进竖屏/横屏画布"，与其他画面处理合并成一次编码。只指定一边、另一边自适应
的调用（`TranscodeOptions` 只给 Width/Height 之一、`PiPOptions.Scale` 推出的
`scale=W:-2`）天然等比，不受影响，也不需要这个选项。

### 帧操作

| 方法 | 用途 |
|---|---|
| `ExtractFrame(input, at, output)` | 抽一帧 |
| `ExtractFrames(input, start, end, step, pattern)` | 帧序列导出（全量或采样） |
| `ExtractCover(input, output)` | 封面图（>1s 取第 1 秒） |
| `GenerateSpriteSheet(input, opts)` | 雪碧图 |
| `NewFrameWriter(opts)` / `Write(frame)` / `Close()` | 持续推帧编码器 |

### 音频

| 方法 | 用途 |
|---|---|
| `ExtractAudio(input, opts, output)` | 抽音频（mp3/aac/wav 按扩展名） |
| `Mute(input, output)` | 去音轨 |
| `ReplaceAudio(video, audio, loop, output, enc)` | 替换背景音乐 |
| `DetectVolume(path)` | 音量探测（平均/峰值 dB） |

### HLS / 硬件

| 方法 | 用途 |
|---|---|
| `ToHLS(input, segSeconds, output)` | 切 HLS（m3u8 + ts） |
| `FromHLS(source, output)` | 合成 HLS → 单文件 |
| `DetectHardwareEncoders()` | 探测 NVENC/QSV/AMF |

### 其他

| 方法 | 用途 |
|---|---|
| `RunWithProgress(args, totalSec, onProgress)` | 带进度回调的通用执行 |
| `CheckVersion(toolPath)` | 校验 ffmpeg/ffprobe 可用 |

### 枚举常量（enums.go）

`TransitionType`（30+ 转场）、`Position`（四角+平铺）、`VideoCodec`（H264/H265/VP9/copy/NVENC/QSV/AMF）、
`AudioCodec`（AAC/MP3/Opus/FLAC/PCM/copy）、`Preset`（ultrafast~veryslow）、`Rotation`（90°/180°/翻转）、
`FitMode`（裁剪填满/补边/拉伸）、`PipEndAction`（定格/隐藏）、`ComposeLayout`（拼接/宫格）、
`Dither`（GIF 抖动算法 9 种）。

两类枚举的非法值策略不同：

- **透传 ffmpeg 词表的开放枚举**（`VideoCodec`/`AudioCodec`/`Preset`/`TransitionType`）：
  常量仅覆盖常用值，未列出的字符串字面量仍可直接赋值，零值（""）= 按上下文取默认；
- **库自有语义的封闭枚举**（`Position`/`FitMode`/`Rotation`/`PipEndAction`/`Dither`）：
  非空未知值在入口报错（不静默回退），零值（""）= 按上下文取默认。

水印 `Position = PosTile`（平铺）在 AddWatermark / ApplyVideoFilters /
MultiCompose / NewFrameWriter 四条路径生成"缩到主画面 1/4 宽后 4×3 平铺
居中"的滤镜链；画中画（PictureInPicture）不支持平铺，传入会报错。

## tools：裁剪版 ffmpeg 构建

[tools/build-ffmpeg/](tools/build-ffmpeg/)：与本库能力面对齐的 ffmpeg/ffprobe
白名单裁剪自编译（encoder/muxer 白名单来自 enums.go 全量枚举，输入面全保留）。
产物与构建凭据 manifest 供 FFBox 打包内嵌分发，构建与验证流程见该目录
[README](tools/build-ffmpeg/README.md)。

## 下游应用

[FFBox](https://github.com/allong6/ffbox) 桌面应用基于本库构建（转换/剪辑/
画面/合成/音频/抽帧六页签、简易/专业双模式、批处理队列、CLI 子命令等
都在应用层）。FFBox 侧的能力缺口与缺陷通过本仓库的 issue 提出（附需求
背景与开发建议），处理约定见 [AGENTS.md](AGENTS.md)。

## 测试

```bash
# 库测试（需在库根放 bin/ffmpeg.exe + test/ 素材，均不入库，缺失自动 skip）
go test ./...
```

## 文档索引

| 文档 | 内容 |
|---|---|
| [docs/API.md](docs/API.md) | 功能方法清单：全部公开 API 分组一览 |
| [docs/CHANGELOG.md](docs/CHANGELOG.md) | 更新记录：各版本功能更新/方法变更/废弃与迁移指引 |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 技术架构：核心设计/文件结构 |
| [tools/build-ffmpeg/README.md](tools/build-ffmpeg/README.md) | 裁剪版 ffmpeg 构建（白名单对齐本库能力面） |
| [AGENTS.md](AGENTS.md) | 开发规则/版本与变更纪律/Issue 工作流/约定 |

