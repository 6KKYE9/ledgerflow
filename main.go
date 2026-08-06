package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"ledgerflow/internal/report"
	"ledgerflow/internal/store"
	"ledgerflow/internal/ui"
)

const usage = `LedgerFlow - 个人记账与财务追踪

用法:
  ledgerflow <命令> [参数]

命令:
  add      记录一笔收支     ledgerflow add -type expense -amount 38.5 -cat 餐饮 -note 午饭
  list     查看记录         ledgerflow list [-cat 餐饮] [-type expense] [-month 2026-08] [-q 咖啡]
  summary  汇总统计         ledgerflow summary [-month 2026-08]
  month    按月趋势         ledgerflow month
  budget   设置/查看预算   ledgerflow budget -month 2026-08 -limit 3000 -alert 0.8
  edit     修改记录         ledgerflow edit <id> -amount 40 -cat 餐饮
  del      删除记录         ledgerflow del <id>
  export   导出 CSV         ledgerflow export -o ledger.csv
  help     显示帮助

示例:
  ledgerflow add -type income -amount 8000 -cat 工资 -note 月薪
  ledgerflow add -type expense -amount 12.5 -cat 交通 -note 地铁
  ledgerflow summary -month 2026-08
`

func main() {
	if len(os.Args) < 2 {
		ui.Header()
		fmt.Print(usage)
		return
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	st, err := store.New("")
	if err != nil {
		ui.Error("无法打开数据存储: " + err.Error())
		os.Exit(1)
	}

	switch cmd {
	case "add":
		cmdAdd(st, args)
	case "list":
		cmdList(st, args)
	case "summary":
		cmdSummary(st, args)
	case "month":
		cmdMonth(st)
	case "budget":
		cmdBudget(st, args)
	case "edit":
		cmdEdit(st, args)
	case "del":
		cmdDel(st, args)
	case "export":
		cmdExport(st, args)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		ui.Error("未知命令: " + cmd)
		fmt.Print(usage)
		os.Exit(1)
	}
}

func cmdAdd(st *store.Store, args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	typ := fs.String("type", "", "类型: income(收入) 或 expense(支出)")
	amount := fs.Float64("amount", 0, "金额")
	cat := fs.String("cat", "", "类别，如 餐饮/交通/工资")
	note := fs.String("note", "", "备注")
	dateStr := fs.String("date", "", "日期 2006-01-02，缺省为今天")
	_ = fs.Parse(args)

	if *typ != "income" && *typ != "expense" {
		ui.Error("请通过 -type 指定 income 或 expense")
		return
	}
	if *amount <= 0 {
		ui.Error("金额必须大于 0")
		return
	}
	if *cat == "" {
		ui.Error("请通过 -cat 指定类别")
		return
	}
	rec := st.Add(store.Type(*typ), *amount, *cat, *note, parseDate(*dateStr))
	ui.Success(fmt.Sprintf("已记录: %s %.2f (%s) [%s]", rec.Type, rec.Amount, rec.Category, rec.ID))
	maybeBudgetAlert(st, rec.Date.Format("2006-01"))
}

func cmdList(st *store.Store, args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	cat := fs.String("cat", "", "按类别筛选")
	typ := fs.String("type", "", "按类型筛选 income/expense")
	month := fs.String("month", "", "按月份筛选 2006-01")
	q := fs.String("q", "", "关键字搜索")
	_ = fs.Parse(args)
	items := st.Filter(*cat, *typ, *q, *month)
	ui.Table(items)
}

func cmdSummary(st *store.Store, args []string) {
	fs := flag.NewFlagSet("summary", flag.ExitOnError)
	month := fs.String("month", "", "按月汇总 2006-01，缺省为全部")
	_ = fs.Parse(args)
	var items []store.Transaction
	if *month == "" {
		items = st.List()
	} else {
		items = st.Filter("", "", "", *month)
	}
	s := report.Build(items)
	ui.Header()
	fmt.Printf("  %s%s\n", ui.Bold+"收入: "+ui.Reset, ui.Green+report.FormatMoney(s.Income)+ui.Reset)
	fmt.Printf("  %s%s\n", ui.Bold+"支出: "+ui.Reset, ui.Red+report.FormatMoney(s.Expense)+ui.Reset)
	fmt.Printf("  %s%s\n", ui.Bold+"结余: "+ui.Reset, balanceColor(s.Balance)+report.FormatMoney(s.Balance)+ui.Reset)
	fmt.Printf("  %s%d\n", ui.Bold+"笔数: "+ui.Reset, s.Count)
	if s.TopCategory != "" {
		fmt.Printf("  %s%s (%.2f)\n", ui.Bold+"最大支出类别: "+ui.Reset, s.TopCategory, s.ByCategory[s.TopCategory])
	}
}

func cmdMonth(st *store.Store) {
	rows := report.ByMonth(st.List())
	ui.Header()
	fmt.Printf("  %s%s%s%s\n", ui.Bold+"月份"+ui.Reset, pad(10), ui.Bold+"收入"+ui.Reset, pad(12))
	for _, r := range rows {
		fmt.Printf("  %s%s%s%s%s\n",
			r.Month,
			pad(14-len(r.Month)),
			ui.Green+report.FormatMoney(r.Income)+ui.Reset,
			pad(14-displayLen(report.FormatMoney(r.Income))),
			ui.Red+report.FormatMoney(r.Expense)+ui.Reset,
		)
	}
}

func cmdBudget(st *store.Store, args []string) {
	fs := flag.NewFlagSet("budget", flag.ExitOnError)
	month := fs.String("month", report.NowMonth(), "月份 2006-01")
	limit := fs.Float64("limit", 0, "预算上限")
	alert := fs.Float64("alert", 0.8, "提醒比例 0~1")
	_ = fs.Parse(args)
	if *limit <= 0 {
		b, ok := st.GetBudget(*month)
		if !ok {
			ui.Info("未设置 " + *month + " 的预算")
			return
		}
		bs := report.Budget(st.List(), b)
		printBudget(bs)
		return
	}
	st.SetBudget(*month, *limit, *alert)
	ui.Success(fmt.Sprintf("已设置 %s 预算上限 %.2f，提醒比例 %.0f%%", *month, *limit, *alert*100))
}

func cmdEdit(st *store.Store, args []string) {
	if len(args) < 1 {
		ui.Error("请提供记录 ID")
		return
	}
	id := args[0]
	fs := flag.NewFlagSet("edit", flag.ExitOnError)
	amount := fs.Float64("amount", -1, "新金额")
	cat := fs.String("cat", "", "新类别")
	note := fs.String("note", "", "新备注")
	dateStr := fs.String("date", "", "新日期 2006-01-02")
	_ = fs.Parse(args[1:])
	if st.Update(id, *amount, *cat, *note, parseDate(*dateStr)) {
		ui.Success("已更新 " + id)
	} else {
		ui.Error("未找到记录 " + id)
	}
}

func cmdDel(st *store.Store, args []string) {
	if len(args) < 1 {
		ui.Error("请提供记录 ID")
		return
	}
	if st.Delete(args[0]) {
		ui.Success("已删除 " + args[0])
	} else {
		ui.Error("未找到记录 " + args[0])
	}
}

func cmdExport(st *store.Store, args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	out := fs.String("o", "ledger.csv", "输出文件路径")
	_ = fs.Parse(args)
	if err := st.ExportCSV(*out, st.List()); err != nil {
		ui.Error("导出失败: " + err.Error())
		return
	}
	ui.Success("已导出到 " + *out)
}

func maybeBudgetAlert(st *store.Store, month string) {
	b, ok := st.GetBudget(month)
	if !ok {
		return
	}
	bs := report.Budget(st.List(), b)
	if bs.NeedAlert {
		ui.Warn(fmt.Sprintf("预算提醒: %s 已使用 %.0f%% (%.2f / %.2f)",
			month, bs.UsedRatio*100, bs.Spent, bs.Limit))
	}
	if bs.OverBudget {
		ui.Error(fmt.Sprintf("预算超支: %s 已超 %.2f", month, -bs.Remaining))
	}
}

func printBudget(bs report.BudgetStatus) {
	ui.Header()
	fmt.Printf("  %s%s\n", ui.Bold+"月份: "+ui.Reset, bs.Month)
	fmt.Printf("  %s%.2f\n", ui.Bold+"预算: "+ui.Reset, bs.Limit)
	fmt.Printf("  %s%.2f\n", ui.Bold+"已用: "+ui.Reset, bs.Spent)
	fmt.Printf("  %s%s%.2f\n", ui.Bold+"剩余: "+ui.Reset, balanceColor(bs.Remaining), bs.Remaining)
	fmt.Printf("  %s%.0f%%\n", ui.Bold+"使用率: "+ui.Reset, bs.UsedRatio*100)
	if bs.OverBudget {
		ui.Error("已超预算")
	}
}

func parseDate(s string) time.Time {
	if s == "" {
		return time.Now()
	}
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Now()
	}
	return d
}

func balanceColor(v float64) string {
	if v >= 0 {
		return ui.Green
	}
	return ui.Red
}

func pad(n int) string {
	if n < 0 {
		n = 0
	}
	return strings.Repeat(" ", n)
}

func displayLen(s string) int {
	return len([]rune(s))
}

var _ = strconv.Itoa
