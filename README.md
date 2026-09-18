# FFBox / ffutils

**本地视频处理工具箱**：从零依赖的 ffmpeg Go 工具包到桌面应用、AI 编排与商业化平台。

## 项目结构（按分支）

```
                    ┌──────────────────────────────────────────────┐
                    │  master（稳定发布基线，跟随 base）              │
                    └──────────────────┬───────────────────────────┘
                                       │ merge
                    ┌──────────────────▼───────────────────────────┐
                    │  base — ffutils 库（本分支）                    │
                    │  纯 Go ffmpeg/ffprobe 工具包，零外部依赖         │
                    │  44 个集成测试，覆盖全部公开 API                 │
                    └──────────────────┬───────────────────────────┘
                                       │ merge
                    ┌──────────────────▼───────────────────────────┐
                    │  gui — FFBox 桌面应用                          │
                    │  Wails v2 + 前端，简易/专业双模式，CLI 同二进制   │
                    ├──────────────────┬───────────────────────────┤
                    │                  │ merge                      │
                    │  ┌───────────────▼─────────────┐  ┌─────────▼──────────┐
                    │  │ ai — AI 一句话处理            │  │ license — 激活+商业化│
                    │  │ OpenAI 兼容接口，三步确认制    │  │ Ed25519 激活码      │
                    │  │ 提示词可编辑，计划可修改再执行  │  │ license-server 平台 │
                    │  └─────────────────────────────┘  │ 申请单/微信支付/停用 │
                    │                                   └────────────────────┘
                    └──────────────────────────────────────────────┘
```

| 分支 | 内容 | 核心文件 |
|---|---|---|
| `base` | ffutils 库：探测/转码/拼接/转场/混音/水印/字幕/淡入淡出/分屏/画中画/抽帧/HLS/推帧/组合滤镜/硬件编码探测 | `*.go`（根目录） |
| `gui` | FFBox 桌面应用：六页签 GUI + CLI + 批处理队列 + 时间锚点 + 宽容降级 | `gui/` |
| `ai` | AI 编排：一句话→提示词→计划→执行（三步确认制） | `gui/service/ai.go` |
| `license` | 激活体系：设备码/Ed25519 验签/离线可用/防回拨/模块激活/合规检测/停用控制 + license-server 平台 | `gui/service/license*.go`, `license-server/` |
| `master` | 稳定发布基线（跟随 base） | — |

## 快速开始

```bash
# 库测试（需 bin/ffmpeg.exe + test/ 素材）
cd . && go test ./...

# GUI 构建
cd gui && wails build

# 一键测试流水线（L1-L3，约 4 分钟）
powershell -File gui\scripts\test-all.ps1

# CLI（同二进制子命令）
gui/build/bin/FFBox.exe probe test/2.mp4
gui/build/bin/FFBox.exe convert *.mp4 --preset mp4-small

# 打包分发
powershell -File gui\scripts\package.ps1 -WithFFmpeg
```

## base：ffutils 库 API 总览

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
| `XfadeConcat(XfadeOptions, output, enc)` | 转场拼接：30+ 转场/异构归一化/音轨对齐 |
| `ComposeGrid(clips, cols, rows, audio, output, enc)` | 分屏/宫格 |
| `PictureInPicture(main, pip, opts, output, enc)` | 画中画 |
| `MixAudio(video, []MixTrack, output, enc)` | 多路音频混剪 |

### 画面处理

| 方法 | 用途 |
|---|---|
| `ApplyVideoFilters(input, VideoFilterChain, output, enc)` | 组合滤镜：旋转→裁剪→淡入淡出→字幕→水印一次编码 |
| `Crop(input, x, y, w, h, output, enc)` | 画面裁剪 |
| `Rotate(input, Rotation, output, enc)` | 旋转/翻转 |
| `AddWatermark(input, Watermark, opacity, output, enc)` | 图片水印（含平铺） |
| `BurnSubtitle(input, subtitle, output, enc)` | 硬字幕 |
| `FadeAV(input, FadeOptions, output, enc)` | 音视频淡入淡出 |
| `Reverse(input, output, enc)` | 倒放 |
| `SetVolume(input, factor, output)` | 音量调节 |

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
`AudioCodec`（AAC/MP3/Opus/FLAC/PCM/copy）、`Preset`（ultrafast~veryslow）、`Rotation`（90°/180°/翻转）。
零值（""）= 按上下文取默认。

## gui：FFBox 桌面应用

详见 [gui/README.md](gui/README.md)。核心特性：

- 六页签：转换/剪辑/画面/合成/音频/抽帧
- 简易/专业双模式（全局开关，预设 vs 全参数）
- 批处理队列 + 进度条 + 取消 + 批次汇总（完成x·跳过y·失败z）
- 时间锚点（距开头/距结尾/按进度）逐文件解析 + 宽容降级（跳过/钳制）
- 硬件编码探测（★NVENC 绿色注入）
- 预设管理（保存/应用/导入/导出 JSON）
- CLI 同二进制子命令（12 个，供脚本/自动化用）

## ai：AI 一句话处理

详见 [gui/README.md](gui/README.md#ai-一句话处理)。三步确认制：

```
描述 → 提示词（可编辑）→ 计划（JSON 可改）→ 执行 → 任务栏
```

## license：激活与商业化

详见 [license-server/README.md](license-server/README.md)。

### 核心机制

| 层 | 机制 |
|---|---|
| 设备码 | Windows MachineGuid 哈希（FF-XXXXXXXX-XXXXXXXX，稳定唯一） |
| 激活码 | Ed25519 签名：`base64(payload).sig`，平台私钥签发/GUI 公钥验签 |
| 离线可用 | 激活码存 `~/.ffbox/license.json`，启动时本地验签（不联网） |
| 防时间回拨 | 本地时间水印（HMAC 防篡改）+ max(now, 水印) + 24h 容差 |
| 模块激活 | 令牌/激活码携带 modules（convert/shot/ai/...），GUI 按模块解锁页签 |
| 合规检测 | 启动+首任务前联网上报：签名响应+nonce+时间戳防伪造/重放 |
| 停用控制 | /admin/revoke 幂等（自动/手动统一），端侧下次检测即注销 |

### license-server 部署

```bash
cd license-server && go build -o ffbox-license .
FFBOX_ADMIN_KEY=你的密钥 ./ffbox-license  # 默认 :8787
# 首启打印公钥 → 贴入 gui/service/license.go → wails build → 发布
```

## 测试

```bash
# 一键流水线
powershell -File gui\scripts\test-all.ps1
# L1: Go 测试（base/gui/app+CLI/license-server）
# L2: 前端静态检查（ID 交叉/div 平衡/方法绑定/运行时冒烟）
# L3: CLI 冒烟
# L4（发布前）: GUI 11 场景（gui/testbridge/README.md）
```

详见 [docs/TESTING.md](docs/TESTING.md)。

## 文档索引

| 文档 | 内容 | 分支 |
|---|---|---|
| [AGENTS.md](AGENTS.md) | 开发规则/分支职责/红线/测试流程 | 全分支统一 |
| [gui/README.md](gui/README.md) | GUI 构建/运行/CLI/预设/硬件/打包 | gui |
| [docs/DESIGN.md](docs/DESIGN.md) | GUI 设计：双模式/布局/里程碑 | gui |
| [docs/TESTING.md](docs/TESTING.md) | 测试标准流程 L1-L4 | gui |
| [gui/testbridge/README.md](gui/testbridge/README.md) | GUI 桥接测试（11 场景+已知坑） | gui |
| [license-server/README.md](license-server/README.md) | 激活平台部署/接口/安全设计 | license |
