package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExportImportCSVRoundTrip(t *testing.T) {
	s := tempStore(t)
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	mustAdd(t, s, TypeIncome, 1234.56, "工资", "三月薪水", []string{"固定"}, day)
	mustAdd(t, s, TypeExpense, 42.5, "餐饮", "火锅, 带逗号", []string{"聚餐", "周末"}, day)

	csvPath := filepath.Join(t.TempDir(), "out.csv")
	if err := s.ExportCSV(csvPath, s.List()); err != nil {
		t.Fatalf("ExportCSV: %v", err)
	}

	// 导出文件应带 UTF-8 BOM（Excel 打开中文不乱码）。
	b, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.HasPrefix(string(b), "\ufeff") {
		t.Fatal("导出的 CSV 应带 UTF-8 BOM")
	}

	// 导回到一个新账本，条数与内容都要对得上。
	s2 := tempStore(t)
	n, skipped, err := s2.ImportCSV(csvPath)
	if err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	if n != 2 || skipped != 0 {
		t.Fatalf("应导入 2 条跳过 0 条, 实际 导入 %d 跳过 %d", n, skipped)
	}
	items := s2.List()
	if len(items) != 2 {
		t.Fatalf("导入后应有 2 条, 实际 %d", len(items))
	}
	var income, expense Transaction
	for _, it := range items {
		if it.Type == "income" {
			income = it
		} else {
			expense = it
		}
	}
	if income.Amount != 1234.56 || income.Category != "工资" {
		t.Fatalf("收入记录导入错误: %+v", income)
	}
	if expense.Note != "火锅, 带逗号" {
		t.Fatalf("含逗号的备注未正确往返: %q", expense.Note)
	}
	if len(expense.Tags) != 2 {
		t.Fatalf("标签未正确往返: %v", expense.Tags)
	}
	if !income.Date.Equal(day) {
		t.Fatalf("日期未正确往返: %v", income.Date)
	}
}

// 我们自己写 BOM，导入时若不剥掉，第一列 "ID" 会变成 "\ufeffID"。
func TestImportStripsBOMFromHeader(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "in.csv")
	content := "\ufeffID,日期,类型,金额,类别,标签,备注,创建时间\n" +
		"myid001,2026-03-15,支出,10.00,餐饮,,面,\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s := tempStore(t)
	if _, _, err := s.ImportCSV(p); err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	items := s.List()
	if len(items) != 1 {
		t.Fatalf("应导入 1 条, 实际 %d", len(items))
	}
	if items[0].ID != "myid001" {
		t.Fatalf("BOM 未剥离导致 ID 列读不到: %q", items[0].ID)
	}
}

func TestImportDateFormats(t *testing.T) {
	ok := map[string]string{
		"2026-03-15": "2026-03-15",
		"2026/03/15": "2026-03-15",
		"2026/3/5":   "2026-03-05",
		"2026-3-5":   "2026-03-05",
		"2026.03.15": "2026-03-15",
		"20260315":   "2026-03-15",
	}
	for in, want := range ok {
		d, valid := parseImportDate(in)
		if !valid {
			t.Fatalf("%q 应能解析", in)
		}
		if got := d.Format("2006-01-02"); got != want {
			t.Fatalf("%q 解析为 %s, 期望 %s", in, got, want)
		}
	}
	for _, in := range []string{"", "  ", "不是日期", "2026-13-01"} {
		if _, valid := parseImportDate(in); valid {
			t.Fatalf("%q 不应被解析成功", in)
		}
	}
}

func TestImportSkipsBadRows(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "in.csv")
	content := "ID,日期,类型,金额,类别,标签,备注,创建时间\n" +
		",2026-03-15,支出,10,餐饮,,好行,\n" +
		",不是日期,支出,10,餐饮,,坏日期,\n" +
		",2026-03-15,支出,-5,餐饮,,负金额,\n" +
		",2026-03-15,支出,abc,餐饮,,坏金额,\n" +
		",2026-03-15,支出,10,,,空类别,\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s := tempStore(t)
	n, skipped, err := s.ImportCSV(p)
	if err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	if n != 1 {
		t.Fatalf("只有 1 行合法, 实际导入 %d", n)
	}
	if skipped != 4 {
		t.Fatalf("应跳过 4 行, 实际 %d", skipped)
	}
}

// 金额列带千位分隔符或货币符号也应能导入。
func TestImportToleratesFormattedAmount(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "in.csv")
	content := "日期,类型,金额,类别\n" +
		"2026-03-15,支出,\"1,234.56\",餐饮\n" +
		"2026-03-15,收入,￥800.00,工资\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s := tempStore(t)
	n, _, err := s.ImportCSV(p)
	if err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	if n != 2 {
		t.Fatalf("应导入 2 条, 实际 %d", n)
	}
	st := s.Stats()
	if st.Expense != 1234.56 || st.Income != 800 {
		t.Fatalf("金额解析错误: 支出 %v 收入 %v", st.Expense, st.Income)
	}
}

