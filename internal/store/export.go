package store

import (
	"encoding/csv"
	"os"
	"strconv"
	"strings"
	"time"
)

// csvHeader 是 CSV 导出的列头。顺序变动需同步 ImportCSV。
var csvHeader = []string{"ID", "日期", "类型", "金额", "类别", "标签", "备注", "创建时间"}

// ExportCSV 将记录导出为 CSV 文件。
func (s *Store) ExportCSV(path string, items []Transaction) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write(csvHeader); err != nil {
		return err
	}
	for _, t := range items {
		typ := "收入"
		if t.Type == "expense" {
			typ = "支出"
		}
		row := []string{
			t.ID,
			t.Date.Format("2006-01-02"),
			typ,
			strconv.FormatFloat(t.Amount, 'f', 2, 64),
			t.Category,
			strings.Join(t.Tags, "|"),
			t.Note,
			t.Created.Format(time.RFC3339),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// ImportCSV 从 CSV 文件导入记录。导入的记录会沿用文件中的 ID（冲突时重新生成）。
// 该函数返回导入成功条数与跳过条数。
func (s *Store) ImportCSV(path string) (int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return 0, 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(rows) == 0 {
		return 0, 0, nil
	}
	header := rows[0]
	col := map[string]int{}
	for i, h := range header {
		col[h] = i
	}
	get := func(row []string, name string) string {
		if i, ok := col[name]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	imported, skipped := 0, 0
	existing := map[string]bool{}
	for _, t := range s.data.Transactions {
		existing[t.ID] = true
	}
	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		dateStr := get(row, "日期")
		typStr := get(row, "类型")
		amountStr := get(row, "金额")
		cat := get(row, "类别")
		note := get(row, "备注")
		tagsStr := get(row, "标签")
		createdStr := get(row, "创建时间")

		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			skipped++
			continue
		}
		typ := string(TypeExpense)
		if typStr == "收入" || typStr == "income" {
			typ = string(TypeIncome)
		}
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil || amount <= 0 {
			skipped++
			continue
		}
		if cat == "" {
			skipped++
			continue
		}
		var tags []string
		if tagsStr != "" {
			for _, tg := range strings.Split(tagsStr, "|") {
				if tg = strings.TrimSpace(tg); tg != "" {
					tags = append(tags, tg)
				}
			}
		}
		var created time.Time
		if createdStr != "" {
			if ct, err := time.Parse(time.RFC3339, createdStr); err == nil {
				created = ct
			}
		}
		if created.IsZero() {
			created = time.Now()
		}
		id := get(row, "ID")
		if id == "" || existing[id] {
			id = newID()
		}
		existing[id] = true
		s.data.Transactions = append(s.data.Transactions, Transaction{
			ID:       id,
			Type:     typ,
			Amount:   amount,
			Category: cat,
			Note:     note,
			Tags:     tags,
			Date:     date,
			Created:  created,
		})
		imported++
	}
	_ = s.save()
	return imported, skipped, nil
}
