package store

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// mustAdd 是测试里的便捷包装：Add 失败直接终止用例。
func mustAdd(t *testing.T, s *Store, typ Type, amount float64, cat, note string, tags []string, date time.Time) Transaction {
	t.Helper()
	rec, err := s.Add(typ, amount, cat, note, tags, date)
	if err != nil {
		t.Fatalf("Add(%v, %v, %q): %v", typ, amount, cat, err)
	}
	return rec
}

// ---------- 基础功能 ----------

func TestAddAndList(t *testing.T) {
	s := tempStore(t)
	now := time.Now()
	mustAdd(t, s, TypeIncome, 100, "工资", "月薪", nil, now)
	mustAdd(t, s, TypeExpense, 30, "餐饮", "午饭", nil, now)

	items := s.List()
	if len(items) != 2 {
		t.Fatalf("期望 2 条记录, 实际 %d", len(items))
	}
	// 日期相同时按创建时间降序，最新的排前面。
	if items[0].Amount != 30 {
		t.Fatalf("最新记录应为 30, 实际 %v", items[0].Amount)
	}
}

func TestFilter(t *testing.T) {
	s := tempStore(t)
	now := time.Now()
	mustAdd(t, s, TypeExpense, 10, "餐饮", "面", nil, now)
	mustAdd(t, s, TypeExpense, 20, "交通", "地铁", nil, now)
	mustAdd(t, s, TypeIncome, 500, "工资", "", nil, now)

	if got := s.Filter("餐饮", "", "", "", ""); len(got) != 1 {
		t.Fatalf("按类别筛选失败: %d", len(got))
	}
	if got := s.Filter("", "income", "", "", ""); len(got) != 1 {
		t.Fatalf("按类型筛选失败: %d", len(got))
	}
	if got := s.Filter("", "", "地铁", "", ""); len(got) != 1 {
		t.Fatalf("关键字筛选失败: %d", len(got))
	}
	if got := s.Filter("", "", "", "", now.Format("2006-01")); len(got) != 3 {
		t.Fatalf("按月份筛选失败: %d", len(got))
	}
	if got := s.Filter("", "", "", "", "1999-01"); len(got) != 0 {
		t.Fatalf("不存在的月份应返回 0 条, 实际 %d", len(got))
	}
}

func TestFilterByTag(t *testing.T) {
	s := tempStore(t)
	now := time.Now()
	mustAdd(t, s, TypeExpense, 10, "餐饮", "面", []string{"外食", "工作日"}, now)
	mustAdd(t, s, TypeExpense, 20, "交通", "地铁", []string{"工作日"}, now)

	if got := s.Filter("", "", "", "外食", ""); len(got) != 1 {
		t.Fatalf("标签 外食 应命中 1 条, 实际 %d", len(got))
	}
	if got := s.Filter("", "", "", "工作日", ""); len(got) != 2 {
		t.Fatalf("标签 工作日 应命中 2 条, 实际 %d", len(got))
	}
}

func TestUpdateDelete(t *testing.T) {
	s := tempStore(t)
	rec := mustAdd(t, s, TypeExpense, 10, "餐饮", "面", nil, time.Now())
	if err := s.Update(rec.ID, 15, "", "", nil, time.Time{}); err != nil {
		t.Fatalf("Update 应成功: %v", err)
	}
	got, ok := s.Get(rec.ID)
	if !ok {
		t.Fatal("Get 应命中")
	}
	if got.Amount != 15 {
		t.Fatalf("金额未更新: %v", got.Amount)
	}
	if err := s.Delete(rec.ID); err != nil {
		t.Fatalf("Delete 应成功: %v", err)
	}
	if len(s.List()) != 0 {
		t.Fatal("删除后应为空")
	}
}

