package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
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
  add        记录一笔收支     ledgerflow add -type expense -amount 38.5 -cat 餐饮 -note 午饭 -tag 日常
  list       查看记录         ledgerflow list [-cat 餐饮] [-type expense] [-month 2026-08] [-q 咖啡] [-tag 旅行]
  recent     最近记录         ledgerflow recent [-n 10]
  summary    汇总统计         ledgerflow summary [-month 2026-08] [-tag 旅行]
  tagsum     按标签统计收支   ledgerflow tagsum [-month 2026-08]
  stats      整体总览         ledgerflow stats
  balance    当前总余额       ledgerflow balance
  top        支出排行         ledgerflow top [-n 5]
  month      按月趋势         ledgerflow month
  week       本周收支汇总     ledgerflow week
  chart      收支柱状图       ledgerflow chart
  categories 查看类别         ledgerflow categories
  tags       查看所有标签     ledgerflow tags
  tag        移除记录上的标签 ledgerflow tag -rm 旅行 <id...>
  budget     设置/查看预算   ledgerflow budget -month 2026-08 -limit 3000 -alert 0.8
  budget     查看全部月份预算 ledgerflow budget -list
  edit       修改记录         ledgerflow edit <id> -amount 40 -cat 餐饮 -tag 旅行
  rename     类别改名         ledgerflow rename -from 餐饮 -to 吃饭
  del        删除记录         ledgerflow del <id> | ledgerflow del --all --yes
  import     导入 CSV 数据    ledgerflow import -o data.csv
  export     导出数据         ledgerflow export -o data.csv [-f csv|json]
  reset      清空所有数据     ledgerflow reset --yes
  help       显示帮助

金额支持简写: 1k=1000, 2.5w=25000, 3千=3000, 5万=50000
标签支持多个: -tag 旅行 -tag 团建   （导入/导出时以 | 分隔）

示例:
  ledgerflow add -type income -amount 8k -cat 工资 -note 月薪
  ledgerflow add -type expense -amount 12.5 -cat 交通 -note 地铁 -tag 通勤 -repeat monthly
  ledgerflow summary -month 2026-08
  ledgerflow top -n 5
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
	case "recent":
		cmdRecent(st, args)
	case "summary":
		cmdSummary(st, args)
	case "tagsum":
		cmdTagSum(st, args)
	case "stats":
		cmdStats(st)
	case "week":
		cmdWeek(st)
	case "balance":
		cmdBalance(st)
	case "top":
		cmdTop(st, args)
	case "month":
		cmdMonth(st)
	case "chart":
		cmdChart(st)
	case "categories":
		cmdCategories(st)
	case "tags":
		cmdTags(st)
	case "tag":
		cmdTag(st, args)
	case "budget":
		cmdBudget(st, args)
	case "edit":
		cmdEdit(st, args)
	case "rename":
		cmdRename(st, args)
	case "del":
		cmdDel(st, args)
	case "import":
		cmdImport(st, args)
	case "export":
		cmdExport(st, args)
	case "reset":
		cmdReset(st, args)
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
	amountStr := fs.String("amount", "", "金额，支持 1k/2.5w/3千/5万 简写")
	cat := fs.String("cat", "", "类别，如 餐饮/交通/工资")
	note := fs.String("note", "", "备注")
	var tagSlice stringSlice
	fs.Var(&tagSlice, "tag", "标签，可重复: -tag 旅行 -tag 团建；或以 | 分隔")
	dateStr := fs.String("date", "", "日期 2006-01-02，缺省为今天")
	repeat := fs.String("repeat", "", "重复方式: monthly（按月重复，生成未来 11 个月）")
	_ = fs.Parse(args)

	if *typ != "income" && *typ != "expense" {
		ui.Error("请通过 -type 指定 income 或 expense")
		return
	}
	amount, err := parseAmount(*amountStr)
	if err != nil || amount <= 0 {
		ui.Error("金额无效，请通过 -amount 指定正数（支持 k/w/千/万 简写）")
		return
	}
	if *cat == "" {
		ui.Error("请通过 -cat 指定类别")
		return
	}
	tags := parseTags(strings.Join(tagSlice, "|"))
	rp := ""
	if *repeat == "monthly" {
		rp = "monthly"
	}
	base := parseDate(*dateStr)
	rec, err := st.Add(store.Type(*typ), amount, *cat, *note, tags, base, rp)
	if err != nil {
		// 原来 Add 不返回错误，save() 失败被静默吞掉：
		// 磁盘满时照样打印"已记录"，用户以为记上了，其实什么都没写。
		ui.Error("记录失败: " + err.Error())
		os.Exit(1)
	}
	ui.Success(fmt.Sprintf("已记录: %s %.2f (%s) [%s]", rec.Type, rec.Amount, rec.Category, rec.ID))
	if len(rec.Tags) > 0 {
		ui.Info("标签: " + strings.Join(rec.Tags, "、"))
	}
	if rp == "monthly" {
		n := 0
		for i := 1; i <= 11; i++ {
			// AddDate 对月末有个反直觉行为：1-31 加一个月得到 3-2 或 3-3
			// （因为 2 月没有 31 号，会往后溢出）。这里改成落到目标月的最后一天，
			// 「每月 31 号的房租」不会莫名跑到 3 月初去。
			d := addMonthsClamped(base, i)
			if _, err := st.Add(store.Type(*typ), amount, *cat, *note, tags, d, rp); err != nil {
				ui.Error(fmt.Sprintf("第 %d 个月的重复记录写入失败: %v", i, err))
				break
			}
			n++
		}
		ui.Info(fmt.Sprintf("已生成未来 %d 个月的重复记录", n))
	}
	maybeBudgetAlert(st, rec.Date.Format("2006-01"))
}

