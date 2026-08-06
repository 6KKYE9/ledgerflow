package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Type 是交易类型。
type Type string

const (
	// TypeIncome 收入。
	TypeIncome Type = "income"
	// TypeExpense 支出。
	TypeExpense Type = "expense"
)

// Transaction 表示一条收支记录。
type Transaction struct {
	ID       string    `json:"id"`
	Type     string    `json:"type"` // income | expense
	Amount   float64   `json:"amount"`
	Category string    `json:"category"`
	Note     string    `json:"note"`
	Date     time.Time `json:"date"`
	Repeat   string    `json:"repeat,omitempty"` // "" | "monthly"
	Created  time.Time `json:"created"`
}

// 预设类别，供快速选择参考。
var DefaultCategories = map[string][]string{
	"income":  {"工资", "奖金", "理财", "兼职", "其他收入"},
	"expense": {"餐饮", "交通", "购物", "居住", "娱乐", "医疗", "教育", "其他支出"},
}

// Budget 表示某个月某个类别的预算上限（仅对支出生效）。
type Budget struct {
	Month   string  `json:"month"`   // 2026-08
	Limit   float64 `json:"limit"`
	AlertAt float64 `json:"alert_at"` // 使用比例超过该值提醒，0~1
}

// Data 是持久化到磁盘的数据结构。
type Data struct {
	Transactions []Transaction `json:"transactions"`
	Budgets      []Budget      `json:"budgets"`
}

// Store 负责数据的加载、保存与查询。
type Store struct {
	mu   sync.Mutex
	path string
	data Data
}

// New 在给定目录创建/打开存储文件 ledger.json。
// 若 dir 为空，依次使用环境变量 LEDGERFLOW_HOME、用户主目录下的 .ledgerflow。
func New(dir string) (*Store, error) {
	if dir == "" {
		if env := os.Getenv("LEDGERFLOW_HOME"); env != "" {
			dir = env
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			dir = filepath.Join(home, ".ledgerflow")
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "ledger.json")
	s := &Store{path: path}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.data = Data{}
			return nil
		}
		return err
	}
	if len(b) == 0 {
		s.data = Data{}
		return nil
	}
	return json.Unmarshal(b, &s.data)
}

func (s *Store) save() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Add 新增一条记录并返回它。可选的 repeat 参数可设为 "monthly"。
func (s *Store) Add(t Type, amount float64, category, note string, date time.Time, repeat ...string) Transaction {
	s.mu.Lock()
	defer s.mu.Unlock()
	rp := ""
	if len(repeat) > 0 {
		rp = repeat[0]
	}
	rec := Transaction{
		ID:       newID(),
		Type:     string(t),
		Amount:   amount,
		Category: category,
		Note:     note,
		Date:     date,
		Repeat:   rp,
		Created:  time.Now(),
	}
	s.data.Transactions = append(s.data.Transactions, rec)
	_ = s.save()
	return rec
}

// List 返回按日期降序的全部记录。
func (s *Store) List() []Transaction {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Transaction, len(s.data.Transactions))
	copy(out, s.data.Transactions)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Date.After(out[j].Date)
	})
	return out
}

// Filter 按类别、类型、关键字和月份筛选。空值表示不限制。
func (s *Store) Filter(category, typ, keyword, month string) []Transaction {
	all := s.List()
	var out []Transaction
	for _, t := range all {
		if category != "" && t.Category != category {
			continue
		}
		if typ != "" && t.Type != typ {
			continue
		}
		if month != "" && t.Date.Format("2006-01") != month {
			continue
		}
		if keyword != "" && !contains(t.Note, keyword) && !contains(t.Category, keyword) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// Get 按 ID 查找。
func (s *Store) Get(id string) (Transaction, bool) {
	for _, t := range s.data.Transactions {
		if t.ID == id {
			return t, true
		}
	}
	return Transaction{}, false
}

// Update 修改一条已有记录。
func (s *Store) Update(id string, amount float64, category, note string, date time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Transactions {
		if s.data.Transactions[i].ID == id {
			t := &s.data.Transactions[i]
			if amount >= 0 {
				t.Amount = amount
			}
			if category != "" {
				t.Category = category
			}
			if note != "" {
				t.Note = note
			}
			if !date.IsZero() {
				t.Date = date
			}
			_ = s.save()
			return true
		}
	}
	return false
}

// Delete 删除一条记录。
func (s *Store) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, t := range s.data.Transactions {
		if t.ID == id {
			s.data.Transactions = append(s.data.Transactions[:i], s.data.Transactions[i+1:]...)
			_ = s.save()
			return true
		}
	}
	return false
}

// SetBudget 设置某月预算。
func (s *Store) SetBudget(month string, limit, alertAt float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Budgets {
		if s.data.Budgets[i].Month == month {
			s.data.Budgets[i].Limit = limit
			s.data.Budgets[i].AlertAt = alertAt
			_ = s.save()
			return
		}
	}
	s.data.Budgets = append(s.data.Budgets, Budget{Month: month, Limit: limit, AlertAt: alertAt})
	_ = s.save()
}

// GetBudget 返回某月预算，无则返回 false。
func (s *Store) GetBudget(month string) (Budget, bool) {
	for _, b := range s.data.Budgets {
		if b.Month == month {
			return b, true
		}
	}
	return Budget{}, false
}

// Reset 清空全部记录与预算。
func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = Data{}
	return s.save()
}

// Categories 列出全部已用类别（去重），按类型分组返回。
func (s *Store) Categories() map[string][]string {
	seen := map[string]bool{}
	out := map[string][]string{"income": {}, "expense": {}}
	for _, t := range s.data.Transactions {
		if seen[t.Category] {
			continue
		}
		seen[t.Category] = true
		if t.Type == "income" {
			out["income"] = append(out["income"], t.Category)
		} else {
			out["expense"] = append(out["expense"], t.Category)
		}
	}
	return out
}

// ExportJSON 将记录导出为 JSON 文件。
func (s *Store) ExportJSON(path string, items []Transaction) error {
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