func TestBudget(t *testing.T) {
	s := tempStore(t)
	if err := s.SetBudget("2026-08", 100, 0.8); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	b, ok := s.GetBudget("2026-08")
	if !ok || b.Limit != 100 {
		t.Fatal("预算未设置")
	}
	// 覆盖同月预算应更新而不是追加。
	if err := s.SetBudget("2026-08", 200, 0.5); err != nil {
		t.Fatalf("SetBudget 覆盖: %v", err)
	}
	if all := s.ListBudgets(); len(all) != 1 {
		t.Fatalf("同月重复设置应覆盖, 实际有 %d 条", len(all))
	}
	b, _ = s.GetBudget("2026-08")
	if b.Limit != 200 || b.AlertAt != 0.5 {
		t.Fatalf("预算未被覆盖: %+v", b)
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	s1, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rec := mustAdd(t, s1, TypeIncome, 99, "工资", "", []string{"年终"}, time.Now())

	s2, err := New(dir) // 重新打开同一目录
	if err != nil {
		t.Fatalf("重新打开: %v", err)
	}
	items := s2.List()
	if len(items) != 1 {
		t.Fatalf("持久化失败, 重新打开得到 %d 条", len(items))
	}
	if items[0].ID != rec.ID || items[0].Amount != 99 {
		t.Fatalf("持久化内容不一致: %+v", items[0])
	}
	if len(items[0].Tags) != 1 || items[0].Tags[0] != "年终" {
		t.Fatalf("标签未持久化: %v", items[0].Tags)
	}
}

// ---------- 回归测试：修复前均为真实缺陷 ----------

// 空 ID 曾经能删掉第一条记录。
// 原因：前缀匹配用 strings.HasPrefix(id, "")，而它对任意串恒为 true。
func TestEmptyIDNeverMatches(t *testing.T) {
	s := tempStore(t)
	mustAdd(t, s, TypeExpense, 10, "餐饮", "", nil, time.Now())
	mustAdd(t, s, TypeExpense, 20, "交通", "", nil, time.Now())

	if _, ok := s.Get(""); ok {
		t.Fatal("Get(\"\") 不应命中任何记录")
	}
	if err := s.Delete(""); err == nil {
		t.Fatal("Delete(\"\") 应返回错误")
	}
	if n := len(s.List()); n != 2 {
		t.Fatalf("空 ID 操作后记录数应保持 2, 实际 %d", n)
	}
	if err := s.Update("", 50, "", "", nil, time.Time{}); err == nil {
		t.Fatal("Update(\"\") 应返回错误")
	}
	if err := s.RemoveTag("", "x"); err == nil {
		t.Fatal("RemoveTag(\"\") 应返回错误")
	}
}

// 前缀命中多条时必须拒绝，而不是随便挑一条改/删。
func TestAmbiguousPrefixRejected(t *testing.T) {
	s := tempStore(t)
	// 手工构造两个共享前缀的 ID。
	s.mu.Lock()
	s.data.Transactions = []Transaction{
		{ID: "abc111", Type: "expense", Amount: 1, Category: "A", Date: time.Now()},
		{ID: "abc222", Type: "expense", Amount: 2, Category: "B", Date: time.Now()},
	}
	s.mu.Unlock()

	if _, ok := s.Get("abc"); ok {
		t.Fatal("前缀命中多条时 Get 应返回 false")
	}
	if err := s.Delete("abc"); err != ErrAmbiguousID {
		t.Fatalf("Delete 应返回 ErrAmbiguousID, 实际 %v", err)
	}
	if n := len(s.List()); n != 2 {
		t.Fatalf("歧义前缀不应删除任何记录, 实际剩 %d", n)
	}
	// 前缀唯一时应正常工作。
	if err := s.Delete("abc1"); err != nil {
		t.Fatalf("唯一前缀应删除成功: %v", err)
	}
}

// Get 与 Update/Delete 曾经行为不一致：Get 支持前缀，Update 只认完整 ID。
func TestPrefixConsistentAcrossOps(t *testing.T) {
	s := tempStore(t)
	rec := mustAdd(t, s, TypeExpense, 10, "餐饮", "", nil, time.Now())
	prefix := rec.ID[:4]

	if _, ok := s.Get(prefix); !ok {
		t.Fatal("Get 应支持前缀")
	}
	if err := s.Update(prefix, 88, "", "", nil, time.Time{}); err != nil {
		t.Fatalf("Update 也应支持前缀: %v", err)
	}
	got, _ := s.Get(rec.ID)
	if got.Amount != 88 {
		t.Fatalf("前缀更新未生效: %v", got.Amount)
	}
	if err := s.Delete(prefix); err != nil {
		t.Fatalf("Delete 也应支持前缀: %v", err)
	}
}

// RemoveTag 曾经用 t.Tags[:0] 原地过滤，会把调用方手里的 slice 改成 [a c c]。
func TestRemoveTagDoesNotCorruptCallerSlice(t *testing.T) {
	s := tempStore(t)
	callerTags := []string{"a", "b", "c"}
	rec := mustAdd(t, s, TypeExpense, 10, "餐饮", "", callerTags, time.Now())

	if err := s.RemoveTag(rec.ID, "b"); err != nil {
		t.Fatalf("RemoveTag: %v", err)
	}

	want := []string{"a", "b", "c"}
	for i := range want {
		if callerTags[i] != want[i] {
			t.Fatalf("调用方的 slice 被篡改: %v (期望 %v)", callerTags, want)
		}
	}
	got, _ := s.Get(rec.ID)
	if len(got.Tags) != 2 || got.Tags[0] != "a" || got.Tags[1] != "c" {
		t.Fatalf("标签删除结果错误: %v", got.Tags)
	}
}

// Add 曾经直接持有调用方的 tags slice，调用方之后改动会静默污染已存数据。
func TestAddCopiesTags(t *testing.T) {
	s := tempStore(t)
	tags := []string{"旅行", "团建"}
	rec := mustAdd(t, s, TypeExpense, 10, "娱乐", "", tags, time.Now())

	tags[0] = "被改掉了"
	got, _ := s.Get(rec.ID)
	if got.Tags[0] != "旅行" {
		t.Fatalf("store 内数据被外部修改污染: %v", got.Tags)
	}
	// 返回值本身也不能与内部共享。
	rec.Tags[1] = "又改一次"
	got2, _ := s.Get(rec.ID)
	if got2.Tags[1] != "团建" {
		t.Fatalf("返回值与内部共享底层数组: %v", got2.Tags)
	}
}

// List 返回的 Tags 曾经与内部共享底层数组。
func TestListReturnsDeepCopy(t *testing.T) {
	s := tempStore(t)
	mustAdd(t, s, TypeExpense, 10, "餐饮", "", []string{"x", "y"}, time.Now())

	items := s.List()
	items[0].Tags[0] = "污染"

	fresh := s.List()
	if fresh[0].Tags[0] != "x" {
		t.Fatalf("List 返回的是浅拷贝, 内部数据被污染: %v", fresh[0].Tags)
	}
}

// 负数 / 0 / NaN / Inf 金额曾经能存进账本，之后所有汇总都会被带歪。
func TestAddRejectsInvalidAmount(t *testing.T) {
	s := tempStore(t)
	bad := []float64{0, -1, -100, math.NaN(), math.Inf(1), math.Inf(-1)}
	for _, v := range bad {
		if _, err := s.Add(TypeExpense, v, "餐饮", "", nil, time.Now()); err == nil {
			t.Fatalf("金额 %v 应被拒绝", v)
		}
	}
	if n := len(s.List()); n != 0 {
		t.Fatalf("非法金额不应写入, 实际写入 %d 条", n)
	}
	if st := s.Stats(); st.Expense != 0 {
		t.Fatalf("总支出应为 0, 实际 %v", st.Expense)
	}
}

func TestAddRejectsBadTypeAndCategory(t *testing.T) {
	s := tempStore(t)
	if _, err := s.Add(Type("transfer"), 10, "餐饮", "", nil, time.Now()); err != ErrBadType {
		t.Fatalf("非法类型应返回 ErrBadType, 实际 %v", err)
	}
	if _, err := s.Add(TypeExpense, 10, "   ", "", nil, time.Now()); err != ErrEmptyCategory {
		t.Fatalf("空类别应返回 ErrEmptyCategory, 实际 %v", err)
	}
}

// 浮点累加会带出 1.0000000000000007 这种尾巴。
func TestMoneyRounding(t *testing.T) {
	s := tempStore(t)
	now := time.Now()
	for i := 0; i < 100; i++ {
		mustAdd(t, s, TypeExpense, 0.01, "餐饮", "", nil, now)
	}
	st := s.Stats()
	if st.Expense != 1.0 {
		t.Fatalf("100 笔 0.01 应等于 1.00, 实际 %v", st.Expense)
	}

	s2 := tempStore(t)
	mustAdd(t, s2, TypeIncome, 0.1, "工资", "", nil, now)
	mustAdd(t, s2, TypeIncome, 0.2, "工资", "", nil, now)
	if got := s2.Stats().Income; got != 0.3 {
		t.Fatalf("0.1+0.2 应等于 0.3, 实际 %v", got)
	}
	// 入账时也应规整到分。
	rec := mustAdd(t, s2, TypeExpense, 3.14159, "餐饮", "", nil, now)
	if rec.Amount != 3.14 {
		t.Fatalf("金额应规整到分, 实际 %v", rec.Amount)
	}
}

// 非法月份曾经能存成预算，之后永远匹配不上任何记录。
func TestSetBudgetValidation(t *testing.T) {
	s := tempStore(t)
	cases := []struct {
		month   string
		limit   float64
		alertAt float64
		desc    string
	}{
		{"2026-13", 100, 0.8, "月份 13"},
		{"下个月", 100, 0.8, "非日期月份"},
		{"2026-8", 100, 0.8, "月份未补零"},
		{"", 100, 0.8, "空月份"},
		{"2026-08", 0, 0.8, "预算 0"},
		{"2026-08", -100, 0.8, "预算负数"},
		{"2026-08", math.NaN(), 0.8, "预算 NaN"},
		{"2026-08", 100, 80, "阈值写成 80 而非 0.8"},
		{"2026-08", 100, -0.1, "阈值负数"},
	}
	for _, c := range cases {
		if err := s.SetBudget(c.month, c.limit, c.alertAt); err == nil {
			t.Fatalf("%s 应被拒绝", c.desc)
		}
	}
	if n := len(s.ListBudgets()); n != 0 {
		t.Fatalf("非法预算不应写入, 实际 %d 条", n)
	}
	if err := s.SetBudget("2026-08", 100, 0.8); err != nil {
		t.Fatalf("合法预算应成功: %v", err)
	}
}

// Update 传负数金额曾经返回 true 但什么都没改。
func TestUpdateNegativeAmountMeansNoChange(t *testing.T) {
	s := tempStore(t)
	rec := mustAdd(t, s, TypeExpense, 10, "餐饮", "", nil, time.Now())

	if err := s.Update(rec.ID, -50, "交通", "", nil, time.Time{}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := s.Get(rec.ID)
	if got.Amount != 10 {
		t.Fatalf("负数金额不应改动金额, 实际 %v", got.Amount)
	}
	if got.Category != "交通" {
		t.Fatalf("其他字段仍应更新, 实际 %q", got.Category)
	}
	if err := s.Update(rec.ID, math.NaN(), "", "", nil, time.Time{}); err != ErrBadAmount {
		t.Fatalf("NaN 金额应返回 ErrBadAmount, 实际 %v", err)
	}
}

// Update 不传日期时不应把日期重置成零值。
func TestUpdateKeepsDateWhenZero(t *testing.T) {
	s := tempStore(t)
	orig := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	rec := mustAdd(t, s, TypeExpense, 10, "餐饮", "", nil, orig)

	if err := s.Update(rec.ID, 20, "", "", nil, time.Time{}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := s.Get(rec.ID)
	if !got.Date.Equal(orig) {
		t.Fatalf("日期不应被改动: %v", got.Date)
	}
}

// 同名类别同时存在收入和支出时，原来只会出现在先遇到的那一组。
func TestCategoriesPerTypeDedup(t *testing.T) {
	s := tempStore(t)
	now := time.Now()
	mustAdd(t, s, TypeIncome, 100, "其他", "", nil, now)
	mustAdd(t, s, TypeExpense, 50, "其他", "", nil, now)
	mustAdd(t, s, TypeExpense, 50, "餐饮", "", nil, now)

	cats := s.Categories()
	if len(cats["income"]) != 1 || cats["income"][0] != "其他" {
		t.Fatalf("收入类别错误: %v", cats["income"])
	}
	if len(cats["expense"]) != 2 {
		t.Fatalf("支出类别应有 2 个（其他/餐饮）, 实际 %v", cats["expense"])
	}
	// 应按字典序排列，输出稳定。
	if cats["expense"][0] > cats["expense"][1] {
		t.Fatalf("类别未排序: %v", cats["expense"])
	}
}

// 关键字搜索应大小写不敏感。
func TestKeywordSearchCaseInsensitive(t *testing.T) {
	s := tempStore(t)
	mustAdd(t, s, TypeExpense, 10, "Food", "Starbucks Latte", nil, time.Now())

	for _, q := range []string{"starbucks", "STARBUCKS", "Latte", "food", "FOOD"} {
		if got := s.Filter("", "", q, "", ""); len(got) != 1 {
			t.Fatalf("关键字 %q 应命中 1 条, 实际 %d", q, len(got))
		}
	}
}

// 标签应去重、去空白。
func TestTagsNormalized(t *testing.T) {
	s := tempStore(t)
	rec := mustAdd(t, s, TypeExpense, 10, "餐饮", "",
		[]string{" 旅行 ", "旅行", "", "   ", "团建"}, time.Now())
	if len(rec.Tags) != 2 {
		t.Fatalf("标签应去重去空白后剩 2 个, 实际 %v", rec.Tags)
	}
	if rec.Tags[0] != "旅行" || rec.Tags[1] != "团建" {
		t.Fatalf("标签内容错误: %v", rec.Tags)
	}
	if all := s.AllTags(); len(all) != 2 {
		t.Fatalf("AllTags 应返回 2 个, 实际 %v", all)
	}
}

// save 失败必须向上报错，且内存状态回滚到与磁盘一致。
func TestSaveErrorPropagatesAndRollsBack(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mustAdd(t, s, TypeExpense, 10, "餐饮", "", nil, time.Now())

	// 把数据目录换成一个不存在的路径，让 CreateTemp 必然失败。
	s.mu.Lock()
	s.path = filepath.Join(dir, "no-such-dir", "ledger.json")
	s.mu.Unlock()

	if _, err := s.Add(TypeExpense, 20, "交通", "", nil, time.Now()); err == nil {
		t.Fatal("目录不存在时 Add 应返回错误")
	}
	if n := len(s.List()); n != 1 {
		t.Fatalf("Add 失败后应回滚, 记录数应为 1, 实际 %d", n)
	}

	rec := s.List()[0]
	if err := s.Update(rec.ID, 999, "", "", nil, time.Time{}); err == nil {
		t.Fatal("Update 在保存失败时应返回错误")
	}
	if got, _ := s.Get(rec.ID); got.Amount != 10 {
		t.Fatalf("Update 失败后应回滚金额, 实际 %v", got.Amount)
	}

	if err := s.Delete(rec.ID); err == nil {
		t.Fatal("Delete 在保存失败时应返回错误")
	}
	if n := len(s.List()); n != 1 {
		t.Fatalf("Delete 失败后应回滚, 实际剩 %d 条", n)
	}
}

// 写盘应该是原子的：不留临时文件，且内容始终是完整 JSON。
func TestSaveIsAtomicAndLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for i := 0; i < 5; i++ {
		mustAdd(t, s, TypeExpense, float64(i+1), "餐饮", "", nil, time.Now())
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("残留临时文件: %s", e.Name())
		}
	}

	b, err := os.ReadFile(filepath.Join(dir, "ledger.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var d Data
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("磁盘文件不是完整 JSON: %v", err)
	}
	if len(d.Transactions) != 5 {
		t.Fatalf("磁盘上应有 5 条, 实际 %d", len(d.Transactions))
	}
}

// 所有读方法过去都不加锁，与写并发时会读到撕裂的数据甚至 panic。
func TestConcurrentReadWriteNoPanic(t *testing.T) {
	s := tempStore(t)
	now := time.Now()
	mustAdd(t, s, TypeExpense, 10, "餐饮", "", []string{"a"}, now)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_, _ = s.Add(TypeExpense, float64(i+1), "餐饮", "", []string{"a", "b"}, now)
		}
		close(stop)
	}()

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = s.List()
				_ = s.Stats()
				_ = s.Categories()
				_ = s.AllTags()
				_ = s.ListBudgets()
				_, _ = s.GetBudget("2026-08")
			}
		}()
	}
	wg.Wait()
}

