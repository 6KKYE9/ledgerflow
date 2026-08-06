package store

import (
	"path/filepath"
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

func TestAddAndList(t *testing.T) {
	s := tempStore(t)
	s.Add(TypeIncome, 100, "工资", "月薪", time.Now())
	s.Add(TypeExpense, 30, "餐饮", "午饭", time.Now())

	items := s.List()
	if len(items) != 2 {
		t.Fatalf("期望 2 条记录, 实际 %d", len(items))
	}
	// 列表按日期降序，这里日期相同，按插入顺序反向
	if items[0].Amount != 30 {
		t.Fatalf("最新记录应为 30, 实际 %v", items[0].Amount)
	}
}

func TestFilter(t *testing.T) {
	s := tempStore(t)
	now := time.Now()
	s.Add(TypeExpense, 10, "餐饮", "面", now)
	s.Add(TypeExpense, 20, "交通", "地铁", now)
	s.Add(TypeIncome, 500, "工资", "", now)

	if got := s.Filter("餐饮", "", "", ""); len(got) != 1 {
		t.Fatalf("按类别筛选失败: %d", len(got))
	}
	if got := s.Filter("", "income", "", ""); len(got) != 1 {
		t.Fatalf("按类型筛选失败: %d", len(got))
	}
	if got := s.Filter("", "", "地铁", ""); len(got) != 1 {
		t.Fatalf("关键字筛选失败: %d", len(got))
	}
}

func TestUpdateDelete(t *testing.T) {
	s := tempStore(t)
	rec := s.Add(TypeExpense, 10, "餐饮", "面", time.Now())
	if !s.Update(rec.ID, 15, "", "", time.Time{}) {
		t.Fatal("Update 应成功")
	}
	got, _ := s.Get(rec.ID)
	if got.Amount != 15 {
		t.Fatalf("金额未更新: %v", got.Amount)
	}
	if !s.Delete(rec.ID) {
		t.Fatal("Delete 应成功")
	}
	if len(s.List()) != 0 {
		t.Fatal("删除后应为空")
	}
}

func TestBudget(t *testing.T) {
	s := tempStore(t)
	s.SetBudget("2026-08", 100, 0.8)
	b, ok := s.GetBudget("2026-08")
	if !ok || b.Limit != 100 {
		t.Fatal("预算未设置")
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	s1, _ := New(dir)
	s1.Add(TypeIncome, 99, "工资", "", time.Now())
	s2, _ := New(dir) // 重新打开同一目录
	if len(s2.List()) != 1 {
		t.Fatalf("持久化失败, 重新打开得到 %d 条", len(s2.List()))
	}
	_ = filepath.Join
}
