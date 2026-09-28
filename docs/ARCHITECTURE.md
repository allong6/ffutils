# 架构文档

> 目标：新会话的 Agent/开发者 5 分钟理解全项目结构与工作方式。

## 1. 项目本质

一个视频处理工具的完整技术栈，从底层到商业化，按分支分层：

```
┌──────────────────────────────────────────────────────────────┐
│ license-server（独立部署的激活平台）                            │
│   令牌管理 / 微信支付 / 合规检测 / 停用控制                      │
│   纯标准库 · Ed25519 签发 · JSON 文件存储                       │
├──────────────────────────────────────────────────────────────┤
│ FFBox GUI（license 分支，含 gui + ai 全部能力）                 │
│   Wails v2 桌面应用 · 六页签 · 简易/专业双模式                   │
│   ├── service 层：任务队列/锚点解析/预设/AI 编排/激活           │
│   ├── 前端：vanilla HTML/JS/CSS（零构建步骤）                   │
│   └── CLI：同二进制子命令（12 个）                              │
├──────────────────────────────────────────────────────────────┤
│ ffutils 库（base 分支）                                        │
│   纯 Go ffmpeg 工具包 · 零外部依赖 · 44 个集成测试              │
│   探测/转码/拼接/转场/混音/水印/字幕/分屏/画中画/推帧/硬件探测   │
└──────────────────────────────────────────────────────────────┘
```

## 2. 分支模型与工作流

```
开发 ──直接提交──▶ base ──merge──▶ master（发布）
                     │
                     └──merge──▶ gui ──直接提交──▶ gui
                                   │
                                   ├──merge──▶ ai（直接提交 AI 编排）
                                   └──merge──▶ license（直接提交激活/商业化）
```

**规则**（详见 AGENTS.md）：
- 库改动 → `base` 分支（写代码 + 测试 + push），gui merge 使用
- GUI 改动 → `gui` 分支（先 merge base），不在此分支改库代码
- AI 编排 → `ai` 分支（先 merge gui），只写提示词/计划/校验
- 激活/商业 → `license` 分支（先 merge gui），GUI 只内嵌公钥
- `AGENTS.md` 修改在 base 进行，其他分支 merge 获取

**红线**：
1. 提交前 `git status --short` 检查，不盲目 `git add -A`
2. 禁止携带未提交修改切换分支
3. PowerShell 脚本 ASCII-only（PS5.1 会把无 BOM UTF-8 当 ANSI 误读）

## 3. ffutils 库（base 分支）

### 核心设计

| 决策 | 原因 |
|---|---|
| 零外部依赖 | 单二进制交付，不需要 go.mod 管理第三方包 |
| 所有命令走 `f.run()` | 统一超时杀进程/stderr 收集/错误包装/Windows 隐藏子进程黑窗 |
| 时间统一 float64 秒 | 不引入毫秒，与 ffmpeg 参数天然一致 |
| 滤镜 label 不复用 | `c%d`（片段归一化）/ `x%d`（叠加结果）两套命名空间 |
| 帧率用 `avg_frame_rate` | `r_frame_rate` 常出现 90000 时间基虚标值 |
| 组合滤镜链 | 裁剪→旋转→适配画布→淡入淡出→字幕→水印一次编码（少几遍重编码） |
| 画面适配 FitMode | 凡把画面塞进固定 W×H 矩形处统一认 stretch/crop/pad，零值=crop（不变形、裁掉溢出） |

### 文件结构

```
ffmpeg.go       入口结构体 FFmpeg、run() 统一执行、超时、错误包装
probe.go        Probe()：一次 ffprobe 返回全部流信息（JSON 解析）
enums.go        TransitionType/Position/FitMode/VideoCodec/AudioCodec/Preset/Rotation
transcode.go    Transcode/Remux/Trim/Speed/ToGIF/TranscodeCommand
concat.go       Concat（流拷贝或重编码）
xfade.go        XfadeConcat（转场拼接+音轨对齐+归一化）
mix.go          MixAudio（多路混音）
extract.go      ExtractFrame/ExtractFrames/ExtractCover/ExtractAudio
compose.go      ComposeGrid/SplitScreen/PictureInPicture
avops.go        Crop/Rotate/AddWatermark/BurnSubtitle/FadeAV/Reverse/SetVolume/HLS
vfilter.go      ApplyVideoFilters（组合滤镜链）
stream.go       FrameWriter（持续推帧编码器）+ Watermark
progress.go     -progress pipe:1 解析 → ProgressFunc 回调
hwaccel.go      DetectHardwareEncoders
```

### 测试

```bash
go test ./...  # 44 个集成测试，需 bin/ + test/ 素材
```
产物 → `test/output/<用例名>/`，人工确认 → `test/output/REVIEW.md`。

## 4. FFBox GUI（gui 分支）

### 技术选型

| 层 | 选型 | 理由 |
|---|---|---|
| UI 框架 | Wails v2 | Go 生态原生集成 ffutils + Web 前端表达力 |
| 前端 | vanilla HTML/JS/CSS | 零构建步骤（不需要 Node.js） |
| 任务调度 | goroutine 串行队列 | 一次只跑一个 ffmpeg（磁盘/CPU 争抢） |
| 配置 | 单 JSON `~/.ffbox/config.json` | 无数据库依赖 |

### 架构分层

