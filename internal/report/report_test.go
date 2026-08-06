package report

import (
	"testing"
	"time"

	"ledgerflow/internal/store"
)

func tx(typ string, amount float64, cat string, date time.Time) store.Transaction {
	return store.Transaction{
		Type:     typ,
		Amount:   amount,
		Category: cat,
		Date:     date,
	}
}

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestBuildEmpty(t *testing.T) {
	s := Build(nil)
	if s.Count != 0 || s.Income != 0 || s.Expense != 0 || s.Balance != 0 {
		t.Fatalf("空输入应全为 0: %+v", s)
	}
	if s.TopCategory != "" {
		t.Fatalf("空输入 TopCategory 应为空, 实际 %q", s.TopCategory)
	}
	if s.ByCategory == nil {
		t.Fatal("ByCategory 不应为 nil，调用方会直接遍历它")
	}
}

func TestBuildBasic(t *testing.T) {
	d := day(2026, 3, 1)
	items := []store.Transaction{
		tx("income", 1000, "工资", d),
		tx("expense", 300, "餐饮", d),
		tx("expense", 200, "交通", d),
		tx("expense", 100, "餐饮", d),
	}
	s := Build(items)
	if s.Count != 4 {
		t.Fatalf("Count 应为 4, 实际 %d", s.Count)
	}
	if s.Income != 1000 || s.Expense != 600 || s.Balance != 400 {
		t.Fatalf("收支汇总错误: %+v", s)
	}
	if s.ByCategory["餐饮"] != 400 {
		t.Fatalf("餐饮小计应为 400, 实际 %v", s.ByCategory["餐饮"])
	}
	// 收入不应计入 ByCategory（那是支出构成）。
	if _, ok := s.ByCategory["工资"]; ok {
		t.Fatal("ByCategory 不应包含收入类别")
	}
	if s.TopCategory != "餐饮" {
		t.Fatalf("TopCategory 应为 餐饮, 实际 %q", s.TopCategory)
	}
}

// 浮点累加尾巴应被规整掉。
func TestBuildRoundsMoney(t *testing.T) {
	d := day(2026, 3, 1)
	var items []store.Transaction
	for i := 0; i < 100; i++ {
		items = append(items, tx("expense", 0.01, "餐饮", d))
	}
	s := Build(items)
	if s.Expense != 1.0 {
		t.Fatalf("100 笔 0.01 应等于 1.00, 实际 %v", s.Expense)
	}
	if s.ByCategory["餐饮"] != 1.0 {
		t.Fatalf("类别小计应为 1.00, 实际 %v", s.ByCategory["餐饮"])
	}

	s2 := Build([]store.Transaction{
		tx("income", 0.1, "工资", d),
		tx("income", 0.2, "工资", d),
	})
	if s2.Income != 0.3 || s2.Balance != 0.3 {
		t.Fatalf("0.1+0.2 应等于 0.3, 实际 收入 %v 结余 %v", s2.Income, s2.Balance)
	}
}

// 金额相同的类别，TopCategory 必须每次都一样（原来靠 map 随机顺序）。
func TestTopCategoryStableOnTie(t *testing.T) {
	d := day(2026, 3, 1)
	items := []store.Transaction{
		tx("expense", 100, "餐饮", d),
		tx("expense", 100, "交通", d),
		tx("expense", 100, "购物", d),
	}
	first := Build(items).TopCategory
	if first == "" {
		t.Fatal("TopCategory 不应为空")
	}
	for i := 0; i < 50; i++ {
		if got := Build(items).TopCategory; got != first {
			t.Fatalf("第 %d 次结果变成了 %q（首次 %q），并列时排名不稳定", i, got, first)
		}
	}
}

func TestByMonth(t *testing.T) {
	items := []store.Transaction{
		tx("income", 1000, "工资", day(2026, 1, 5)),
		tx("expense", 300, "餐饮", day(2026, 1, 20)),
		tx("expense", 500, "居住", day(2026, 2, 1)),
		tx("income", 900, "工资", day(2026, 3, 5)),
	}
	rows := ByMonth(items)
	if len(rows) != 3 {
		t.Fatalf("应有 3 个月, 实际 %d", len(rows))
	}
	// 必须按月份升序。
	want := []string{"2026-01", "2026-02", "2026-03"}
	for i, w := range want {
		if rows[i].Month != w {
			t.Fatalf("第 %d 行应为 %s, 实际 %s", i, w, rows[i].Month)
		}
	}
	if rows[0].Income != 1000 || rows[0].Expense != 300 || rows[0].Balance != 700 {
		t.Fatalf("1 月汇总错误: %+v", rows[0])
	}
	if rows[1].Balance != -500 {
		t.Fatalf("2 月结余应为 -500, 实际 %v", rows[1].Balance)
	}
}

func TestByMonthRounds(t *testing.T) {
	var items []store.Transaction
	for i := 0; i < 3; i++ {
		items = append(items, tx("expense", 0.1, "餐饮", day(2026, 1, 1)))
	}
	rows := ByMonth(items)
	if rows[0].Expense != 0.3 {
		t.Fatalf("三笔 0.1 应等于 0.3, 实际 %v", rows[0].Expense)
	}
}

