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

（暂无——下一次功能更新写在这里，发版时更名为版本号 + 日期。）

## v1.6.0（2026-10-26）

### 行为变化

- **`DetectHardwareEncoders` 候选表按平台区分**（#7）。Windows 保持
  NVENC / NVENC HEVC / QSV / AMF 四项不变；Linux 改为
  NVENC / NVENC HEVC / QSV + VAAPI H.264（`h264_vaapi`）+
  VAAPI HEVC（`hevc_vaapi`），AMF 为 Windows-only 技术不再探测
  （此前必失败、白跑一次试编码）；其他平台回退 NVENC / NVENC HEVC /
  QSV 三项。VAAPI 试编码走 `-vaapi_device /dev/dri/renderD128` +
  `format=nv12,hwupload`，设备节点不存在（无 GPU 服务器等）时
  快速跳过 VAAPI 候选、不算错误。公开签名不变；调用方对探测结果
  按编码器取用即可，无需迁移。Since v1.6.0.

### 新增

- **`VideoCodec` 新增 `VideoVAAPI`（`h264_vaapi`）/
  `VideoVAAPI26`（`hevc_vaapi`）**（#7）：Linux 下 AMD/Intel 开源
  驱动的 VAAPI 硬件编码，命名沿用 `VideoNVENC`/`VideoNVENC26` 的
  `26`=H.265 惯例；可直接作为 `EncodeOptions.VideoCodec` 用于转码
  （VAAPI 编码需 `-vaapi_device` 前置参数，调用方经 `ExtraArgs`
  传入）。Since v1.6.0.

## v1.5.0（2026-10-19）

### 新增

- **`EnhanceOptions.V2`：快速增强 v2 内核开关**（#6）。零值 false
  保持 v1.4.0 旧链（hqdn3d → lanczos → unsharp）行为不变；
  `V2: true` 走新链：**deblock + atadenoise 修复 → lanczos 放大 →
  cas 锐化**，针对加噪/低码率素材（调用方 FFlow 同素材 A/B 实测
  全面占优）：atadenoise 时域自适应降噪（逐帧闪变的当噪点、稳定的
  当内容，不抹毛发/纹理），deblock 专治压缩块效应，cas 对比度自适
  应锐化（无过冲白边——机器实测同源过冲像素约为 unsharp 链的
  1/13，白边问题实锤收敛）。atadenoise 阈值经实测调优为
  `0a=0:0b=0.04:0s=1.5`：平坦时域噪声压制 ~87%（默认参数仅 ~23%）。
  v1 链待调用方迁移完成后废弃（至少保留两个 MINOR 版本）。Since v1.5.0.
- **`EnhanceOptions.DenoiseLevel`：降噪档位**（#6）。0=未设置（默认，
  以 `Denoise` bool 为准）、1=轻（atadenoise）、2=强（deblock +
  atadenoise，轻/强差异即 deblock 开关）；非零优先于 `Denoise`，
  `Denoise=true` 且 `DenoiseLevel=0` 兼容映射为 1；越界（<0 或 >2）
  报错。仅 v2 链分档；旧链下非零仅表开启（hqdn3d 预设不分档）。
  Since v1.5.0.
- **`EnhanceOptions.Upscaler` / `NnediWeights`：nnedi 高质量插值档**
  （#6，专业模式用）。`Upscaler` 取 `""`/`"lanczos"`（默认，零值
  行为不变）或 `"nnedi"`（神经网络边缘导向插值，边缘重建优于
  lanczos、速度低约一个量级，调用方 UI 需标注"较慢"）；非法值显式
  报错。`Upscaler="nnedi"` 时 `NnediWeights`（权重文件路径）必填，
  缺失或文件不存在显式报错、不静默回退；且需 Scale=2/4（nnedi 仅
  支持倍增，Scale=1 报错）。实现说明：ffmpeg 移植版 nnedi 是纯去
  隔行器（输出尺寸恒等于输入，没有 VapourSynth 的 dh 倍高模式），
  倍增由库内组合链实现——邻近采样铺行 + nnedi 神经网络重建插值行，
  宽度方向借转置走同一流程（transpose 1+2 恒等还原方向）；Scale=4
  倍增一次（2x）后 lanczos 放大到位（级联两遍 nnedi 代价过高）。
  权重文件：vapoursynth-nnedi3 的 `nnedi3_weights.bin`，实际体积
  **约 13.6 MB**（issue 原估"几百 KB"需修正，调用方资产分发体积
  预估请以此为准）。Since v1.5.0.

### 迁移指引（#6）

