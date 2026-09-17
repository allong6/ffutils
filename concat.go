package ffutils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Concat 用 concat demuxer 拼接多个媒体文件（要求编码参数一致）。
// 默认流式拷贝不重编码；需要重编码时传入 enc（如 VideoCodec 留空即默认 libx264）。
// copy 模式：enc.VideoCodec == "copy"。
func (f *FFmpeg) Concat(paths []string, output string, enc EncodeOptions) error {
	if len(paths) < 2 {
		return fmt.Errorf("至少需要 2 个文件才能拼接，收到 %d 个", len(paths))
	}

	listPath := output + ".concat.txt"
	listFile, err := os.Create(listPath)
	if err != nil {
		return fmt.Errorf("创建拼接列表文件失败: %w", err)
	}
	defer os.Remove(listPath)

	for _, p := range paths {
		abs, aerr := filepath.Abs(p)
		if aerr != nil {
			abs = p
		}
		// 单引号转义：concat 列表中路径用 '...' 包裹，内部 ' 写成 '\''
		escaped := fmt.Sprintf("'%s'", strings.ReplaceAll(abs, "'", `'\''`))
		if _, werr := fmt.Fprintf(listFile, "file %s\n", escaped); werr != nil {
			listFile.Close()
			return fmt.Errorf("写入拼接列表失败: %w", werr)
		}
	}
	listFile.Close()

	args := []string{"-f", "concat", "-safe", "0", "-i", listPath}
	if enc.VideoCodec == "copy" {
		args = append(args, "-c", "copy", "-y")
	} else {
		args = append(args, enc.outputArgs()...)
	}
	args = append(args, output)
	if _, err = f.run(f.ffmpegBin(), args); err != nil {
		return err
	}
	return nil
}