func cmdList(st *store.Store, args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	cat := fs.String("cat", "", "按类别筛选")
	typ := fs.String("type", "", "按类型筛选 income/expense")
	month := fs.String("month", "", "按月份筛选 2006-01")
	q := fs.String("q", "", "关键字搜索")
	tag := fs.String("tag", "", "按标签筛选")
	_ = fs.Parse(args)
	items := st.Filter(*cat, *typ, *q, *tag, *month)
	ui.Table(items)
}

func cmdRecent(st *store.Store, args []string) {
	fs := flag.NewFlagSet("recent", flag.ExitOnError)
	n := fs.Int("n", 10, "显示最近 N 条记录")
	_ = fs.Parse(args)
	if *n <= 0 {
		*n = 10
	}
	items := st.List()
	if len(items) > *n {
		items = items[:*n]
	}
	ui.Table(items)
}

func cmdSummary(st *store.Store, args []string) {
	fs := flag.NewFlagSet("summary", flag.ExitOnError)
	month := fs.String("month", "", "按月汇总 2006-01，缺省为全部")
	tag := fs.String("tag", "", "按标签汇总")
	_ = fs.Parse(args)
	var items []store.Transaction
	if *month == "" {
		items = st.List()
	} else {
		items = st.Filter("", "", "", "", *month)
	}
	if *tag != "" {
		items = st.Filter("", "", "", *tag, "")
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
	listAll := fs.Bool("list", false, "列出全部已设置的月份预算")
	_ = fs.Parse(args)
	if *listAll {
		budgets := st.ListBudgets()
		ui.Header()
		if len(budgets) == 0 {
			ui.Info("尚未设置任何预算，使用 budget -month 2026-08 -limit 3000 来设置")
			return
		}
		ui.Info(fmt.Sprintf("已设置 %d 个月份的预算:", len(budgets)))
		for _, b := range budgets {
			bs := report.Budget(st.List(), b)
			status := "正常"
			if bs.OverBudget {
				status = ui.Red + "超支" + ui.Reset
			} else if bs.NeedAlert {
				status = ui.Yellow + "预警" + ui.Reset
			}
			ui.Info(fmt.Sprintf("  %s  上限 %.2f / 已用 %.2f (%s)  [%s]",
				b.Month, b.Limit, bs.Spent, fmt.Sprintf("%.0f%%", bs.UsedRatio*100), status))
		}
		return
	}
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
	// 原来这里是裸调用 st.SetBudget(...)，返回值被完全丢掉：
	// -month 2026-13 或 -alert 80（本意 80%）都会被"设置成功"，
	// 之后永远匹配不到任何记录 / 永远不提醒，用户毫不知情。
	if err := st.SetBudget(*month, *limit, *alert); err != nil {
		ui.Error("设置预算失败: " + err.Error())
		os.Exit(1)
	}
	ui.Success(fmt.Sprintf("已设置 %s 预算上限 %.2f，提醒比例 %.0f%%", *month, *limit, *alert*100))
}

func cmdEdit(st *store.Store, args []string) {
	if len(args) < 1 {
		ui.Error("请提供记录 ID")
		return
	}
	id := args[0]
	fs := flag.NewFlagSet("edit", flag.ExitOnError)
	amountStr := fs.String("amount", "", "新金额（支持 k/w/千/万 简写）")
	cat := fs.String("cat", "", "新类别")
	note := fs.String("note", "", "新备注")
	var tagSlice stringSlice
	fs.Var(&tagSlice, "tag", "新标签（整体覆盖，可重复或 | 分隔）")
	dateStr := fs.String("date", "", "新日期 2006-01-02")
	_ = fs.Parse(args[1:])
	amount := -1.0
	if *amountStr != "" {
		v, err := parseAmount(*amountStr)
		if err != nil || v <= 0 {
			ui.Error("金额无效")
			return
		}
		amount = v
	}
	tags := []string(nil)
	if len(tagSlice) > 0 {
		tags = parseTags(strings.Join(tagSlice, "|"))
	}
	// parseDate("") 原来返回今天，于是 edit 不带 -date 也会把日期改成今天。
	// 这里必须区分"没给日期"和"给了日期"。
	var newDate time.Time
	if *dateStr != "" {
		newDate = parseDate(*dateStr)
	}
	if err := st.Update(id, amount, *cat, *note, tags, newDate); err != nil {
		ui.Error("更新失败: " + err.Error())
		os.Exit(1)
	}
	ui.Success("已更新 " + id)
}

func cmdRename(st *store.Store, args []string) {
	fs := flag.NewFlagSet("rename", flag.ExitOnError)
	from := fs.String("from", "", "原类别名")
	to := fs.String("to", "", "新类别名")
	_ = fs.Parse(args)
	if *from == "" || *to == "" {
		ui.Error("用法: ledgerflow rename -from 旧类别 -to 新类别")
		return
	}
	n, err := st.RenameCategory(*from, *to)
	if err != nil {
		ui.Error("改名失败: " + err.Error())
		os.Exit(1)
	}
	if n == 0 {
		ui.Info("没找到任何「" + *from + "」的记录，没改")
		return
	}
	ui.Success(fmt.Sprintf("已把 %d 条「%s」改成「%s」", n, *from, *to))
}

func cmdDel(st *store.Store, args []string) {
	fs := flag.NewFlagSet("del", flag.ExitOnError)
	all := fs.Bool("all", false, "删除全部记录（需配合 --yes）")
	yes := fs.Bool("yes", false, "确认删除")
	_ = fs.Parse(args)
	if *all {
		if !*yes {
			ui.Warn("将删除全部记录，确认请加 --yes")
			return
		}
		if err := st.Reset(); err != nil {
			ui.Error("清空失败: " + err.Error())
			return
		}
		ui.Success("已删除全部记录")
		return
	}
	if len(fs.Args()) < 1 {
		ui.Error("请提供记录 ID，或使用 --all --yes 删除全部")
		return
	}
	if err := st.Delete(fs.Args()[0]); err != nil {
		ui.Error("删除失败: " + err.Error())
		os.Exit(1)
	}
	ui.Success("已删除 " + fs.Args()[0])
}

func cmdTagSum(st *store.Store, args []string) {
	fs := flag.NewFlagSet("tagsum", flag.ExitOnError)
	month := fs.String("month", "", "限定月份 2006-01")
	_ = fs.Parse(args)
	items := st.List()
	if *month != "" {
		items = st.Filter("", "", "", "", *month)
	}
	type agg struct {
		income  float64
		expense float64
	}
	byTag := map[string]*agg{}
	var untaggedInc, untaggedExp float64
	for _, t := range items {
		if len(t.Tags) == 0 {
			if t.Type == "income" {
				untaggedInc += t.Amount
			} else {
				untaggedExp += t.Amount
			}
			continue
		}
		for _, tag := range t.Tags {
			a, ok := byTag[tag]
			if !ok {
				a = &agg{}
				byTag[tag] = a
			}
			if t.Type == "income" {
				a.income += t.Amount
			} else {
				a.expense += t.Amount
			}
		}
	}
	ui.Header()
	ui.Info("按标签收支汇总（收入 / 支出 / 净额）:")
	tags := make([]string, 0, len(byTag))
	for k := range byTag {
		tags = append(tags, k)
	}
	sort.Strings(tags)
	for _, tag := range tags {
		a := byTag[tag]
		net := a.income - a.expense
		ui.Info(fmt.Sprintf("  %s  收入 %.2f / 支出 %.2f / 净额 %s%.2f%s",
			tag, a.income, a.expense, balanceColor(net), net, ui.Reset))
	}
	if untaggedInc != 0 || untaggedExp != 0 {
		net := untaggedInc - untaggedExp
		ui.Info(fmt.Sprintf("  %s  收入 %.2f / 支出 %.2f / 净额 %s%.2f%s",
			"(无标签)", untaggedInc, untaggedExp, balanceColor(net), net, ui.Reset))
	}
	if len(tags) == 0 && untaggedInc == 0 && untaggedExp == 0 {
		ui.Info("暂无记录")
	}
}

func cmdStats(st *store.Store) {
	ui.PrintStats(st.Stats())
}

// cmdWeek 汇总本周（周一~周日）的收支与结余。
func cmdWeek(st *store.Store) {
	now := time.Now()
	// 算出本周一 0 点：先取今天星期几（周日为 0），回退到周一
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(weekday - 1))
	// 周日结束 = 周一 +7 天
	items := st.List()
	var inWeek []store.Transaction
	for _, t := range items {
		if !t.Date.Before(monday) && t.Date.Before(monday.AddDate(0, 0, 7)) {
			inWeek = append(inWeek, t)
		}
	}
	ui.Header()
	ui.Info(fmt.Sprintf("本周（%s ~ %s）", monday.Format("2006-01-02"), monday.AddDate(0, 0, 6).Format("2006-01-02")))
	if len(inWeek) == 0 {
		ui.Info("本周还没有记账记录")
		return
	}
	s := report.Build(inWeek)
	fmt.Printf("  %s%s\n", ui.Bold+"收入: "+ui.Reset, ui.Green+report.FormatMoney(s.Income)+ui.Reset)
	fmt.Printf("  %s%s\n", ui.Bold+"支出: "+ui.Reset, ui.Red+report.FormatMoney(s.Expense)+ui.Reset)
	fmt.Printf("  %s%s\n", ui.Bold+"结余: "+ui.Reset, balanceColor(s.Balance)+report.FormatMoney(s.Balance)+ui.Reset)
	ui.Info(fmt.Sprintf("本周 %d 笔记录", s.Count))
}

