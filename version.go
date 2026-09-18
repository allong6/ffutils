package ffutils

import (
	"regexp"
	"strconv"
	"strings"
)

// ffmpeg 版本探测与能力标志：库内少数能力有最低版本要求（xfade 转场
// 需 4.3+，amix 的 normalize 选项需 4.4+）。探测一次缓存，能力不足处
// 自动走等价/降级路径，避免用户自配旧版 ffmpeg 时直接报错。

var ffVersionRe = regexp.MustCompile(`ffmpeg version n?(\d+)\.(\d+)`)

// Version 返回 ffmpeg 主/次版本号，首次调用时探测（ffmpeg -version
// 首行）并缓存。探测失败返回 (0,0,false)，此时能力标志按支持处理
// （工具不可用的错误会在真正执行命令时暴露，不做过度降级）。
func (f *FFmpeg) Version() (major, minor int, ok bool) {
	f.verOnce.Do(func() {
		out, err := f.run(f.ffmpegBin(), []string{"-version"})
		if err != nil {
			return
		}
		line := out
		if i := strings.IndexByte(out, '\n'); i > 0 {
			line = out[:i]
		}
		if m := ffVersionRe.FindStringSubmatch(line); m != nil {
			f.verMajor, _ = strconv.Atoi(m[1])
			f.verMinor, _ = strconv.Atoi(m[2])
			f.verOK = true
		}
	})
	return f.verMajor, f.verMinor, f.verOK
}

// HasXfade xfade 转场滤镜需 ffmpeg 4.3+。
func (f *FFmpeg) HasXfade() bool {
	maj, min, ok := f.Version()
	if !ok {
		return true
	}
	return maj > 4 || (maj == 4 && min >= 3)
}

// HasAmixNormalize amix 的 normalize 选项需 ffmpeg 4.4+。
// 旧版不传该参数，由调用方对各路音量预乘路数抵消默认的自动衰减。
func (f *FFmpeg) HasAmixNormalize() bool {
	maj, min, ok := f.Version()
	if !ok {
		return true
	}
	return maj > 4 || (maj == 4 && min >= 4)
}

// setVersionForTest 注入版本号（跳过探测），用于测试旧版兼容分支。
func (f *FFmpeg) setVersionForTest(major, minor int) {
	f.verOnce.Do(func() {})
	f.verMajor, f.verMinor, f.verOK = major, minor, true
}

// amixNormalizeSuffix amix 的 normalize 参数段（4.4+ 才有该选项）。
// 旧版返回空串，由调用方对各路音量预乘路数抵消 amix 默认的自动衰减
// （旧版行为等价 normalize=1：每路乘 1/N）。
func (f *FFmpeg) amixNormalizeSuffix() string {
	if f.HasAmixNormalize() {
		return ":normalize=0"
	}
	return ""
}