func TestRenameCategory(t *testing.T) {
	s := tempStore(t)
	now := time.Now()
	mustAdd(t, s, TypeExpense, 10, "餐饮", "", nil, now)
	mustAdd(t, s, TypeExpense, 20, "餐饮", "", nil, now)
	mustAdd(t, s, TypeExpense, 30, "交通", "", nil, now)

	n, err := s.RenameCategory("餐饮", "吃饭")
	if err != nil {
		t.Fatalf("RenameCategory: %v", err)
	}
	if n != 2 {
		t.Fatalf("应改动 2 条, 实际 %d", n)
	}
	if got := s.Filter("吃饭", "", "", "", ""); len(got) != 2 {
		t.Fatalf("重命名未生效: %d", len(got))
	}
	// 空参数应报错而不是静默清空类别。
	if _, err := s.RenameCategory("", "x"); err == nil {
		t.Fatal("空的原类别应报错")
	}
	if _, err := s.RenameCategory("交通", ""); err == nil {
		t.Fatal("空的新类别应报错")
	}
	// 不存在的类别改 0 条，不应报错。
	if n, err := s.RenameCategory("不存在", "y"); err != nil || n != 0 {
		t.Fatalf("不存在的类别应返回 (0, nil), 实际 (%d, %v)", n, err)
	}
}

