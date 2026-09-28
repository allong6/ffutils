package ffutils

// 本文件集中定义所有枚举参数。类型均基于 string，字段零值（""）表示
// "由方法按上下文决定默认值"，常量仅覆盖常用值——ffmpeg 支持但未列出
// 的值仍可直接用字符串字面量赋值。

// ---------------------------------------------------------------------------
// xfade 转场类型（Transition.Type）
// ---------------------------------------------------------------------------

// TransitionType xfade 滤镜支持的转场效果。
type TransitionType string

// 淡入淡出类
const (
	Fade      TransitionType = "fade"      // 交叉淡化（最常用）
	FadeBlack TransitionType = "fadeblack" // 经黑场过渡
	FadeWhite TransitionType = "fadewhite" // 经白场过渡
	Dissolve  TransitionType = "dissolve"  // 溶解
	FadeGrays TransitionType = "fadegrays" // 灰度溶解
	HBlur     TransitionType = "hblur"     // 水平模糊过渡
	Pixelize  TransitionType = "pixelize"  // 像素化过渡
	Radial    TransitionType = "radial"    // 径向擦除
	Distance  TransitionType = "distance"  // 距离渐变
)

// 擦除（wipe）类
const (
	WipeLeft  TransitionType = "wipeleft"
	WipeRight TransitionType = "wiperight"
	WipeUp    TransitionType = "wipeup"
	WipeDown  TransitionType = "wipedown"
	WipeTL    TransitionType = "wipetl" // 从左上角开始的扇形擦除
	WipeTR    TransitionType = "wipetr"
	WipeBL    TransitionType = "wipebl"
	WipeBR    TransitionType = "wipebr"
)

// 滑动（slide）类：新画面从对应方向推入
const (
	SlideLeft  TransitionType = "slideleft"
	SlideRight TransitionType = "slideright"
	SlideUp    TransitionType = "slideup"
	SlideDown  TransitionType = "slidedown"
)

// 平滑滑动（smooth）类：带缓动的推入
const (
	SmoothLeft  TransitionType = "smoothleft"
	SmoothRight TransitionType = "smoothright"
	SmoothUp    TransitionType = "smoothup"
	SmoothDown  TransitionType = "smoothdown"
)

// 圆形/矩形开合类
const (
	CircleCrop  TransitionType = "circlecrop" // 圆形收缩/展开
	CircleOpen  TransitionType = "circleopen"
	CircleClose TransitionType = "circleclose"
	RectCrop    TransitionType = "rectcrop" // 矩形收缩/展开
	VertOpen    TransitionType = "vertopen" // 垂直百叶窗展开
	VertClose   TransitionType = "vertclose"
	HorzOpen    TransitionType = "horzopen" // 水平百叶窗展开
	HorzClose   TransitionType = "horzclose"
)

// 切片（slice）类
const (
	HLSlice TransitionType = "hlslice" // 水平切片进入
	HRSlice TransitionType = "hrslice"
	VUSlice TransitionType = "vuslice" // 垂直切片进入
	VDSlice TransitionType = "vdslice"
)

// ---------------------------------------------------------------------------
// 水印 / 叠加位置（Watermark.Position）
// ---------------------------------------------------------------------------

// Position 水印在画面中的预设位置，Margin 控制距边缘的距离。
type Position string

const (
	PosTopLeft     Position = "topleft"
	PosTopRight    Position = "topright"
	PosBottomLeft  Position = "bottomleft"
	PosBottomRight Position = "bottomright" // 默认
	PosTile        Position = "tile"        // 平铺：水印缩到约 1/4 宽后 4×3 平铺满画面（防盗用/批量标记）
)

// ---------------------------------------------------------------------------
// 画面适配模式（VideoFilterChain.Fit / XfadeOptions.Fit / GridOptions.Fit /
// MultiComposeOptions.Fit / OverlayLayer.Fit）
// ---------------------------------------------------------------------------

// FitMode 把画面塞进固定尺寸画布时的适配方式（输入宽高比与目标不一致时生效）。
//
// 库内统一规则：凡是把画面强制成某个固定 W×H 矩形的地方都认这个枚举；
// 只指定一边、另一边自适应的调用（TranscodeOptions 只给 Width/Height 之一、
// PiPOptions.Scale 推出的 scale=W:-2）天然等比，不受影响。
//
// 零值（""）表示"由调用方法按上下文决定默认值"，当前各处默认均为 FitCrop。
type FitMode string

