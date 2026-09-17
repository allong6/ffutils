package ffutils

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// VideoStream 描述视频流信息，无视频流时为 nil。
type VideoStream struct {
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Duration  float64 `json:"duration"`    // 秒
	FrameRate float64 `json:"frame_rate"`  // 帧率（已换算，如 29.97）
	Bitrate   int     `json:"bit_rate"`    // bps
	Codec     string  `json:"codec_name"`
}

// AudioStream 描述音频流信息，无音频流时为 nil。
type AudioStream struct {
	Duration   float64 `json:"duration"`  // 秒
	Bitrate    int     `json:"bit_rate"`  // bps
	SampleRate int     `json:"sample_rate"`
	Channels   int     `json:"channels"`
	Codec      string  `json:"codec_name"`
}

// ProbeResult 一次探测得到的媒体信息。
type ProbeResult struct {
	// Duration 容器总时长（秒），取自 format 节点
	Duration float64
	HasVideo bool
	HasAudio bool
	Video    *VideoStream
	Audio    *AudioStream
}

// probeStream 是 ffprobe -show_streams 输出的原始结构（仅保留需要的字段）。
type probeStream struct {
	CodecType    string `json:"codec_type"`
	CodecName    string `json:"codec_name"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Duration     string `json:"duration"`
	BitRate      string `json:"bit_rate"`
	RFrameRate   string `json:"r_frame_rate"`
	AvgFrameRate string `json:"avg_frame_rate"`
	SampleRate   string `json:"sample_rate"`
	Channels     int    `json:"channels"`
}

// Probe 探测媒体文件信息。一次调用同时返回容器时长、视频流、音频流信息，
// 避免旧版 GetAVDuration/GetVADuration/HasAudio/GetVideoInfo 各起一个进程。
// 用 -of json 输出解析（结构稳定），而不是旧版脆弱的 csv 切分。
// 注意：只取第一路视频/音频流；流级 duration 缺失（部分 mkv/webm）时回退容器时长。
func (f *FFmpeg) Probe(path string) (*ProbeResult, error) {
	out, err := f.run(f.ffprobeBin(), []string{
		"-v", "error",
		"-show_entries",
		"format=duration:stream=codec_type,codec_name,width,height,duration,bit_rate,r_frame_rate,avg_frame_rate,sample_rate,channels",
		"-of", "json",
		path,
	})
	if err != nil {
		return nil, err
	}

	var raw struct {
		Streams []probeStream `json:"streams"`
		Format  struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("解析 ffprobe 输出失败: %w", err)
	}

	res := &ProbeResult{}
	res.Duration, _ = strconv.ParseFloat(strings.TrimSpace(raw.Format.Duration), 64)

	for _, s := range raw.Streams {
		switch s.CodecType {
		case "video":
			if res.Video != nil {
				continue // 只取第一路视频流
			}
			v := &VideoStream{
				Width:   s.Width,
				Height:  s.Height,
				Bitrate: atoiSafe(s.BitRate),
				Codec:   s.CodecName,
			}
			v.Duration = atofSafe(s.Duration)
			if v.Duration == 0 {
				v.Duration = res.Duration
			}
			// avg_frame_rate 是真实平均帧率；r_frame_rate 常为时间基虚标值(如 90000)
			v.FrameRate = parseFrameRate(s.AvgFrameRate)
			if v.FrameRate <= 0 || v.FrameRate > 240 {
				v.FrameRate = parseFrameRate(s.RFrameRate)
			}
			res.Video, res.HasVideo = v, true
		case "audio":
			if res.Audio != nil {
				continue // 只取第一路音频流
			}
			a := &AudioStream{
				Bitrate:    atoiSafe(s.BitRate),
				SampleRate: atoiSafe(s.SampleRate),
				Channels:   s.Channels,
				Codec:      s.CodecName,
			}
			a.Duration = atofSafe(s.Duration)
			if a.Duration == 0 {
				a.Duration = res.Duration
			}
			res.Audio, res.HasAudio = a, true
		}
	}
	return res, nil
}

func (f *FFmpeg) ffprobeBin() string {
	if f.FFprobePath != "" {
		return f.FFprobePath
	}
	return "ffprobe"
}

func (f *FFmpeg) ffmpegBin() string {
	if f.FFmpegPath != "" {
		return f.FFmpegPath
	}
	return "ffmpeg"
}

// parseFrameRate 解析 r_frame_rate（如 "30000/1001"），无理数时返回 0。
func parseFrameRate(s string) float64 {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, "/")
	if len(parts) == 2 {
		num, err1 := strconv.ParseFloat(parts[0], 64)
		den, err2 := strconv.ParseFloat(parts[1], 64)
		if err1 == nil && err2 == nil && den != 0 {
			return num / den
		}
		return 0
	}
	return atofSafe(s)
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func atofSafe(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}
