# AGENTS.md

本仓库是 ffutils——一个无外部依赖的 ffmpeg/ffprobe Go 工具包；`gui/` 目录（gui 分支）
是基于它构建的桌面应用 FFBox。本文档约束在本仓库中工作时应遵循的规则，
**全分支共用同一份内容，不按分支维护不同版本**。

## 构建与测试

```bash
go vet ./... && go build ./...   # 每次改动后必须通过
go test ./...                    # 集成测试
```

- 测试依赖本地资源：`bin/` 下放 `ffmpeg.exe`/`ffprobe.exe`，`test/` 下放测试素材
  （1.mp4/2.mp4/3.mp4、若干 mp3）。缺失时测试自动 skip，不会失败。
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

- 库的新功能、bug 修复、测试都提交到 `base`；下游分支只允许 merge base，
  不允许直接向 base 提交界面代码，也不允许在 gui 分支直接修改库代码。
- gui 发现库缺陷或需要新能力：切回 base 修改并测试，再 merge 回来。

## 分支红线（历史事故教训，必须遵守）

1. **提交前必须 `git status --short` 检查暂存内容**，禁止盲目的 `git add -A`：
   - `base` 分支上出现任何 `gui/` 路径 = 错误（base 的 .gitignore 已忽略 `gui/`）；
   - `gui` 分支上出现 `gui/build/`、`gui/frontend/wailsjs/` = 错误。
   曾发生：gui 构建产物被 `git add -A` 带进 base 提交，需要二次提交清理。
2. **禁止携带未提交修改切换分支**（`stash` 后到另一分支 `stash pop`、
   或直接 checkout 带走工作区改动）。正确流程：先确定目标分支并确认
   工作区干净，再在目标分支上从零修改。多次事故均源于跨分支携带代码。
3. **库层行为问题回 base 修**：包括但不限于子进程行为（控制台黑窗、
   退出码、超时杀进程）。曾发生：在 gui 分支写了库文件再挪回 base。
4. **合并时 `AGENTS.md` / `.gitignore` 冲突的既定解法**：
   - `AGENTS.md`：以 base 版本为准（统一内容，见 Git 约定）；
   - `.gitignore`：保留 gui 分支的 `gui/build/`、`gui/frontend/wailsjs/` 条目，
     **不采纳** base 的 `gui/` 整目录规则（那会忽略 gui 分支自己的源码）。