// 英文列名也应识别。
func TestImportEnglishHeaders(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "in.csv")
	content := "date,type,amount,category,note\n" +
		"2026-03-15,expense,20,Food,lunch\n" +
		"2026-03-16,income,500,Salary,\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s := tempStore(t)
	n, skipped, err := s.ImportCSV(p)
	if err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	if n != 2 || skipped != 0 {
		t.Fatalf("应导入 2 条跳过 0, 实际 导入 %d 跳过 %d", n, skipped)
	}
	if st := s.Stats(); st.Income != 500 || st.Expense != 20 {
		t.Fatalf("英文列名解析错误: %+v", st)
	}
}

// 行尾少几列不应让整个文件读取失败。
func TestImportToleratesRaggedRows(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "in.csv")
	content := "ID,日期,类型,金额,类别,标签,备注,创建时间\n" +
		",2026-03-15,支出,10,餐饮\n" + // 缺尾部三列
		",2026-03-16,支出,20,交通,,地铁,,,额外列\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s := tempStore(t)
	n, _, err := s.ImportCSV(p)
	if err != nil {
		t.Fatalf("列数不齐时不应整体失败: %v", err)
	}
	if n != 2 {
		t.Fatalf("应导入 2 条, 实际 %d", n)
	}
}

// 导入 ID 冲突时应重新生成，而不是产生两条同 ID 记录。
func TestImportRegeneratesConflictingID(t *testing.T) {
	s := tempStore(t)
	rec := mustAdd(t, s, TypeExpense, 10, "餐饮", "", nil, time.Now())

	dir := t.TempDir()
	p := filepath.Join(dir, "in.csv")
	content := "ID,日期,类型,金额,类别\n" +
		rec.ID + ",2026-03-15,支出,99,交通\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, _, err := s.ImportCSV(p); err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	items := s.List()
	if len(items) != 2 {
		t.Fatalf("应有 2 条, 实际 %d", len(items))
	}
	if items[0].ID == items[1].ID {
		t.Fatal("ID 冲突时应重新生成")
	}
}

// 导入失败（保存失败）必须报错并回滚，不能只打印"导入成功"。
func TestImportSaveErrorRollsBack(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p := filepath.Join(dir, "in.csv")
	content := "日期,类型,金额,类别\n2026-03-15,支出,10,餐饮\n"
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s.mu.Lock()
	s.path = filepath.Join(dir, "no-such-dir", "ledger.json")
	s.mu.Unlock()

	n, _, err := s.ImportCSV(p)
	if err == nil {
		t.Fatal("保存失败时 ImportCSV 应返回错误")
	}
	if n != 0 {
		t.Fatalf("失败时不应报告导入条数, 实际 %d", n)
	}
	if len(s.List()) != 0 {
		t.Fatal("导入失败后应回滚内存状态")
	}
}

func TestExportJSON(t *testing.T) {
	s := tempStore(t)
	day := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	mustAdd(t, s, TypeExpense, 10, "餐饮", "面", []string{"外食"}, day)

	p := filepath.Join(t.TempDir(), "out.json")
	if err := s.ExportJSON(p, s.List()); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var items []Transaction
	if err := json.Unmarshal(b, &items); err != nil {
		t.Fatalf("导出的不是合法 JSON: %v", err)
	}
	if len(items) != 1 || items[0].Amount != 10 {
		t.Fatalf("导出内容错误: %+v", items)
	}
}

// 空列表应导出成 []，而不是 null（后者很多工具解析不了）。
func TestExportJSONEmptyIsArray(t *testing.T) {
	s := tempStore(t)
	p := filepath.Join(t.TempDir(), "out.json")
	if err := s.ExportJSON(p, s.List()); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	b, _ := os.ReadFile(p)
	if strings.TrimSpace(string(b)) == "null" {
		t.Fatal("空列表应导出为 [] 而不是 null")
	}
}

// 导出失败时不应破坏已存在的目标文件，也不应残留临时文件。
func TestExportFailureKeepsOldFile(t *testing.T) {
	s := tempStore(t)
	mustAdd(t, s, TypeExpense, 10, "餐饮", "", nil, time.Now())

	dir := t.TempDir()
	p := filepath.Join(dir, "out.json")
	if err := s.ExportJSON(p, s.List()); err != nil {
		t.Fatalf("首次导出: %v", err)
	}
	before, _ := os.ReadFile(p)

	// 目标目录不存在 -> CreateTemp 失败。
	bad := filepath.Join(dir, "no-such-dir", "out.json")
	if err := s.ExportJSON(bad, s.List()); err == nil {
		t.Fatal("目录不存在时导出应报错")
	}

	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("失败的导出不应影响已有文件")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("残留临时文件: %s", e.Name())
		}
	}
}
