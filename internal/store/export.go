package store

import (
	"encoding/csv"
	"os"
	"strconv"
	"time"
)

// ExportCSV 将记录导出为 CSV 文件。
func (s *Store) ExportCSV(path string, items []Transaction) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write([]string{"ID", "日期", "类型", "金额", "类别", "备注", "创建时间"}); err != nil {
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
