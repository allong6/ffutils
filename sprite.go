package ffutils

import "fmt"

// SpriteResult 雪碧图布局信息（不依赖外部类型）。
type SpriteResult struct {
	// FrameWidth/FrameHeight 单帧尺寸（像素）
	FrameWidth  int `json:"frame_width"`
	FrameHeight int `json:"frame_height"`
	// Cols/Rows 列数 / 行数
	Cols int `json:"cols"`
	Rows int `json:"rows"`
	// Frames 实际采样的帧数
	Frames int `json:"frames"`
	// Interval 每隔多少帧取一帧
	Interval int `json:"interval"`
}

// SpriteOptions 雪碧图参数。
type SpriteOptions struct {
	// Output 输出图片路径（通常为 jpg）
	Output string
	// FrameMax 单帧最长边像素预算，默认 160
	FrameMax int
	// SheetMax 整张雪碧图单边像素预算（总帧数与布局据此计算），默认 6144
	SheetMax int
}

// GenerateSpriteSheet 按均匀间隔采样生成雪碧图，并返回布局信息供播放器换算。
func (f *FFmpeg) GenerateSpriteSheet(path string, opts SpriteOptions) (*SpriteResult, error) {
	frameMax := opts.FrameMax
	if frameMax <= 0 {
		frameMax = 160
	}
	sheetMax := opts.SheetMax
	if sheetMax <= 0 {
		sheetMax = 6144
	}

	info, err := f.Probe(path)
	if err != nil {
		return nil, fmt.Errorf("探测视频失败: %w", err)
	}
	if !info.HasVideo || info.Video == nil {
		return nil, fmt.Errorf("%s 没有视频流", path)
	}
	v := info.Video

	// 单帧宽度：在 (frameMax-50, frameMax] 内取能整除原宽的值，避免缩放畸变
	width := frameMax
	for i := frameMax; i > frameMax-50; i-- {
		if v.Width%i == 0 {
			width = i
			break
		}
	}
	height := v.Height * width / v.Width
	if height > frameMax { // 竖屏视频按高度约束
		height = frameMax
		width = v.Width * height / v.Height
	}

	cols := sheetMax / width
	if cols < 1 {
		cols = 1
	}
	rows := sheetMax / height
	if rows < 1 {
		rows = 1
	}

	totalFrames := int(v.Duration * v.FrameRate)
	if totalFrames < 1 {
		totalFrames = 1
	}
	maxFrames := cols * rows
	interval := totalFrames / maxFrames
	if interval < 1 {
		interval = 1
	}
	frames := totalFrames / interval
	if frames < 1 {
		frames = 1
	}
	rows = (frames + cols - 1) / cols

	args := []string{
		"-i", path,
		"-vf", fmt.Sprintf("select=not(mod(n\\,%d)),scale=%d:%d,tile=%dx%d", interval, width, height, cols, rows),
		"-frames:v", "1", "-y",
		opts.Output,
	}
	if _, err := f.run(f.ffmpegBin(), args); err != nil {
		return nil, err
	}
	return &SpriteResult{
		FrameWidth:  width,
		FrameHeight: height,
		Cols:        cols,
		Rows:        rows,
		Frames:      frames,
		Interval:    interval,
	}, nil
}