```go
// 旧写法（v1.4.0）——不传 V2 时行为完全不变，无需改动
Enhance: &EnhanceOptions{Denoise: true, Strength: 2, Scale: 2}

// 推荐写法（v2 内核：修复更强、锐化无白边；降噪档位替代 bool）
Enhance: &EnhanceOptions{V2: true, DenoiseLevel: 2, Strength: 2, Scale: 2}

// 专业模式细控：降噪轻档 + nnedi 插值（权重随资产分发）
Enhance: &EnhanceOptions{
	V2: true, DenoiseLevel: 1, Strength: 2,
	Upscaler: "nnedi", NnediWeights: "assets/nnedi3_weights.bin", Scale: 2,
}
```

## v1.4.0（2026-10-19）

### 新增

- **`VideoFilterChain.Enhance`：清晰增强**（快速增强档，#5）。
  `EnhanceOptions{Denoise, Strength, Scale}`：前置降噪（hqdn3d 固定
  预设，放大前的源噪点是伪影放大器）→ lanczos 高质量放大 → 锐化
  （unsharp 三档 0.8/1.5/2.2），一次编码完成；档位 → 滤镜参数的映射
  在库内单一事实源，调用方产品层只传档位。链内位置：适配画布之后、
  字幕与水印之前（增强作用于最终基准尺寸；锐化字幕会产生毛边、放大
  会抬升水印噪点，故不作用于叠加层）。`Scale` 为 1/2/4（<=0 默认 1，
  仅放大不缩小），与 `Fit` 叠加不互斥——Fit 定基准、Scale 在其上
  放大（Fit 640x480 + Scale=2 → 1280x960）。`Strength` 1~3（<=0
  默认 1）。Since v1.4.0.
- **`FramesToVideo`：图片序列重组视频**（AI 超清管线的重组步，#5）——
  `ExtractFrames` 的逆向。`FramesToVideo(pattern, fps, audioPath,
  output, enc)`：N 张图按 fps 重组成视频，**帧数守恒**（N 张进 →
  成片 N 帧，实测 -framerate 均匀时间戳 1:1 传递）；fps 必填由调用
  方从源视频探测传入（VFR 源调用方转 CFR）；`audioPath` 非空时以其
  音轨为成片音轨（编码走 EncodeOptions 既有语义，零值 aac、流拷贝传
  `AudioCopy`），空 = 无声成片；pattern 无匹配文件报错（printf 序号
  占位转 glob 做存在性检查，序号连续性由调用方分块流程保证）。
  分块流水口径：分块抽帧 → 外部工具放大 → 本方法统一编码重组 →
  块间 `Concat(VideoCopy)` 拼齐。Since v1.4.0.

## v1.3.0（2026-10-18）

### 新增

- **`VideoFilterChain.Cover`：首帧覆盖封面**（#4）。多数平台/本地缩略图
  取视频第 1 帧做封面，首帧黑屏/虚焦时用自选封面图整幅替换首帧。
  `CoverOptions{Path, Fit, PadColor}`：封面与画面宽高比不一致时按
  `FitMode` 适配（crop 默认/pad 补边/stretch 拉伸，复用画布适配语义）；
  适配目标为**链后有效画面尺寸**（裁剪/旋转/适配画布之后，复用
  v1.2.0 ScalePct 建立的尺寸推导，现抽取为 effectiveMainSize 宽高对）。
  生效条件用帧号表达式 `enable='not(n)'`——只盖第 1 帧，不依赖帧率
  探测，变帧率视频天然稳定（ffmpeg 4.4 与 8.x 实测可用）。封面段在
  链尾最后执行（水印与淡入淡出之后）：封面帧纯净、不被水印盖住、
  不受淡入影响。音频不动、总时长不变。Since v1.3.0.
- **`PrependCover`：片头插入封面静帧**（#4）。总时长增加 frames 帧
  （库内按探测帧率换算时长），原内容一帧不少——需要"封面停留一段
  时间"的场景用它（与只换首帧、时长不变的 Cover 互补）。封面按主
  画面原始尺寸适配（同一 `CoverOptions`）；封面段与原视频经
  filter_complex 一次编码拼接（单遍出片，规避分段编码参数不一致的
  花屏）。有音轨时插入段以 anullsrc 垫等长静音（采样率/声道/时基与
  源对齐），音画起点一致；源无音轨则成片同样无音轨。`frames <=0`
  默认 15（约半秒）。Since v1.3.0.

## v1.2.0（2026-10-17）

### 新增

