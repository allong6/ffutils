# AGENTS.md

本仓库是 ffutils——一个无外部依赖的 ffmpeg/ffprobe Go 工具包；`gui/` 目录（gui 分支）
是基于它构建的桌面应用 FFBox。本文档约束在本仓库中工作时应遵循的规则，
**全分支共用同一份内容，不按分支维护不同版本**。

## 文档索引（新会话必读）

| 文档 | 内容 | 位置 |
|---|---|---|
| **本文档** | 开发规则/分支职责/红线/测试流程 | `AGENTS.md`（全分支统一） |
| [docs/STATUS.md](docs/STATUS.md) | **项目状态快照：分支进度/最近完成/待办/环境 + 新会话上手 SOP**（新会话先读） | `docs/`（gui 起维护，随分支同步） |
| [README.md](README.md) | 项目全景：分支结构/API 总览/快速开始/文档索引 | `README.md`（base 定义） |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 技术架构：分层设计/文件结构/关键决策/安全体系 | `docs/`（base 定义） |
| [gui/README.md](gui/README.md) | GUI 构建/CLI/预设/硬件/打包 | `gui/`（gui 分支） |
| [docs/DESIGN.md](docs/DESIGN.md) | GUI 设计文档：双模式/里程碑 | `docs/`（gui 分支） |
| [docs/FEATURES.md](docs/FEATURES.md) | 功能清单：各页签能力/组合语义/输出规则 | `docs/`（gui 分支） |
| [docs/UI-GUIDE.md](docs/UI-GUIDE.md) | 开发样式说明：布局/标题体系/组件规范 | `docs/`（gui 分支） |
| [docs/AI.md](docs/AI.md) | AI 一句话处理：需求契约/提示词/执行映射 | `docs/`（gui 分支） |
| [docs/TESTING.md](docs/TESTING.md) | 测试标准流程 L1-L4 | `docs/`（gui 分支） |
| [license-server/README.md](license-server/README.md) | 激活平台部署/接口/安全 | `license-server/`（license 分支） |

## 构建与测试

```bash
go vet ./... && go build ./...   # 每次改动后必须通过
go test ./...                    # 集成测试
```

- 测试依赖本地资源：`bin/` 下放 `ffmpeg.exe`/`ffprobe.exe`，`test/` 下放测试素材
  （以目录现有为准——1/2/3.mp4 等常驻，其余按需增删）。缺失时测试自动
  skip，不会失败。
- 新增公开方法必须配 1-3 个测试用例（ffutils_test.go），自动断言能断言的一切：
  输出文件存在且非空、时长与理论值误差、分辨率、编码器、流类型。
  用 `assertDuration` / `assertFileExists` / `outDir` / `testFF` 辅助函数。
- 测试产物输出到 `test/output/<用例名>/`，机器无法判断的（转场效果、混音听感、
  画面内容）追加到 `test/output/REVIEW.md` 的人工确认清单，用 checkbox 标注。
- gui 的 service 层测试复用仓库根的 `bin/`、`test/`（`cd gui && go test ./service/`）。

## 项目约定

- **时间单位统一为 float64 秒**，不引入毫秒。
- **所有 ffmpeg/ffprobe 命令必须通过 `f.run()` 执行**（ffmpeg.go），它统一了
  超时杀进程、stderr 收集、错误包装和 Windows 隐藏子进程控制台窗口。
  不允许直接使用 `exec.Command`（界面代码同样不允许绕过 ffutils）。
- **ffmpeg filter_complex 的 label 不允许复用**：片段归一化用 `c%d`，
  叠加结果用 `x%d`（xfade.go 现有惯例）。xfade/concat 类滤镜要求所有输入
  分辨率、帧率一致，异构素材必须先过 `fps/scale/setsar/settb` 归一化。
- **编码参数走 `EncodeOptions` / `TranscodeOptions`**，零值即合理默认，
  不在方法里硬编码编码器；特殊需求用 `ExtraArgs` 逃生舱。
- **帧率解析以 `avg_frame_rate` 为准**，`r_frame_rate` 常出现 90000 之类的
  时间基虚标值，只作回退（probe.go）。
- 一次性命令（转码、拼接等）用 `CombinedOutput` 类流程；持续推帧等长生命周期
  场景参照 `FrameWriter`（stream.go）：stdin 管道 + 后台 goroutine 读 stderr
  （不读会写满管道阻塞 ffmpeg）+ `Close()` 返回最终错误。

## Git 约定