const (
	// FitStretch 拉伸填满：直接缩放到目标尺寸，不保持宽高比（会变形）。
	// 这是 2026-09 之前各合成路径的固有行为，显式传入可还原旧观感。
	FitStretch FitMode = "stretch"
	// FitCrop 裁剪填满（默认）：等比放大到铺满画布后居中裁剪，不变形、
	// 不留边，代价是超出画布的部分被裁掉。横屏素材进竖屏画布时即"裁两边"。
	FitCrop FitMode = "crop"
	// FitPad 完整显示：等比缩放到整幅可见后居中补边，不变形、不裁剪，
	// 代价是画布留边（黑边）。横竖屏互换想保住全部画面时用它。
	FitPad FitMode = "pad"
)

// ---------------------------------------------------------------------------
// 视频编码器（EncodeOptions.VideoCodec / TranscodeOptions.VideoCodec）
// ---------------------------------------------------------------------------

// VideoCodec 常用视频编码器。"copy" 表示流拷贝不重编码（EncodeOptions
// 零值默认 libx264；TranscodeOptions 零值按输出扩展名推断）。
type VideoCodec string

const (
	VideoCopy    VideoCodec = "copy"       // 流拷贝，不重编码
	VideoH264    VideoCodec = "libx264"    // H.264，兼容性最好（默认）
	VideoH265    VideoCodec = "libx265"    // H.265/HEVC，同质量体积更小
	VideoVP9     VideoCodec = "libvpx-vp9" // WebM
	VideoVP8     VideoCodec = "libvpx"     // WebM（旧）
	VideoTheora  VideoCodec = "libtheora"  // Ogg
	VideoGIF     VideoCodec = "gif"        // 动图
	VideoProRes  VideoCodec = "prores_ks"  // Apple ProRes（剪辑中间格式）
	VideoNVENC   VideoCodec = "h264_nvenc" // NVIDIA 硬件编码
	VideoNVENC26 VideoCodec = "hevc_nvenc" // NVIDIA 硬件 H.265
	VideoQSV     VideoCodec = "h264_qsv"   // Intel 核显硬件编码
	VideoAMF     VideoCodec = "h264_amf"   // AMD 硬件编码
)

// ---------------------------------------------------------------------------
// 音频编码器（EncodeOptions.AudioCodec / TranscodeOptions.AudioCodec /
// AudioExtractOptions.Codec）
// ---------------------------------------------------------------------------

// AudioCodec 常用音频编码器。"copy" 表示不重编码。
type AudioCodec string

const (
	AudioCopy   AudioCodec = "copy"       // 流拷贝，不重编码
	AudioAAC    AudioCodec = "aac"        // AAC，兼容性最好（默认）
	AudioMP3    AudioCodec = "libmp3lame" // MP3
	AudioOpus   AudioCodec = "libopus"    // Opus（WebM 默认）
	AudioVorbis AudioCodec = "libvorbis"  // Vorbis（Ogg）
	AudioAC3    AudioCodec = "ac3"        // 杜比数字（环绕声）
	AudioPCM    AudioCodec = "pcm_s16le"  // 无损 PCM（WAV）
	AudioFLAC   AudioCodec = "flac"       // 无损压缩
)

// ---------------------------------------------------------------------------
// x264/x265 编码速度预设（EncodeOptions.Preset / TranscodeOptions.Preset）
// ---------------------------------------------------------------------------

// Preset 编码速度/压缩效率权衡：越快体积越大、质量略降。
type Preset string

const (
	PresetUltrafast Preset = "ultrafast" // 最快（实时推帧等场景默认）
	PresetSuperfast Preset = "superfast"
	PresetVeryfast  Preset = "veryfast"
	PresetFaster    Preset = "faster"
	PresetFast      Preset = "fast"
	PresetMedium    Preset = "medium" // x264 默认，画质/速度均衡（转码默认）
	PresetSlow      Preset = "slow"
	PresetSlower    Preset = "slower"
	PresetVeryslow  Preset = "veryslow" // 最慢，同质量体积最小
)