func TestTopCategories(t *testing.T) {
	d := day(2026, 3, 1)
	items := []store.Transaction{
		tx("expense", 100, "餐饮", d),
		tx("expense", 50, "餐饮", d),
		tx("expense", 200, "居住", d),
		tx("expense", 10, "交通", d),
		tx("income", 9999, "工资", d), // 收入不参与排行
	}
	ranks := TopCategories(items, 0)
	if len(ranks) != 3 {
		t.Fatalf("应有 3 个支出类别, 实际 %d", len(ranks))
	}
	if ranks[0].Category != "居住" || ranks[0].Amount != 200 {
		t.Fatalf("第一名应为 居住/200, 实际 %+v", ranks[0])
	}
	if ranks[1].Category != "餐饮" || ranks[1].Amount != 150 || ranks[1].Count != 2 {
		t.Fatalf("第二名应为 餐饮/150/2 笔, 实际 %+v", ranks[1])
	}
	// limit 截断。
	if got := TopCategories(items, 2); len(got) != 2 {
		t.Fatalf("limit=2 应返回 2 条, 实际 %d", len(got))
	}
	// limit 大于总数不应 panic。
	if got := TopCategories(items, 99); len(got) != 3 {
		t.Fatalf("limit 超出时应返回全部, 实际 %d", len(got))
	}
}

// 金额并列时排名必须稳定。
func TestTopCategoriesStableOnTie(t *testing.T) {
	d := day(2026, 3, 1)
	items := []store.Transaction{
		tx("expense", 100, "丙", d),
		tx("expense", 100, "甲", d),
		tx("expense", 100, "乙", d),
	}
	first := TopCategories(items, 0)
	for i := 0; i < 50; i++ {
		cur := TopCategories(items, 0)
		for j := range cur {
			if cur[j].Category != first[j].Category {
				t.Fatalf("第 %d 次排名发生变化: %v vs %v", i, cur[j].Category, first[j].Category)
			}
		}
	}
}

func TestBudgetStatus(t *testing.T) {
	b := store.Budget{Month: "2026-03", Limit: 1000, AlertAt: 0.8}
	items := []store.Transaction{
		tx("expense", 500, "餐饮", day(2026, 3, 1)),
		tx("expense", 400, "居住", day(2026, 3, 20)),
		tx("expense", 999, "餐饮", day(2026, 4, 1)), // 别的月份，不计入
		tx("income", 5000, "工资", day(2026, 3, 1)), // 收入不计入
	}
	bs := Budget(items, b)
	if bs.Spent != 900 {
		t.Fatalf("已用应为 900, 实际 %v", bs.Spent)
	}
	if bs.Remaining != 100 {
		t.Fatalf("剩余应为 100, 实际 %v", bs.Remaining)
	}
	if bs.OverBudget {
		t.Fatal("900/1000 不应算超预算")
	}
	if !bs.NeedAlert {
		t.Fatal("90% 已超过 80% 阈值, 应提醒")
	}
}

func TestBudgetOverAndZeroLimit(t *testing.T) {
	items := []store.Transaction{tx("expense", 1200, "餐饮", day(2026, 3, 1))}

	bs := Budget(items, store.Budget{Month: "2026-03", Limit: 1000, AlertAt: 0.8})
	if !bs.OverBudget || bs.Remaining != -200 {
		t.Fatalf("超预算判断错误: %+v", bs)
	}

	// Limit 为 0 时不能除零产生 Inf。
	bs0 := Budget(items, store.Budget{Month: "2026-03", Limit: 0, AlertAt: 0.8})
	if bs0.UsedRatio != 0 {
		t.Fatalf("Limit=0 时使用率应为 0, 实际 %v", bs0.UsedRatio)
	}
}

// 正好用完预算不应因浮点误差被判成超支。
func TestBudgetExactlyOnLimitNotOver(t *testing.T) {
	var items []store.Transaction
	for i := 0; i < 100; i++ {
		items = append(items, tx("expense", 0.01, "餐饮", day(2026, 3, 1)))
	}
	bs := Budget(items, store.Budget{Month: "2026-03", Limit: 1, AlertAt: 0.8})
	if bs.Spent != 1.0 {
		t.Fatalf("已用应为 1.00, 实际 %v", bs.Spent)
	}
	if bs.OverBudget {
		t.Fatalf("正好用完不应算超预算 (spent=%v limit=1)", bs.Spent)
	}
	if bs.Remaining != 0 {
		t.Fatalf("剩余应为 0, 实际 %v", bs.Remaining)
	}
}

func TestFormatMoney(t *testing.T) {
	cases := map[float64]string{
		0:       "0.00",
		1:       "1.00",
		3.14159: "3.14",
		-25.5:   "-25.50",
		1234.5:  "1234.50",
	}
	for in, want := range cases {
		if got := FormatMoney(in); got != want {
			t.Fatalf("FormatMoney(%v) = %q, 期望 %q", in, got, want)
		}
	}
}

func TestNowMonth(t *testing.T) {
	got := NowMonth()
	if _, err := time.Parse("2006-01", got); err != nil {
		t.Fatalf("NowMonth 应返回 2006-01 格式, 实际 %q", got)
	}
	if got != time.Now().Format("2006-01") {
		t.Fatalf("NowMonth 应等于当前年月, 实际 %q", got)
	}
}