func cmdBalance(st *store.Store) {
	ui.Header()
	s := st.Stats()
	fmt.Printf("  %s%s\n", ui.Bold+"收入: "+ui.Reset, ui.Green+report.FormatMoney(s.Income)+ui.Reset)
	fmt.Printf("  %s%s\n", ui.Bold+"支出: "+ui.Reset, ui.Red+report.FormatMoney(s.Expense)+ui.Reset)
	fmt.Printf("  %s%s\n", ui.Bold+"余额: "+ui.Reset, balanceColor(s.Balance)+report.FormatMoney(s.Balance)+ui.Reset)
	if s.Count == 0 {
		ui.Info("还没有任何记录，先 add 一条吧")
	}
}

func cmdTop(st *store.Store, args []string) {
	fs := flag.NewFlagSet("top", flag.ExitOnError)
	n := fs.Int("n", 5, "显示前 N 个类别")
	month := fs.String("month", "", "限定月份 2006-01")
	_ = fs.Parse(args)
	var items []store.Transaction
	if *month == "" {
		items = st.List()
	} else {
		items = st.Filter("", "", "", "", *month)
	}
	ranks := report.TopCategories(items, *n)
	ui.PrintTop(ranks)
}

func cmdImport(st *store.Store, args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	out := fs.String("o", "ledger.csv", "待导入的 CSV 文件路径（与 export 格式一致）")
	_ = fs.Parse(args)
	imported, skipped, err := st.ImportCSV(*out)
	if err != nil {
		ui.Error("导入失败: " + err.Error())
		return
	}
	ui.Success(fmt.Sprintf("导入完成：成功 %d 条，跳过 %d 条（来源 %s）", imported, skipped, *out))
}