- **`Watermark.ScalePct`：水印按主画面宽的百分比缩放**（#3）。0 =
  原始尺寸（现状不变），有效范围 1~100；50 表示水印宽为主画面宽的
  50%，高度按宽高比自适应（`scale=n:-2`）。与 PosTile 互斥（平铺已有
  固定的 1/4 宽缩放）。主画面宽由库按各路径链序推导（Fit 画布 >
  裁剪+旋转换算 > 探测原始宽 > 已知画布/输出宽），调用方只传语义
  百分比、无需自行换算像素。四条水印路径（AddWatermark /
  ApplyVideoFilters / MultiCompose / NewFrameWriter）同步生效。
  Since v1.2.0.
- **`Watermark.Motion`：水印动态轨迹**（防盗用动态水印，#3），新类型
  `Motion{Kind, Interval, TX, TY, Period, Axis}` 与枚举
  `MotionKind`（`MotionRotate` 四角轮换 / `MotionBounce` 三角波往返 /
  `MotionDrift` 单向穿越回绕）、`DriftAxis`（水平/垂直）。nil 或
  `Kind` 空 = 静态（现状不变）。轨迹用 overlay 的 x/y 时间表达式
  逐帧求值，渲染成本与静态几乎相同；时间参数一律秒（周期/间隔语义，
  分辨率无关），未参与的轴向锚定 `Position`。`ScalePct` 可与 Motion
  自由组合（w/h 运行时取缩放后的值）。校验：未知 Kind/Axis、
  Motion 或 ScalePct 与 PosTile 组合、bounce 双轴同时 `<=0`、时间
  参数为负均在入口报错。四条水印路径同步生效。Since v1.2.0.

### 修复

- **NewFrameWriter（FrameWriter）的水印透明度被静默忽略**：非平铺
  水印路径此前不处理 `Watermark.Opacity`（拼滤镜时直接丢弃），
  0~1 之间的取值实际完全不透明。自 v1.2.0 起与其他三条水印路径
  一致生效。**行为变化**：此前依赖"FrameWriter 水印恒不透明"的
  调用方如需保持旧观感，把 Opacity 留 0（或 >=1）即可。

### 内部（不影响 API）

- 四条水印路径各自手拼的水印 filter_graph 分段收拢为统一的
  `watermarkSegments`（私有），零值字段时产出与 v1.1.0 逐段一致；
  入口校验统一为 `validateWatermark`。
- 测试素材批次更新（1/2/3.mp4 → 01_seal 等 7 段）后的测试基线迁移：
  引用与时长/分辨率断言对齐新素材（仅测试代码，不影响库）。

## v1.1.0（2026-09-30）

### 新增

- `Watermark.TileGap`：平铺水印（PosTile）格子的透明间隔（像素），
  `<=0` 无间隔（与现状一致）。格子宽度固定为主画面 1/4，间隔从水印
  内容宽度中扣（网格总尺寸不变）；间隔大到挤没水印内容时报错。
  Since v1.1.0.

### 修复

- **PosTile 平铺水印仅左上角第一格有水印、其余 11 格为黑色填充**
  （#1）：水印图片是单帧流，`tile` 滤镜每格消费一帧。现 tile 前
  `loop` 补足 12 帧，12 格全部有水印。影响 AddWatermark /
  ApplyVideoFilters / MultiCompose / **NewFrameWriter** 四条路径
  （issue 报告三条，推帧路径同样受累）。**平铺输出视觉变化明显**：
  从"1 格水印 + 黑格"变为 12 格全水印。
- **ApplyVideoFilters 的淡入淡出不作用于水印与字幕**（#2）：fade
  此前拼在主画面段，水印叠加与字幕烧制都在其后——淡出时画面渐暗、
  水印与字幕保持鲜亮。现 fade 恒在链尾作用于成片整体：淡出时画面、
  字幕、水印一起变暗消失（字幕语义经确认：随成片淡出）。链序变为
  裁剪→旋转→适配→字幕→水印→淡入淡出；纯 Fade（无水印/字幕）输出
  不变。

## v1.0.0（2026-09-29）：独立成库

从旧仓库 Ffmpeg_utils（本地工作副本 video_generate）base 分支独立成
单独的库仓库 [github.com/allong6/ffutils](https://github.com/allong6/ffutils)，
git 历史完整保留。基线能力与 API 总览见 [README](../README.md)。

### 仓库内资产（不影响库 API）

- `tools/build-ffmpeg` 裁剪构建工具（自 ffbox 迁入）：与本库能力面对齐
  的 ffmpeg/ffprobe 白名单自编译（encoder/muxer 白名单来自 enums.go
  全量枚举，输入面全保留），产物供 FFBox 打包内嵌分发。
- 文档体系：[docs/API.md](API.md) 功能方法清单、
  [docs/ARCHITECTURE.md](ARCHITECTURE.md) 纯库架构、README 纯库化
  （清除旧仓库 gui/license 遗留章节）。

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
