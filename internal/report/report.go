package report

import (
	"fmt"
	"sort"
	"time"

	"ledgerflow/internal/store"
)

// Summary 汇总某段时间内的收支情况。
type Summary struct {
	Income      float64
	Expense     float64
	Balance     float64
	Count       int
	ByCategory  map[string]float64
	TopCategory string
}

// Build 基于给定记录生成汇总。
func Build(items []store.Transaction) Summary {
	s := Summary{ByCategory: map[string]float64{}}
	for _, t := range items {
		s.Count++
		if t.Type == "income" {
			s.Income += t.Amount
		} else {
			s.Expense += t.Amount
		}
		if t.Type == "expense" {
			s.ByCategory[t.Category] += t.Amount
		}
	}
	// 浮点累加会留下 1.0000000000000007 这样的尾巴，
	// 汇总后统一规整到分，避免 "%.2f" 之外的地方（比如比较、判超预算）出偏差。
	s.Income = store.RoundMoney(s.Income)
	s.Expense = store.RoundMoney(s.Expense)
	s.Balance = store.RoundMoney(s.Income - s.Expense)
	for c, v := range s.ByCategory {
		s.ByCategory[c] = store.RoundMoney(v)
	}

	// 原来直接遍历 map 找最大值：金额相同的两个类别，
	// 每次运行 TopCategory 可能不一样（Go 的 map 遍历顺序是随机的）。
	// 现在金额相同时按类别名取字典序小的那个，输出稳定可复现。
	// 别拿 max 的零值当初始基准：所有支出都是 0 的时候 v > max 永远不成立，
	// TopCategory 会莫名其妙变成空。改成用 first 标记首个元素。
	top := ""
	var max float64
	first := true
	for c, v := range s.ByCategory {
		if first || v > max || (v == max && c < top) {
			max = v
			top = c
			first = false
		}
	}
	s.TopCategory = top
	return s
}

// Monthly 返回按月份的收支序列，已排序。
type Monthly struct {
	Month   string
	Income  float64
	Expense float64
	Balance float64
}

// ByMonth 把记录按月份聚合。
func ByMonth(items []store.Transaction) []Monthly {
	m := map[string]*Monthly{}
	for _, t := range items {
		key := t.Date.Format("2006-01")
		row, ok := m[key]
		if !ok {
			row = &Monthly{Month: key}
			m[key] = row
		}
		if t.Type == "income" {
			row.Income += t.Amount
		} else {
			row.Expense += t.Amount
		}
		row.Balance = row.Income - row.Expense
	}
	out := make([]Monthly, 0, len(m))
	for _, v := range m {
		v.Income = store.RoundMoney(v.Income)
		v.Expense = store.RoundMoney(v.Expense)
		v.Balance = store.RoundMoney(v.Income - v.Expense)
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Month < out[j].Month })
	return out
}

// BudgetStatus 描述某月预算执行情况。
type BudgetStatus struct {
	Month      string
	Limit      float64
	Spent      float64
	Remaining  float64
	UsedRatio  float64
	AlertAt    float64
	OverBudget bool
	NeedAlert  bool
}

// Budget 计算某月预算状态。
func Budget(items []store.Transaction, b store.Budget) BudgetStatus {
	var spent float64
	for _, t := range items {
		if t.Type == "expense" && t.Date.Format("2006-01") == b.Month {
			spent += t.Amount
		}
	}
	// 先规整到分再算比例：否则 100 笔 0.01 累加出来的 1.0000000000000007
	// 会让"正好用完预算"被判成超支。
	spent = store.RoundMoney(spent)
	ratio := 0.0
	if b.Limit > 0 {
		ratio = spent / b.Limit
	}
	return BudgetStatus{
		Month:      b.Month,
		Limit:      b.Limit,
		Spent:      spent,
		Remaining:  store.RoundMoney(b.Limit - spent),
		UsedRatio:  ratio,
		AlertAt:    b.AlertAt,
		OverBudget: spent > b.Limit,
		NeedAlert:  b.AlertAt > 0 && ratio >= b.AlertAt,
	}
}

// CategoryRank 表示单个类别的金额排行。
type CategoryRank struct {
	Category string
	Amount   float64
	Count    int
}

// TopCategories 返回支出类别金额排行（降序），最多 limit 条；limit<=0 表示不限制。
func TopCategories(items []store.Transaction, limit int) []CategoryRank {
	m := map[string]*CategoryRank{}
	for _, t := range items {
		if t.Type != "expense" {
			continue
		}
		r, ok := m[t.Category]
		if !ok {
			r = &CategoryRank{Category: t.Category}
			m[t.Category] = r
		}
		r.Amount += t.Amount
		r.Count++
	}
	out := make([]CategoryRank, 0, len(m))
	for _, v := range m {
		v.Amount = store.RoundMoney(v.Amount)
		out = append(out, *v)
	}
	// 金额相同的类别原来靠 map 遍历顺序决定名次，每次跑 top 榜单顺序都可能变。
	// 加上类别名做次级排序键，结果就稳定了。
	sort.Slice(out, func(i, j int) bool {
		if out[i].Amount != out[j].Amount {
			return out[i].Amount > out[j].Amount
		}
		return out[i].Category < out[j].Category
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// FormatMoney 统一金额格式。
func FormatMoney(v float64) string {
	return fmt.Sprintf("%.2f", v)
}

// NowMonth 返回当前年月字符串。
func NowMonth() string {
	return time.Now().Format("2006-01")
}
