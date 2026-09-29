# 裁剪版 ffmpeg 构建(ffutils 能力面构建)

自编译与 ffutils/FFBox 现行版本同线的 ffmpeg/ffprobe:encoder/muxer 白名单
裁剪,decoder/demuxer/filter 全保留(输入面承诺不砍)。体积约为通用
essentials 构建的 70%,能力清单与 ffutils API 面(白名单来源即 enums.go
全量枚举)完全对齐。2026-09-29 自 ffbox 仓库迁入本仓(裁剪语义属库能力面,
FFBox 打包从本目录取产物内嵌)。

## 产物

- `work/out/bin/ffmpeg.exe`、`ffprobe.exe`(work/ 已 gitignore,产物不入库)
- `work/out/manifest.json`:构建事实凭据——ffmpeg 版本、完整 configure 行、
  双 exe 的 SHA-256 与体积、全部外部库版本、构建日期。
  用途:内嵌/分发的 hash 校验源、GPL 合规披露(configure 行即裁剪声明)、
  升级比对新旧差异。验证结论(test-all/矩阵)不写进 manifest,记录在
  本文件"验证记录"与 ffbox 仓 docs/STATUS.md。
- 仓库根的 `manifest.json` 是**最近一次通过全量验证的基线快照**(提交时从
  work/out/ 复制);每次构建会重新生成到 work/out/,验证通过后手动覆盖快照。

## 使用

前置:Windows x64 + [MSYS2](https://www.msys2.org/)(装到任意盘均可,
C 盘紧张建议装 F 盘)。建议先切国内镜像(可选,见下)。

```bash
# MSYS2 mingw64 shell 中:
/tools/build-ffmpeg/build-trim.sh          # 首次运行自动装依赖+下载源码
# 产物与 manifest 在 tools/build-ffmpeg/work/out/
```

从 Git Bash/CI 一键调用(免开 MSYS2 终端):

```bash
MSYSTEM=MINGW64 /f/msys64/usr/bin/bash.exe -lc '/f/workspace/ffutils/tools/build-ffmpeg/build-trim.sh'
```

脚本幂等:依赖用 `pacman --needed`,源码/头文件存在即复用,重复运行增量编译。

镜像切换(可选,国内提速):编辑 `/etc/pacman.d/mirrorlist.mingw` 与
`mirrorlist.msys`,删除 `mirror.msys2.org` 行使清华/中科大等源置顶。

## 验证流程(改动 configure 白名单后必须执行)

验证在**同级 ffbox 仓库**进行(矩阵驱动 FFBox CLI,两仓 bin/ 指向同一共享目录):

1. 产物换入 `bin/`(先备份):`cp bin/ffmpeg.exe bin/ffmpeg.bak` 等
2. ffbox 仓 `powershell -File gui/scripts/test-all.ps1`(L1-L3)
3. 三个矩阵:`node gui/scripts/{convert,video,pages}-matrix.mjs`(共 123 项)
4. 恢复 `bin/` 原版
5. 验证结论追加到本文件"验证记录",并同步 ffbox 仓 docs/STATUS.md

## 验证记录

- **2026-09-23(正式资产化复验)**:本目录脚本在全新 work/ 目录完整重建
  (manifest 快照即此构建,ffmpeg sha256 前缀 762a148c),test-all 全绿 +
  三矩阵 44+49+30=123 项全过。两次构建 hash 不一致(嵌入路径/PE 时间戳差异,
  同配置同尺寸),**验证结论绑定 manifest 记录的 hash**。
- **2026-09-23(基线首验)**:ffmpeg 8.1.2 裁剪基线首次全量验证——test-all
  全绿,三矩阵 123 项全过;白名单缺口(libass/hls/network)由第一轮矩阵暴露
  后补齐。硬件编码 4 家已编入,本机无 N 卡/QSV,真机行为待 GPU 机器回归。
  二进制 72.6MB/72.4MB(vs essentials 各 101MB)。

## 踩坑记录(改 configure 前必读)

1. **lavfi 输入属于 avdevice**:`-f lavfi testsrc`(硬件探测命脉)依赖 lavfi
   indev,`--disable-avdevice` 会杀掉它。正解:`--disable-devices --enable-indev=lavfi`。
2. **nvenc 属 autodetect 家族**:`--disable-autodetect` 下必须显式 `--enable-nvenc`,
   否则 encoder 白名单里的 h264_nvenc/hevc_nvenc 被静默丢弃。
3. **PCM 封装改名**:ffmpeg 8.x 起 `s16le` → `pcm_s16le`(旧名只是别名),
   白名单写旧名不报错但不生效。
4. **白名单必须覆盖的隐性依赖**:libass(字幕烧录 subtitles 滤镜)、
   hls+segment 封装(ToHLS/FromHLS)、network 不能禁(FromHLS 支持 http URL)。
   这些是第一轮矩阵验证暴露的真缺口,靠测试兜底,不靠记忆。
5. **MSYS2 打包瑕疵两处**(脚本已自动修补):libvpl.a 含 C++ 对象但 vpl.pc
   缺 `-lstdc++`;x265.pc 的 Libs.private 带 `-lgcc_s`,与 `-static` 冲突
   (`_Unwind_Resume` 重复定义)。
6. **configure 偶发失败** `cannot open /tmp/.../test.exe: Invalid argument`
   = 杀软/文件系统抖动,重跑一次即过,不是配置问题。
7. **GPL 合规**:构建含 libx264/libx265(GPL),产物二进制为 GPL。以进程
   边界调用不传染 FFBox 主程序;对外分发时以 manifest 的 configure 行 +
   库版本作为对应源码的披露凭据。
