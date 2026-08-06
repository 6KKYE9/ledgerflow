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
	Expense    float64
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
	s.Balance = s.Income - s.Expense
	top := ""
	var max float64
	for c, v := range s.ByCategory {
		if v > max {
			max = v
			top = c
		}
	}
	s.TopCategory = top
	return s
}

// Monthly 返回按月份的收支序列，已排序。
type Monthly struct {
	Month    string
	Income   float64
	Expense  float64
	Balance  float64
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
	ratio := 0.0
	if b.Limit > 0 {
		ratio = spent / b.Limit
	}
	return BudgetStatus{
		Month:      b.Month,
		Limit:      b.Limit,
		Spent:      spent,
		Remaining:  b.Limit - spent,
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
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Amount > out[j].Amount })
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