- 提交信息用中文，正文概括本次改动内容。
- `.gitignore` 排除了 `bin/`、`test/` 素材、`test/output/`、`legacy_ffmpeg.go.txt`，
  不要把这些加进版本库。
- **AGENTS.md 的修改在 base 分支进行**，gui 分支通过 merge 获取；
  任何分支都不允许出现与其他分支内容不一致的 AGENTS.md。

## 交互约定

- 任务完成后给出简要总结即可，不要主动开启新话题或追问"还有什么需要"。
- 涉及不可逆操作（删除文件、强制推送、修改远程仓库设置）先向用户确认。

## 分支职责

| 分支 | 职责 |
|---|---|
| `master` | 主分支：稳定发布基线，暂定直接跟随 base 推送 |
| `base` | ffmpeg 功能基础定义分支：只包含 ffutils 库本身与测试，不引入任何 UI/TUI/交互层代码 |
| `gui` | GUI 工具分支：基于 ffutils 的桌面应用（FFBox），**开始任何工作前先 `git merge base`** |
| ~~`ai`~~ | 已并回 gui（2026-09-20）：AI 一句话处理作为 gui 的一个功能项开发完成，独立分支撤销；后续 AI 功能直接在 gui 提交，ffmpeg 能力缺口仍回 base 补 |
| `license` | 激活分支：基于 gui（激活门 + license-server 平台），开工前先 `git merge gui`；GUI 只内嵌公钥验签（私钥绝不进 GUI），平台代码在 license-server/ 独立模块可整体迁移 |

### 提交流向与基准

```
        （功能/修复直接提交）                （发布：merge/fast-forward）
  开发 ──────────────▶ base ──────────────▶ master
                        │
                        │ merge（每次开工前同步）
                        ▼
                        gui ──界面代码直接提交──▶ gui
```

- **base 是唯一上游基准**：master 与 gui 的内容都源于 base，方向永远是
  base → 下游；**严禁反向**（gui/master 向 base 提交或 merge）。
- 直接提交只发生在两处：库代码提交到 base，界面代码提交到 gui；
  master 不承载直接开发，只作为发布基线跟随 base。
- 库的新功能、bug 修复、测试都提交到 `base`；不允许直接向 base 提交
  界面代码，也不允许在 gui 分支直接修改库代码。
- gui 发现库缺陷或需要新能力：切回 base 修改并测试，再 merge 回来。

## 分支红线（历史事故教训，必须遵守）

1. **提交前必须 `git status --short` 检查暂存内容**，禁止盲目的 `git add -A`：
   - `base` 分支上出现任何 `gui/` 路径 = 错误（base 的 .gitignore 已忽略 `gui/`）；
   - `gui` 分支上出现 `gui/build/`、`gui/frontend/wailsjs/` = 错误。
   曾发生：gui 构建产物被 `git add -A` 带进 base 提交，需要二次提交清理。
2. **禁止携带未提交修改切换分支**（`stash` 后到另一分支 `stash pop`、
   或直接 checkout 带走工作区改动）。正确流程：先确定目标分支并确认
   工作区干净，再在目标分支上从零修改。多次事故均源于跨分支携带代码。
   曾发生：gui 的 cli.go 改动 stash 后在 base 上 pop，冲突落进 base
   工作区（2026-09-21，即时恢复）——需跨分支时先提交或明确隔离。
3. **库层行为问题回 base 修**：包括但不限于子进程行为（控制台黑窗、
   退出码、超时杀进程）。曾发生：在 gui 分支写了库文件再挪回 base。
4. **合并时 `AGENTS.md` / `.gitignore` 冲突的既定解法**：
   - `AGENTS.md`：以 base 版本为准（统一内容，见 Git 约定）；
   - `.gitignore`：保留 gui 分支的 `gui/build/`、`gui/frontend/wailsjs/` 条目，
     **不采纳** base 的 `gui/` 整目录规则（那会忽略 gui 分支自己的源码）。

## gui 分支补充约束（M4 起）

- **CLI 子命令**（gui/cli.go）改动后必须跑 `cd gui && go test -run TestCLI ./`；
  参数解析用自研 parseCLI（参数位置任意），不要引入 Go flag 包（首个位置参数后停止解析）。
- **PowerShell 脚本必须 ASCII-only**：PS 5.1 对无 BOM 的 UTF-8 按 ANSI 误读，
  中文注释可能破坏解析（package.ps1 已两次踩坑）。