func TestRemoveTagErrors(t *testing.T) {
	s := tempStore(t)
	rec := mustAdd(t, s, TypeExpense, 10, "餐饮", "", []string{"a"}, time.Now())

	if err := s.RemoveTag(rec.ID, ""); err == nil {
		t.Fatal("空标签应报错")
	}
	if err := s.RemoveTag(rec.ID, "不存在的标签"); err == nil {
		t.Fatal("删除不存在的标签应报错")
	}
	if err := s.RemoveTag("ffffffffff", "a"); err != ErrNotFound {
		t.Fatalf("不存在的 ID 应返回 ErrNotFound, 实际 %v", err)
	}
	// 删光之后 Tags 应为 nil，而不是空切片，保证 JSON 里被 omitempty 掉。
	if err := s.RemoveTag(rec.ID, "a"); err != nil {
		t.Fatalf("RemoveTag: %v", err)
	}
	got, _ := s.Get(rec.ID)
	if got.Tags != nil {
		t.Fatalf("标签删光后应为 nil, 实际 %v", got.Tags)
	}
}

func TestStatsEmptyAndBasic(t *testing.T) {
	s := tempStore(t)
	if st := s.Stats(); st.Count != 0 || st.Balance != 0 || st.Days != 0 {
		t.Fatalf("空账本统计应全为 0: %+v", st)
	}

	d1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	mustAdd(t, s, TypeIncome, 1000, "工资", "", nil, d1)
	mustAdd(t, s, TypeExpense, 300, "餐饮", "", nil, d1)
	mustAdd(t, s, TypeExpense, 200, "交通", "", nil, d2)

	st := s.Stats()
	if st.Count != 3 {
		t.Fatalf("Count 应为 3, 实际 %d", st.Count)
	}
	if st.Income != 1000 || st.Expense != 500 || st.Balance != 500 {
		t.Fatalf("收支统计错误: %+v", st)
	}
	if st.Days != 2 {
		t.Fatalf("Days 应为 2, 实际 %d", st.Days)
	}
	if st.AvgExpensePerDay != 250 {
		t.Fatalf("日均支出应为 250, 实际 %v", st.AvgExpensePerDay)
	}
	if !st.FirstDate.Equal(d1) || !st.LastDate.Equal(d2) {
		t.Fatalf("首末日期错误: %v ~ %v", st.FirstDate, st.LastDate)
	}
}

