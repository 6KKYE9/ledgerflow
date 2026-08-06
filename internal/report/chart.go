package report

import (
	"fmt"
	"strings"
)

// Bar 单个柱状图条目。
type Bar struct {
	Label string
	Value float64
	Color string
}

// RenderBars 在终端绘制简单的文本柱状图。
// maxWidth 为柱子最大高度（字符行数）。
func RenderBars(bars []Bar, maxWidth int) string {
	if len(bars) == 0 {
		return "  (无数据)"
	}
	var maxVal float64
	for _, b := range bars {
		if b.Value > maxVal {
			maxVal = b.Value
		}
	}
	if maxVal <= 0 {
		maxVal = 1
	}
	var sb strings.Builder
	for _, b := range bars {
		h := int((b.Value / maxVal) * float64(maxWidth))
		if b.Value > 0 && h == 0 {
			h = 1
		}
		fmt.Fprintf(&sb, "  %s\n", b.Label)
		for i := 0; i < h; i++ {
			sb.WriteString("  " + b.Color + "█" + reset + "\n")
		}
		fmt.Fprintf(&sb, "  %s%.2f%s\n", b.Color, b.Value, reset)
		sb.WriteString("\n")
	}
	return sb.String()
}

const reset = "\033[0m"
