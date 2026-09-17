# AGENTS.md

本仓库是 ffutils——一个无外部依赖的 ffmpeg/ffprobe Go 工具包。
本文档约束在本仓库中工作（修改代码、加功能、修 bug）时应遵循的规则。

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

## 项目约定

- **时间单位统一为 float64 秒**，不引入毫秒。
- **所有 ffmpeg/ffprobe 命令必须通过 `f.run()` 执行**（ffmpeg.go），它统一了
  超时杀进程、stderr 收集和错误包装。不允许直接使用 `exec.Command`。
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

## 交互约定

- 任务完成后给出简要总结即可，不要主动开启新话题或追问"还有什么需要"。
- 涉及不可逆操作（删除文件、强制推送、修改远程仓库设置）先向用户确认。