func TestValidMonth(t *testing.T) {
	ok := []string{"2026-01", "2026-12", "1999-06"}
	bad := []string{"", "2026-13", "2026-00", "2026-1", "26-01", "2026/01", "下个月"}
	for _, m := range ok {
		if !ValidMonth(m) {
			t.Fatalf("%q 应为合法月份", m)
		}
	}
	for _, m := range bad {
		if ValidMonth(m) {
			t.Fatalf("%q 应为非法月份", m)
		}
	}
}

func TestReset(t *testing.T) {
	s := tempStore(t)
	mustAdd(t, s, TypeExpense, 10, "餐饮", "", nil, time.Now())
	if err := s.SetBudget("2026-08", 100, 0.8); err != nil {
		t.Fatalf("SetBudget: %v", err)
	}
	if err := s.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	if len(s.List()) != 0 || len(s.ListBudgets()) != 0 {
		t.Fatal("Reset 后应全部清空")
	}
}

func TestRoundMoneyExported(t *testing.T) {
	cases := map[float64]float64{
		3.14159:  3.14,
		2.005:    2.01,
		-1.005:   -1.0,
		0:        0,
		1.0 / 3:  0.33,
		99.99999: 100,
	}
	for in, want := range cases {
		if got := RoundMoney(in); math.Abs(got-want) > 1e-9 {
			t.Fatalf("RoundMoney(%v) = %v, 期望 %v", in, got, want)
		}
	}
	if got := RoundMoney(math.NaN()); got != 0 {
		t.Fatalf("NaN 应规整为 0, 实际 %v", got)
	}
	if got := RoundMoney(math.Inf(1)); got != 0 {
		t.Fatalf("Inf 应规整为 0, 实际 %v", got)
	}
}

func TestListSortStable(t *testing.T) {
	s := tempStore(t)
	day := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	// 同一天连续插入多条，顺序必须每次一致。
	for i := 0; i < 8; i++ {
		mustAdd(t, s, TypeExpense, float64(i+1), "餐饮", "", nil, day)
	}
	first := s.List()
	for i := 0; i < 20; i++ {
		cur := s.List()
		for j := range cur {
			if cur[j].ID != first[j].ID {
				t.Fatalf("第 %d 次 List 顺序发生变化", i)
			}
		}
	}
}

func TestNewLoadsCorruptedFileAsError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ledger.json"), []byte("{不是 json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := New(dir); err == nil {
		t.Fatal("损坏的账本文件应返回错误而不是静默清空")
	}
}

func TestNewHandlesEmptyFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ledger.json"), nil, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	s, err := New(dir)
	if err != nil {
		t.Fatalf("空文件应被当作空账本: %v", err)
	}
	if len(s.List()) != 0 {
		t.Fatal("空文件应解析为 0 条记录")
	}
}
