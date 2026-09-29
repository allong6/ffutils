# 更新记录（CHANGELOG）

ffutils 的全部功能更新、方法变更与废弃都记录在本文件，供下游调用方
（FFBox 及其他使用者）按版本同步调整。

记录规则（详见 [AGENTS.md](../AGENTS.md) "版本与变更记录"）：

- 任何功能更新（新增 API、行为/参数语义变化、废弃、移除、对输出结果
  有影响的修复）随同一提交写进下方 **Unreleased** 段，一项一条；
- **方法签名变更与废弃必须附迁移指引**：旧写法 → 推荐写法；
- 对应公开 API 的 godoc 注释同步标注版本（新增 `Since vX.Y.Z`、废弃
  `Deprecated:`、语义变化注明起始版本）。

## Unreleased

- 迁入 `tools/build-ffmpeg` 裁剪构建工具（自 ffbox 迁入）：与本库能力
  面对齐的 ffmpeg/ffprobe 白名单自编译，encoder/muxer 白名单来自
  enums.go 全量枚举，输入面全保留。不影响库 API。
- 文档纯库化梳理：README 清除旧仓库遗留的 gui/ai/license/测试/文档
  索引章节与失效链接；新增 [docs/API.md](API.md) 功能方法清单（全部
  公开 API 分组一览）。不影响库 API。

## v1.0.0（2026-09-29）：独立成库

从旧仓库 Ffmpeg_utils（本地工作副本 video_generate）base 分支独立成
单独的库仓库，git 历史完整保留。基线能力与 API 总览见
[README](../README.md)。

### 从旧仓库迁移

- **module 路径 `ffutils` → `github.com/allong6/ffutils`**。调用方把
  `import "ffutils"` 改为：

  ```go
  import "github.com/allong6/ffutils"
  ```

  并在下游 go.mod 中 `go get github.com/allong6/ffutils@v1.0.0` 后
  `go mod tidy`。

### 语义变更（含在基线内，从更早的旧库代码迁移时注意）

- **画中画 `PiPOptions.UsePipAudio`：替换 → 混合**。此前语义是"用小窗
  音轨替换主画面音轨"；现为"与主画面声音混合播出"（解说场景正解），
  替换语义废弃。配套新增 `PipVolume` / `MainVolume`（两路各自音量
  倍率）与 `MuteMainAudio`（丢弃主画面声音）。
- **合成路径默认画面适配：拉伸 → 裁剪填满（FitCrop）**。新增统一枚举
  `FitMode`（FitStretch / FitCrop / FitPad），凡把画面塞进固定 W×H
  画布的路径都认（`VideoFilterChain.Fit` / `XfadeOptions.Fit` /
  `GridOptions.Fit` / `MultiComposeOptions.Fit` / `OverlayLayer.Fit`）。
  需要旧拉伸观感的显式传 `FitStretch`。
- **`GIFOptions.Dither` 枚举化（封闭枚举）**：9 种算法常量
  （DitherBayer / DitherFS / …），非空未知值在入口报错（此前自由
  字符串直接透传 ffmpeg）。合法字符串字面量仍兼容（底层是
  `type Dither string`）。

### 新增 API（含在基线内）

- `ToGIFWith` + `GIFOptions`：完整 GIF 质量参数（宽度/帧率/颜色数/
  抖动算法/bayer 尺度）；旧 `ToGIF` 保持兼容（转调，固定 640 宽/
  128 色/bayer5）。
- `ReverseRange` / `SpeedRange`：带时间区间的倒放/变速（长视频倒放
  内存占用的主要缓解手段）；`ReverseOpts` / `SpeedOpts` 完整版
  （区间 + 输出宽度/帧率，参数体 `PlayOptions`）。`Reverse` / `Speed`
  保持整条语义，且支持无音轨输入（GIF/无声视频自动只处理画面）。

### 修复（对输出结果有影响）

- `ReplaceAudio`（非循环）：配乐短于视频时静音填充到视频结尾。
- 组合滤镜顺序：裁剪 → 旋转（裁剪坐标按原始画面定义）。
- 纯水印/不透明水印的 filter_graph 空段（曾拼出 label 直连非法段）。
- 帧序列计数：方括号目录（如 `[截图]`）下不再恒报 0。
- Opus 音频默认采样率 48000（44100 会让 libopus 打开编码器失败）。

更早的变更历史见 `git log`。