func cmdExport(st *store.Store, args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	out := fs.String("o", "ledger.csv", "输出文件路径")
	fmtType := fs.String("f", "csv", "格式: csv 或 json")
	_ = fs.Parse(args)
	var err error
	switch strings.ToLower(*fmtType) {
	case "json":
		err = st.ExportJSON(*out, st.List())
	case "csv":
		err = st.ExportCSV(*out, st.List())
	default:
		// 原来是 default 走 CSV：-f jsonn 这种手滑会静默导出成 CSV，
		// 文件名却还是 .json，后续解析必然失败且看不出原因。
		ui.Error("不支持的格式 " + *fmtType + "，可选 csv 或 json")
		os.Exit(1)
	}
	if err != nil {
		ui.Error("导出失败: " + err.Error())
		return
	}
	ui.Success("已导出 " + *fmtType + " 到 " + *out)
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

// addMonthsClamped 在 base 上加 n 个月，并把日期钳制在目标月份的最后一天。
//
// time.Time.AddDate 对月末是溢出语义：2026-01-31 加 1 个月会得到 2026-03-03，
// 因为 2 月没有 31 号。对"每月 31 号的房租"这类重复记账来说，
// 这会让记录莫名跑到下下个月，统计和预算全部错位。
// 这里改成钳制：2026-01-31 + 1 个月 = 2026-02-28（闰年则 02-29）。
func addMonthsClamped(base time.Time, n int) time.Time {
	y, m, d := base.Date()
	// 先把日定为 1 号做月份运算，避免 AddDate 内部的规范化溢出。
	first := time.Date(y, m, 1, base.Hour(), base.Minute(), base.Second(),
		base.Nanosecond(), base.Location())
	target := first.AddDate(0, n, 0)
	last := daysInMonth(target.Year(), target.Month())
	if d > last {
		d = last
	}
	return time.Date(target.Year(), target.Month(), d, base.Hour(), base.Minute(),
		base.Second(), base.Nanosecond(), base.Location())
}

// daysInMonth 返回指定年月的天数（自动处理闰年 2 月）。
func daysInMonth(year int, m time.Month) int {
	// 下个月的第 0 天 = 本月最后一天。
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
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

// stringSlice 是一个可重复使用的命令行标志值（如 -tag a -tag b）。
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// parseTags 解析标签参数，支持重复 -tag 或单个以 | 分隔的字符串。
func parseTags(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	// 支持以 | 或 、 或 , 分隔的一次性写法
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '|' || r == '、' || r == ','
	})
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseAmount 解析金额，支持 k/w/千/万/亿 简写（如 1k、2.5w、3千、5万）。
//
// 原实现用 s[len(s)-1:] 取"最后一个字符"，那其实是最后一个**字节**。
// "千"/"万" 在 UTF-8 里各占 3 字节，取到的是它们的末字节（0x83/0x87），
// 永远匹配不上 case "千" / case "万" —— 也就是说这两个后缀从来没生效过，
// 输入 "3千" 会直接掉到 ParseFloat 报错，README 里宣传的功能是坏的。
// 这里改成按 rune 取末字符。
func parseAmount(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("金额不能为空")
	}
	// 容忍千位分隔符和货币符号，粘贴过来的金额也能用。
	s = strings.NewReplacer(",", "", "，", "", "￥", "", "¥", "", "$", "", " ", "").Replace(s)
	if s == "" {
		return 0, fmt.Errorf("金额不能为空")
	}

	r := []rune(s)
	mult := 1.0
	switch r[len(r)-1] {
	case 'k', 'K', '千':
		mult = 1e3
		r = r[:len(r)-1]
	case 'w', 'W', '万':
		mult = 1e4
		r = r[:len(r)-1]
	case '亿':
		mult = 1e8
		r = r[:len(r)-1]
	}
	body := strings.TrimSpace(string(r))
	if body == "" {
		return 0, fmt.Errorf("金额缺少数字部分: %q", s)
	}

	v, err := strconv.ParseFloat(body, 64)
	if err != nil {
		return 0, fmt.Errorf("无法解析金额 %q", s)
	}
	v *= mult
	// ParseFloat 认 "NaN"/"Inf"/"1e400"，这些一旦入账，
	// 之后所有汇总都会变成 NaN 且再也救不回来。
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("金额不是有限数: %q", s)
	}
	return v, nil
}