```
gui/
├── main.go           Wails 入口 + CLI 入口（同二进制）
├── app.go            前端绑定方法（Bridge 层）
├── cli.go            CLI 子命令
├── service/          核心业务层（纯 Go，不依赖 UI）
│   ├── config.go     配置持久化 + 预设 + AI 配置 + 平台地址
│   ├── jobs.go       串行队列 + 合规门 + 进度回调
│   ├── tasks.go      TaskSpec 统一任务模型（13 种 Kind）
│   ├── modes.go      简易/专业参数映射（单一事实来源）
│   ├── anchor.go     TimeAnchor 时间锚点 + resolveRange 宽容降级
│   ├── convert.go    转换执行 + 默认命名
│   ├── errors.go     ffmpeg 错误人话化
│   └── license*.go   激活体系（license 分支）
├── frontend/dist/    前端（零构建）
│   ├── index.html    页面结构
│   ├── app.js        逻辑（锚点组件/折叠面板/页签切换/批次汇总）
│   └── style.css     统一间距体系
├── scripts/          test-all.ps1 / frontend-check.mjs / package.ps1
└── testbridge/       GUI 桥接测试（11 场景）
```

### 关键设计决策

| 决策 | 说明 |
|---|---|
| service 层与 UI 解耦 | 纯 Go 可独立测试（app_test.go 直接调 App 方法走全链路） |
| 时间锚点 | `TimeAnchor{Anchor: head/tail/pct, Value: n}` 逐文件按实际时长解析 |
| 宽容降级 | 起点超时长→跳过（灰色）；持续超结尾→钳制；截图超界→取最后一帧 |
| 命令预览 | 专业模式可查看实际 ffmpeg 参数（信任感+排障） |
| 批次汇总 | 一批任务全终态时提示"完成x·跳过y·失败z" |
| 合规门 | 首任务入队前联网上报一次（非周期轮询），被停用即拒绝 |

### CLI（同二进制）

```
FFBox convert/trim/mute/volume/shot/concat/fx/grid/pip/bgm/probe/hw
时间参数语义与 GUI 一致（--head/--tail/--pct = 锚点）
```

## 5. AI 编排（ai 分支）

三步确认制（防跑偏的核心设计）：

```
①描述 → ②提示词（可编辑，含文件摘要+操作目录+五条规则）→ ③计划（JSON 可改）→ 执行
```

- `BuildPrompt(desc, files)`：组装完整提示词，前端展示/编辑后再提交
- `ParsePlan(raw)`：从模型回复提取 JSON（容忍围栏/前后杂文/未知操作过滤）
- `PlannedOp.ToTaskSpec(files)`：映射为队列任务（复用全部锚点/降级机制）

## 6. 激活与商业化（license 分支）

### 密钥架构

```
license-server（私钥）──签发──▶ 激活码 ──公钥验签──▶ FFBox GUI
```

GUI 只持有公钥（`licensePublicKey` 常量），私钥永远在平台侧。
激活码 = `base64url(payload) + "." + base64url(Ed25519签名)`
载荷：`{v, product, device, token前缀, plan, modules[], issued, expires}`

### 安全层

| 层 | 防什么 | 机制 |
|---|---|---|
| 验签 | 伪造激活码 | Ed25519 公钥本地验签 |
| 设备绑定 | 一码多用 | 载荷含设备码，启动时比对 |
| 时间水印 | 回拨系统时间 | HMAC(设备码+盐) 防篡改，max(now,水印) |
| 签名响应 | 中间人伪造合规"ok" | /checkin 响应 Ed25519 签名 + nonce + 时间戳 |
| 模块控制 | 越权使用未购功能 | 令牌/激活码携带 modules，GUI 按模块解锁页签 |

### 合规检测时点

1. 应用启动（refreshLicense 内）
2. 首个任务入队前（PreEnqueueCheck 一次性）

不做周期轮询（用户决策）。

### license-server

独立 Go 模块（`license-server/`），纯标准库，JSON 文件存储：
- `/activate` `/deactivate` — 令牌绑定/解绑设备
- `/checkin` — 合规上报（签名响应+永久登记）
- `/admin/revoke` `/admin/unrevoke` — 停用/恢复（幂等）
- `/admin/tokens` — 创建令牌（模块/天数/设备数）
- `/apply` `/pay/create` `/wxpay/notify` — 申请单/微信支付
- `/buy` `/order/{no}` `/my` `/admin-ui` — H5 页面

## 7. 测试体系

详见 [TESTING.md](TESTING.md)。

| 层 | 命令 | 覆盖 |
|---|---|---|
| L1 | `test-all.ps1` | Go 测试：base(44) / gui-service(27) / gui-app+CLI(16) / license-server(11) |
| L2 | `frontend-check.mjs` | 前端 ID 交叉/div 平衡/方法绑定/运行时冒烟 |
| L3 | `test-all.ps1` 内含 | CLI 冒烟（probe + convert） |
| L4 | 手动/桥接 | GUI 11 场景（testbridge/README.md） |

## 8. 打包分发

```powershell
# 常规打包（含 ffmpeg）
powershell -File gui\scripts\package.ps1 -WithFFmpeg

# 带平台地址的发布版
$env:FFBOX_LICENSE_URL = "https://license.example.com"
powershell -File gui\scripts\package.ps1 -WithFFmpeg
```

产出：`gui/build/FFBox-win64-<日期>[-ffmpeg].zip`
