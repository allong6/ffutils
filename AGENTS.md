# AGENTS.md

本仓库是 **ffutils**——无外部依赖的 ffmpeg/ffprobe Go 工具包。2026-09-29
从旧仓库 Ffmpeg_utils（本地工作副本 video_generate）的 base 分支独立成库
（v1.0.0，git 历史完整保留），**单仓库、单分支（main）、独立 remote**，
不再与 FFBox/GUI 共用仓库与分支。下游（FFBox 等）通过
`github.com/allong6/ffutils` 引用，本库的一切变更都是对外的 API 契约。

## 文档索引（新会话必读）

| 文档 | 内容 | 何时必读/必更 |
|---|---|---|
| **本文档** | 开发规则/版本与变更纪律/项目约定/测试流程 | 每次开工前 |
| [docs/CHANGELOG.md](docs/CHANGELOG.md) | 版本变更记录：功能更新/方法变更/废弃与迁移指引 | **每次功能更新随同一提交必更**；发版时定稿 |
| [README.md](README.md) | 项目全景/API 总览/快速开始 | 公开 API 变更后必更 |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 技术架构：核心设计/文件结构 | 分层/文件结构变化后必更 |
| [tools/build-ffmpeg/README.md](tools/build-ffmpeg/README.md) | 裁剪版 ffmpeg/ffprobe 构建（白名单对齐本库能力面） | 构建脚本/白名单变化后必更 |

## 构建与测试

```bash
go vet ./... && go build ./...   # 每次改动后必须通过
go test ./...                    # 集成测试（130+ 用例，覆盖全部公开 API）
```

- 测试依赖本地资源：`bin/` 下放 `ffmpeg.exe`/`ffprobe.exe`，`test/` 下放
  测试素材（1/2/3.mp4 等常驻）。两者均不入库、缺失时测试自动 skip；
  可以是实体目录，也可以用符号链接共享其他工作副本的资源（当前即如此）。
- 新增公开方法必须配 1-3 个测试用例（ffutils_test.go），自动断言能断言的
  一切：输出文件存在且非空、时长与理论值误差、分辨率、编码器、流类型。
  用 `assertDuration` / `assertFileExists` / `outDir` / `testFF` 辅助函数。
- 测试产物输出到 `test/output/<用例名>/`，机器无法判断的（转场效果、混音
  听感、画面内容）追加到 `test/output/REVIEW.md` 的人工确认清单，用
  checkbox 标注。

## 版本与变更记录（对外契约，每次功能更新强制执行）

本库按版本号发布、被下游直接 import，**一次功能更新的交付物 = 代码 +
测试 + CHANGELOG 条目 + godoc 版本注释**，四者随同一提交合入、缺一不可：

1. **所有功能更新必须写进 [docs/CHANGELOG.md](docs/CHANGELOG.md) 的
   Unreleased 段**，一项一条，覆盖：新增 API、行为/参数语义变化、废弃、
   移除、对输出结果有影响的修复。**方法签名变更与废弃必须附迁移指引**
   （旧写法 → 推荐写法），让下游开发者能对号调整。
2. **godoc 版本注释**：新增公开 API 在注释中标注 `Since vX.Y.Z.`；
   语义变化标注"自 vX.Y.Z 起 …"；废弃用 Go 标准标记
   `// Deprecated: 自 vX.Y.Z 起废弃，改用 XXX`（gopls/静态检查会直接
   向调用方提示）。版本号按下一个计划版本填写；发版时把 Unreleased 段
   更名为该版本号 + 日期。
3. **版本号语义化**（git tag）：MINOR = 新增 API/能力；PATCH = 对齐文档
   语义的修复与内部优化；MAJOR = 不兼容变更。v1.x 期间公开 API 只增
   不删；废弃 API 至少保留两个 MINOR 版本，移除须升 MAJOR 且在
   CHANGELOG 给出迁移示例。

发版流程：CHANGELOG 定稿 → `go vet && go build && go test` 全绿 →
`git tag vX.Y.Z`（配置远端后 `git push --tags`）。

## 项目约定

- **时间单位统一为 float64 秒**，不引入毫秒。
- **所有 ffmpeg/ffprobe 命令必须通过 `f.run()` 执行**（ffmpeg.go），它统一了
  超时杀进程、stderr 收集、错误包装和 Windows 隐藏子进程控制台窗口。
  不允许直接使用 `exec.Command`。
- **ffmpeg filter_complex 的 label 不允许复用**：片段归一化用 `c%d`，
  叠加结果用 `x%d`（xfade.go 现有惯例）。xfade/concat 类滤镜要求所有输入
  分辨率、帧率一致，异构素材必须先过 `fps/scale/setsar/settb` 归一化。
- **编码参数走 `EncodeOptions` / `TranscodeOptions`**，零值即合理默认，
  不在方法里硬编码编码器；特殊需求用 `ExtraArgs` 逃生舱。
- **帧率解析以 `avg_frame_rate` 为准**，`r_frame_rate` 常出现 90000 之类的
  时间基虚标值，只作回退（probe.go）。
- **路径纪律**：Go 侧构造路径一律 `filepath.Join`（内部即 Clean），禁止
  手拼分隔符；外部传入的路径（命令行参数、对话框返回、上层应用产物等）在
  **入口**做一次 `filepath.Clean` 归一（空值跳过——`Clean("")` 会得 "."）。
  **例外**：嵌入 ffmpeg 滤镜表达式的路径必须正斜杠+转义（`\` 是滤镜转义符，
  见 vfilter.go 的 `subtitlePathExpr`）；作为 `-i` 输入传入的路径不受此限
  （两种斜杠 ffmpeg 都认）。
- 一次性命令（转码、拼接等）用 `CombinedOutput` 类流程；持续推帧等长生命
  周期场景参照 `FrameWriter`（stream.go）：stdin 管道 + 后台 goroutine 读
  stderr（不读会写满管道阻塞 ffmpeg）+ `Close()` 返回最终错误。

## Git 约定

- 提交信息用中文，正文概括本次改动内容。
- **单分支 main 直接开发**：功能、修复、文档全部直接提交 main。旧仓库的
  base/gui/master 多分支模型已随独立成库作废，不再迁回。
- 本仓库不保留指向旧仓库（video_generate / Ffmpeg_utils）的 remote；
  建立新远端后 `git remote add origin <新仓库地址>`。
- `.gitignore` 排除了 `bin/`、`test/` 素材、`test/output/`、
  `legacy_ffmpeg.go.txt`，不要把这些加进版本库。
- 提交前必须 `git status --short` 检查暂存内容，禁止盲目的 `git add -A`
  （防测试产物、素材、共享符号链接目标等混入）。

## 交互约定

- 任务完成后给出简要总结即可，不要主动开启新话题或追问"还有什么需要"。
- 涉及不可逆操作（删除文件、强制推送、修改远程仓库设置）先向用户确认。
