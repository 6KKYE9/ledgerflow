package ui

import (
	"fmt"
	"strings"

	"ledgerflow/internal/report"
	"ledgerflow/internal/store"
)

// 颜色转义码（导出供主程序复用）
const (
	Reset  = "\033[0m"
	Bold   = "\033[1m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Cyan   = "\033[36m"
	Gray   = "\033[90m"
)

// Header 打印程序标题。
func Header() {
	fmt.Println(Bold + Cyan + "  LedgerFlow" + Reset + Gray + "  · 个人记账与财务追踪" + Reset)
	fmt.Println(Gray + "  ------------------------------" + Reset)
}

// Info 输出普通信息。
func Info(msg string) {
	fmt.Println(Gray + "  " + msg + Reset)
}

// Success 输出成功信息。
func Success(msg string) {
	fmt.Println(Green + "  ✓ " + msg + Reset)
}

// Warn 输出警告信息。
func Warn(msg string) {
	fmt.Println(Yellow + "  ! " + msg + Reset)
}

// Error 输出错误信息。
func Error(msg string) {
	fmt.Println(Red + "  ✗ " + msg + Reset)
}

// Table 打印交易记录表格。
func Table(items []store.Transaction) {
	if len(items) == 0 {
		Info("暂无记录")
		return
	}
	cols := []string{"日期", "类型", "金额", "类别", "标签", "备注", "ID"}
	widths := columnWidths(items, cols)
	printDivider(widths)
	printRow(widths, cols, true)
	printDivider(widths)
	for _, t := range items {
		typ := "收入"
		color := Green
		sign := "+"
		if t.Type == "expense" {
			typ = "支出"
			color = Red
			sign = "-"
		}
		amount := color + fmt.Sprintf("%s%.2f", sign, t.Amount) + Reset
		date := t.Date.Format("01-02")
		tags := strings.Join(t.Tags, "、")
		printCells(widths, []string{date, typ, amount, t.Category, tags, t.Note, t.ID})
	}
	printDivider(widths)
	fmt.Printf(Gray+"  共 %d 条记录\n"+Reset, len(items))
}

func columnWidths(items []store.Transaction, cols []string) []int {
	w := make([]int, len(cols))
	for i, c := range cols {
		w[i] = displayWidth(c)
	}
	for _, t := range items {
		check := []string{
			t.Date.Format("01-02"),
			func() string {
				if t.Type == "expense" {
					return "支出"
				}
				return "收入"
			}(),
			fmt.Sprintf("%+.2f", t.Amount),
			t.Category, strings.Join(t.Tags, "、"), t.Note, t.ID,
		}
		for i, v := range check {
			if d := displayWidth(v); d > w[i] {
				w[i] = d
			}
		}
	}
	return w
}

func printDivider(w []int) {
	var sb strings.Builder
	sb.WriteString(Gray + "  +")
	for _, c := range w {
		sb.WriteString(strings.Repeat("-", c+2))
		sb.WriteString("+")
	}
	sb.WriteString(Reset)
	fmt.Println(sb.String())
}

func printRow(w []int, cols []string, _ bool) {
	printCells(w, cols)
}

func printCells(w []int, cells []string) {
	var sb strings.Builder
	sb.WriteString(Gray + "  |" + Reset)
	for i, c := range cells {
		pad := w[i] - displayWidth(c)
		if pad < 0 {
			pad = 0
		}
		sb.WriteString(" " + c + strings.Repeat(" ", pad) + " " + Gray + "|" + Reset)
	}
	fmt.Println(sb.String())
}

// displayWidth 计算字符串显示宽度（中文按 2 计）。
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		if r > 0x2E7F { // 粗略判断为宽字符（CJK 等）
			n += 2
		} else {
			n++
		}
	}
	return n
}

// Categories 打印类别清单。
func Categories(groups map[string][]string) {
	ui := func(title string, items []string, color string) {
		if len(items) == 0 {
			return
		}
		fmt.Println(color + "  " + title + Reset + ": " + strings.Join(items, "、"))
	}
	ui("收入", groups["income"], Green)
	ui("支出", groups["expense"], Red)
}

// PrintStats 打印整体统计概览。
func PrintStats(st store.Stats) {
	Header()
	if st.Count == 0 {
		Info("暂无数据，先用 add 记录一笔吧")
		return
	}
	fmt.Printf("  %s%d\n", Bold+"记录数: "+Reset, st.Count)
	fmt.Printf("  %s%s\n", Bold+"总收入: "+Reset, Green+report.FormatMoney(st.Income)+Reset)
	fmt.Printf("  %s%s\n", Bold+"总支出: "+Reset, Red+report.FormatMoney(st.Expense)+Reset)
	fmt.Printf("  %s%s%s\n", Bold+"净结余: "+Reset, balanceColor(st.Balance), report.FormatMoney(st.Balance)+Reset)
	fmt.Printf("  %s%d 天\n", Bold+"记账天数: "+Reset, st.Days)
	fmt.Printf("  %s%s 元/天\n", Bold+"日均支出: "+Reset, report.FormatMoney(st.AvgExpensePerDay))
	fmt.Printf("  %s%s ~ %s\n", Bold+"首笔→末笔: "+Reset, st.FirstDate.Format("2006-01-02"), st.LastDate.Format("2006-01-02"))
}

// PrintTop 打印支出类别排行。
func PrintTop(ranks []report.CategoryRank) {
	Header()
	if len(ranks) == 0 {
		Info("暂无支出记录")
		return
	}
	Info("支出类别排行（金额 / 笔数）")
	var max float64
	for _, r := range ranks {
		if r.Amount > max {
			max = r.Amount
		}
	}
	if max <= 0 {
		max = 1
	}
	for i, r := range ranks {
		h := int((r.Amount / max) * 10)
		if r.Amount > 0 && h == 0 {
			h = 1
		}
		bar := strings.Repeat("█", h)
		rank := fmt.Sprintf("%d.", i+1)
		fmt.Printf("  %s%s%s %s   %s  (%d笔)\n",
			Yellow+rank+Reset,
			pad(2-displayWidth(rank)),
			Red+bar+Reset,
			r.Category,
			report.FormatMoney(r.Amount),
			r.Count,
		)
	}
}

func balanceColor(v float64) string {
	if v >= 0 {
		return Green
	}
	return Red
}

func pad(n int) string {
	if n < 0 {
		n = 0
	}
	return strings.Repeat(" ", n)
}