- **预设持久化**在 `~/.ffbox/config.json` 的 presets 字段，不另建存储。
- **硬件编码探测**结果只做 UI 展示缓存（App.hwOnce），不落盘。

## 测试规范（详见 gui 分支 docs/TESTING.md——分层定义/命令/产物）

**日常改动只做针对性测试**（改哪个模块跑哪层，见下表）；**全量验证
（清场重跑 + 完整报告）仅在用户明确要求"完整测试"时执行**——标准执行器
是三个矩阵脚本（见 L3），报告落 `test/output/<页>-full/REPORT.md`，
含机器断言明细与人工核对清单。

测试分四层，**执行时机与范围**（谁在什么场景必须跑什么）：

| 层 | 验证什么 | 怎么验证 | 执行时机 |
|---|---|---|---|
| L1 代码层 | base 库/service/绑定的 Go 行为 | `go test` 类测试（base 每个公开 API 至少 1 个真实执行用例，断言文件/时长/分辨率/编码器；归因类问题先跑基准命令拿双版本数据）；**App 层绑定（app_test.go）必须走真 Enqueue* 方法并断言输出时长——曾因只断言"成功"漏掉字段搬运断链** | 每次代码改动，提交前 |
| L2 前端静态+模拟 | html/js/css 完整性与交互逻辑 | `frontend-check.mjs`（ID/方法交叉、标签平衡）+ `convert-front-sim.mjs`（转换页字段收集/事件链/模式互转模拟）+ `ai-front-sim.mjs`（AI 页场景模拟）。**模拟器的 stub 必须镜像真实后端行为**（如联动返回值），并优先用真实点击路径（点按钮冒泡），预置状态会掩盖事件顺序类 bug | 每次 `gui/frontend/dist/` 改动，提交前 |
| L3 CLI 矩阵 | 与 GUI 同执行链的功能全量（所有选项×组合×素材，多维度断言：尺寸/帧率/码率/编码/音轨/体积/时长） | `convert-matrix.mjs` / `video-matrix.mjs` / `pages-matrix.mjs`（构建 exe 的 CLI 通道）+ `go test -run TestCLI ./` | 新功能交付前跑对应页矩阵；"完整测试"时三脚本全量 |
| L4a 布局验证 | 真实渲染下的布局样式 | **构建 exe + MCP 截图核对**（左右双栏/页签对位/模式显隐）；MCP 不可用时构建启动留用户验证并注明"未视觉核对" | 涉及 html/css/布局/显隐逻辑的改动，提交前 |
| L4b 功能交互 | 真实渲染下的功能流转 | **构建 exe + MCP 驱动**核心路径并截图存档；MCP 不可用时用 testbridge（真实浏览器 DOM + CLI 后端）或模拟器兜底；AI 对话另跑 live 测试（AI_TEST_KEY） | 新功能/交互改动，交付前 |

- L1-L3 一键：`powershell -File gui\scripts\test-all.ps1`；L4c 全场景
  （发布前/大改后）见 gui/testbridge/README.md。
- 产物：自动测试 `test/output/<用例名>/`；矩阵与全量报告
  `test/output/<页>-full/`；MCP 截图 `test/output/screenshots/`；
  机器不可判项进 test/output/REVIEW.md 或报告内人工清单（仅保留未确认项）。

## 界面改动的提交与交付纪律（2026-09-19 起，用户明确要求）

1. **提交前必须验证布局样式**：凡涉及 `gui/frontend/dist/`（html/js/css）
   的改动，跑过 L2/模拟后还必须**构建并启动应用核对布局**（L4a）：
   `go build -tags desktop,production -o <临时exe> .` 后运行，确认
   整体结构（左右双栏、页签行对位、右栏双视图）未被破坏——静态检查
   无法发现视觉性布局崩塌。有 computer-use MCP 时附截图，不可用时
   在交付说明中注明"布局未经人工核对"。
2. **任务完成后必须编译打包并打开供用户验证**：每个界面类任务收尾时
   构建（`wails build` 或上述 go build）并启动应用留给用户检查，
   不要只交付代码。
3. **样式调整不急于提交（2026-09-20 起）**：视觉/样式类改动（颜色、
   间距、字号、显隐等）保持工作区未提交状态，构建启动交用户确认
   无误后才 commit；用户提出修改意见时先 revert/调整再重建，
   避免频繁回滚已提交历史。
4. 功能增删同步更新 docs/FEATURES.md 与 docs/UI-GUIDE.md；
   项目状态变化（里程碑完成/待办/环境变更）更新 docs/STATUS.md。
