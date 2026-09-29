# 架构文档

> 目标：新会话的 Agent/开发者 5 分钟理解本库结构与工作方式。

## 1. 项目本质

ffutils 是纯 Go 的 ffmpeg/ffprobe 工具包：

- **零外部依赖**（纯标准库），单 package `ffutils`；
- 能力面：探测/转码/拼接/转场/混音/水印/字幕/淡入淡出/分屏/画中画/
  抽帧/HLS/推帧/组合滤镜/雪碧图/硬件编码探测，130+ 集成测试覆盖全部
  公开 API；
- 2026-09-29 从旧仓库 Ffmpeg_utils（本地工作副本 video_generate）base
  分支独立成库（v1.0.0），**单仓库、单分支（main）**，git 历史完整保留；
- 下游：FFBox 桌面应用（独立仓库 github.com/allong6/ffbox）等直接
  `import "github.com/allong6/ffutils"`；
- **版本与变更记录是对外契约**：[CHANGELOG.md](CHANGELOG.md) 记录每个
  版本的功能更新、方法变更与废弃的迁移指引，godoc 注释标注
  `Since` / `Deprecated` 版本（规则见 [AGENTS.md](../AGENTS.md)）。

## 2. 核心设计

| 决策 | 原因 |
|---|---|
| 零外部依赖 | 单二进制交付，不需要管理第三方包 |
| 所有命令走 `f.run()` | 统一超时杀进程/stderr 收集/错误包装/Windows 隐藏子进程黑窗 |
| 时间统一 float64 秒 | 不引入毫秒，与 ffmpeg 参数天然一致 |
| 滤镜 label 不复用 | `c%d`（片段归一化）/ `x%d`（叠加结果）两套命名空间 |
| 帧率用 `avg_frame_rate` | `r_frame_rate` 常见 90000 时间基虚标值 |
| 组合滤镜链 | 裁剪→旋转→适配画布→淡入淡出→字幕→水印一次编码（少几遍重编码） |
| 画面适配 FitMode | 凡把画面塞进固定 W×H 矩形处统一认 stretch/crop/pad，零值=crop |
| 能力降级探测（version.go） | xfade 需 4.3+ / amix normalize 需 4.4+，旧版 ffmpeg 自动走等价路径 |
| CHANGELOG + godoc 版本注释 | 方法变更/废弃附迁移指引，下游按版本适配 |

## 3. 文件结构

```
ffmpeg.go       入口结构体 FFmpeg、New()、CheckVersion、run() 统一执行
probe.go        Probe()：一次 ffprobe 返回全部流信息（JSON 解析）
options.go      EncodeOptions 编码参数（零值默认）与参数拼装
enums.go        开放枚举（VideoCodec/AudioCodec/Preset/TransitionType）
                + 封闭枚举（Position/FitMode/Rotation/Dither/PipEndAction/ComposeLayout）
transcode.go    Transcode/Remux/Trim/Mute/ReplaceAudio/Speed(SpeedRange/SpeedOpts)/
                ToGIF(With)/GIFOptions/TranscodeCommand/seekArgs
concat.go       Concat（流拷贝或重编码）
xfade.go        XfadeConcat（转场拼接+音轨对齐+归一化）
mix.go          MixAudio/MixBackground（多路混音）
volume.go       DetectVolume（音量探测/统计）
extract.go      ExtractFrame/ExtractCover/ExtractAudio/ExtractFrames(EveryN)
sprite.go       GenerateSpriteSheet（雪碧图）
compose.go      ComposeGrid(Opts/Audio)/SplitScreen/PictureInPicture/PiPOptions
multicompose.go MultiCompose（多层叠加/平铺水印等通用合成）
avops.go        Crop/Rotate/AddWatermark/BurnSubtitle/FadeAV/Reverse(ReverseRange/
                ReverseOpts)/SetVolume/ToHLS/FromHLS/PlayOptions
vfilter.go      ApplyVideoFilters（组合滤镜链）/FitOptions
stream.go       FrameWriter（持续推帧编码器）/Watermark
progress.go     -progress pipe:1 解析 → ProgressFunc 回调
hwaccel.go      DetectHardwareEncoders
version.go      ffmpeg 版本探测与能力标志（HasXfade/HasAmixNormalize）
exec_windows.go / exec_other.go  Windows 隐藏子进程控制台窗口
```

## 4. 测试

```bash
go vet ./... && go build ./...
go test ./...   # 需 bin/ 下 ffmpeg/ffprobe + test/ 素材，缺失自动 skip
```

产物 → `test/output/<用例名>/`，人工确认 → `test/output/REVIEW.md`。

## 5. tools/build-ffmpeg

与本库能力面对齐的 ffmpeg/ffprobe 白名单裁剪自编译：encoder/muxer 白名单
来自 enums.go 全量枚举，输入面全保留；产物与构建凭据 manifest 供 FFBox
打包内嵌分发。构建与验证流程见
[tools/build-ffmpeg/README.md](../tools/build-ffmpeg/README.md)。