func cmdChart(st *store.Store) {
	rows := report.ByMonth(st.List())
	ui.Header()
	ui.Info("最近月份收支柱状图（蓝=收入 红=支出）")
	var income, expense []report.Bar
	for _, r := range rows {
		income = append(income, report.Bar{Label: r.Month, Value: r.Income, Color: ui.Cyan})
		expense = append(expense, report.Bar{Label: r.Month, Value: r.Expense, Color: ui.Red})
	}
	fmt.Print(report.RenderBars(income, 10))
	fmt.Print(report.RenderBars(expense, 10))
}

func cmdCategories(st *store.Store) {
	ui.Header()
	ui.Categories(st.Categories())
	ui.Info("提示：使用 add -cat 可记录任意自定义类别")
}

func cmdTags(st *store.Store) {
	ui.Header()
	tags := st.AllTags()
	if len(tags) == 0 {
		ui.Info("暂无标签，使用 add -tag 为记录打标签")
		return
	}
	ui.Info("全部标签（按使用频率）: " + strings.Join(tags, "、"))
}

// cmdTag 现在只支持一个子操作：把某条记录上的某个标签摘掉
func cmdTag(st *store.Store, args []string) {
	fs := flag.NewFlagSet("tag", flag.ExitOnError)
	rm := fs.String("rm", "", "要移除的标签名")
	_ = fs.Parse(args)
	if *rm == "" {
		ui.Error("用法: ledgerflow tag -rm 标签名 <id...>")
		return
	}
	ids := fs.Args()
	if len(ids) == 0 {
		ui.Error("请提供至少一条记录 ID")
		return
	}
	ok, miss := 0, 0
	for _, id := range ids {
		if err := st.RemoveTag(id, *rm); err != nil {
			miss++
			ui.Error(fmt.Sprintf("%s: %v", id, err))
			continue
		}
		ok++
	}
	if ok > 0 {
		ui.Success(fmt.Sprintf("已从 %d 条记录移除标签「%s」", ok, *rm))
	}
	if miss > 0 && ok == 0 {
		os.Exit(1)
	}
}

func cmdReset(st *store.Store, args []string) {
	fs := flag.NewFlagSet("reset", flag.ExitOnError)
	yes := fs.Bool("yes", false, "确认清空，不加此参数不会执行")
	_ = fs.Parse(args)
	if !*yes {
		ui.Warn("此操作将删除全部记录与预算，确认请加 --yes 参数")
		return
	}
	if err := st.Reset(); err != nil {
		ui.Error("清空失败: " + err.Error())
		return
	}
	ui.Success("已清空全部数据")
}

var _ = strconv.Itoa
