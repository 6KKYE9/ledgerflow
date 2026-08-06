package store

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// csvHeader 是 CSV 导出的列头。顺序变动需同步 ImportCSV。
var csvHeader = []string{"ID", "日期", "类型", "金额", "类别", "标签", "备注", "创建时间"}

// ExportCSV 将记录导出为 CSV 文件。
//
// 原来用 os.Create 直接往目标路径写，且只 defer Close 不检查它的返回值：
// 写到一半磁盘满，用户手上会留下一个"看起来成功了"的半截 CSV，
// 而且如果覆盖的是已有文件，旧文件已经被截断、找不回来了。
// 现在先写临时文件再 Rename，失败时目标文件保持原样。
func (s *Store) ExportCSV(path string, items []Transaction) error {
	return writeFileAtomic(path, func(f *os.File) error {
		// 加 UTF-8 BOM，否则 Excel 打开中文列头会是乱码。
		if _, err := f.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
			return err
		}
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
	})
}

// writeFileAtomic 把 write 写出的内容原子地落到 path：
// 先写同目录下的临时文件，Sync 后再 Rename 覆盖目标。
// 中途任何一步失败都不会动到 path 上的既有文件。
func writeFileAtomic(path string, write func(*os.File) error) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".export-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if tmp != "" {
			f.Close()
			os.Remove(tmp)
		}
	}()

	if err := write(f); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	tmp = ""
	return nil
}

// parseImportDate 尝试用几种常见格式解析日期。
// 原来只认 2006-01-02，从 Excel 里存出来的 2026/1/5 会被整行跳过，
// 而且不给任何提示，用户只看到"跳过 N 条"却不知道为什么。
func parseImportDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	layouts := []string{
		"2006-01-02", "2006/01/02", "2006/1/2", "2006-1-2",
		"2006.01.02", "20060102", time.RFC3339,
	}
	for _, l := range layouts {
		if d, err := time.Parse(l, s); err == nil {
			return d, true
		}
	}
	return time.Time{}, false
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
	// 手工编辑过的 CSV 经常尾列缺失或多一列，默认的严格列数检查会让
	// 整个文件直接读取失败。放宽后由后面的逐行校验决定跳过哪些行。
	r.FieldsPerRecord = -1
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
		// 我们导出时会写 UTF-8 BOM（否则 Excel 显示中文乱码），
		// 但 encoding/csv 不会自动剥掉它，第一个列名会变成 "\ufeffID"，
		// 导致自己导出的文件再导入时 ID 列永远读不到。
		h = strings.TrimPrefix(h, "\ufeff")
		col[strings.TrimSpace(h)] = i
	}
	// 支持中英文两套列名，方便从其他记账工具导出的表直接导入。
	alias := map[string][]string{
		"ID":   {"ID", "id"},
		"日期":   {"日期", "date", "Date"},
		"类型":   {"类型", "type", "Type"},
		"金额":   {"金额", "amount", "Amount"},
		"类别":   {"类别", "category", "Category"},
		"标签":   {"标签", "tags", "Tags"},
		"备注":   {"备注", "note", "Note"},
		"创建时间": {"创建时间", "created", "Created"},
	}
	get := func(row []string, name string) string {
		for _, n := range alias[name] {
			if i, ok := col[n]; ok && i >= 0 && i < len(row) {
				return strings.TrimSpace(row[i])
			}
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

		date, ok := parseImportDate(dateStr)
		if !ok {
			skipped++
			continue
		}
		typ := string(TypeExpense)
		if typStr == "收入" || strings.EqualFold(typStr, "income") {
			typ = string(TypeIncome)
		}
		// 金额列常带千位分隔符或货币符号，先清掉再解析。
		amountStr = strings.NewReplacer(",", "", "￥", "", "¥", "", "$", "", " ", "").Replace(amountStr)
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil || !validAmount(amount) {
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
			Amount:   roundMoney(amount),
			Category: cat,
			Note:     note,
			Tags:     normalizeTags(tags),
			Date:     date,
			Created:  created,
		})
		imported++
	}
	if imported == 0 {
		return 0, skipped, nil
	}
	// 原来写的是 `_ = s.save()`：磁盘写失败被完全吞掉，
	// 命令行照样打印"导入 N 条成功"，重启后这 N 条根本不在。
	// 现在向上报错，并把内存回滚到导入前的状态。
	if err := s.save(); err != nil {
		s.data.Transactions = s.data.Transactions[:len(s.data.Transactions)-imported]
		return 0, skipped, err
	}
	return imported, skipped, nil
}
