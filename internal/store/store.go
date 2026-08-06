package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	Tags     []string  `json:"tags,omitempty"`
	Created  time.Time `json:"created"`
}

// 预设类别，供快速选择参考。
var DefaultCategories = map[string][]string{
	"income":  {"工资", "奖金", "理财", "兼职", "其他收入"},
	"expense": {"餐饮", "交通", "购物", "居住", "娱乐", "医疗", "教育", "其他支出"},
}

// Budget 表示某个月某个类别的预算上限（仅对支出生效）。
type Budget struct {
	Month   string  `json:"month"` // 2026-08
	Limit   float64 `json:"limit"`
	AlertAt float64 `json:"alert_at"` // 使用比例超过该值提醒，0~1
}

// Data 是持久化到磁盘的数据结构。
type Data struct {
	Transactions []Transaction `json:"transactions"`
	Budgets      []Budget      `json:"budgets"`
}

// Stats 描述整体统计概览。
type Stats struct {
	Count            int
	Income           float64
	Expense          float64
	Balance          float64
	Days             int
	AvgExpensePerDay float64
	FirstDate        time.Time
	LastDate         time.Time
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
	dir = strings.TrimSpace(dir)
	if dir == "" {
		if env := strings.TrimSpace(os.Getenv("LEDGERFLOW_HOME")); env != "" {
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

// save 把当前数据原子地写回磁盘。调用方必须已持有 s.mu。
//
// 原实现有两个问题：
//  1. 用 os.WriteFile 写临时文件，不 Sync 就 Rename。数据可能还在页缓存里，
//     断电后会留下一个长度正确但内容为空洞的文件。
//  2. "Windows 下 Rename 会失败所以回退成直接覆盖" 这个假设是错的 ——
//     Go 的 os.Rename 在 Windows 上用的是 MoveFileEx(MOVEFILE_REPLACE_EXISTING)，
//     覆盖已存在文件没有问题。而那个回退分支恰恰放弃了原子性：
//     os.WriteFile 会先把目标文件截断，中途崩溃就把整个账本弄丢了。
func (s *Store) save() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	f, err := os.CreateTemp(dir, ".ledger-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	// 失败路径上别留下垃圾临时文件。
	defer func() {
		if tmp != "" {
			f.Close()
			os.Remove(tmp)
		}
	}()

	if _, err := f.Write(b); err != nil {
		return err
	}
	// Sync 之后数据才真正落盘，Rename 的原子性才有意义。
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	tmp = "" // 交接成功，取消上面的清理
	return nil
}

// ErrBadAmount 表示金额非法（非正数、NaN 或 Inf）。
var ErrBadAmount = errors.New("金额必须是大于 0 的有限数")

// ErrBadType 表示交易类型非法。
var ErrBadType = errors.New("类型必须是 income 或 expense")

// ErrEmptyCategory 表示类别为空。
var ErrEmptyCategory = errors.New("类别不能为空")

// validAmount 校验金额。
// 原来完全不校验：负数能存进去（总支出会变成负的），
// NaN 更糟 —— 一旦进了账本，之后所有汇总都会变成 NaN 且再也救不回来。
func validAmount(v float64) bool {
	return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0)
}

// normalizeTags 复制并清洗标签，去掉空白项与重复项。
// 必须复制：原来直接把调用方的 slice 存进结构体，
// 调用方之后修改自己的 slice 会静默改掉已保存的数据。
func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, tg := range tags {
		tg = strings.TrimSpace(tg)
		if tg == "" || seen[tg] {
			continue
		}
		seen[tg] = true
		out = append(out, tg)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Add 新增一条记录并返回它。可选的 repeat 参数可设为 "monthly"。
// 原来这个方法没有返回 error，save() 的失败被 `_ =` 吞掉：
// 磁盘满或目录不可写时，命令行照样打印"已记录"，但数据根本没落盘。
func (s *Store) Add(t Type, amount float64, category, note string, tags []string, date time.Time, repeat ...string) (Transaction, error) {
	if t != TypeIncome && t != TypeExpense {
		return Transaction{}, ErrBadType
	}
	if !validAmount(amount) {
		return Transaction{}, ErrBadAmount
	}
	if strings.TrimSpace(category) == "" {
		return Transaction{}, ErrEmptyCategory
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	rp := ""
	if len(repeat) > 0 {
		rp = repeat[0]
	}
	rec := Transaction{
		ID:       newID(),
		Type:     string(t),
		Amount:   roundMoney(amount),
		Category: strings.TrimSpace(category),
		Note:     note,
		Tags:     normalizeTags(tags),
		Date:     date,
		Repeat:   rp,
		Created:  time.Now(),
	}
	s.data.Transactions = append(s.data.Transactions, rec)
	if err := s.save(); err != nil {
		// 回滚内存状态，别让内存和磁盘不一致。
		s.data.Transactions = s.data.Transactions[:len(s.data.Transactions)-1]
		return Transaction{}, err
	}
	// 返回值的 Tags 也要独立一份：否则调用方拿到 rec 后改 rec.Tags，
	// 又会隔着返回值把 store 里的数据改掉（和"没复制入参"是同一类问题）。
	return cloneTx(rec), nil
}

// cloneTx 返回一条记录的深拷贝（Tags 独立分配）。
func cloneTx(t Transaction) Transaction {
	if t.Tags != nil {
		tags := make([]string, len(t.Tags))
		copy(tags, t.Tags)
		t.Tags = tags
	}
	return t
}

// List 返回按日期降序的全部记录。
// 日期相同时按创建时间降序，保证顺序稳定 ——
// 原来只比 Date，同一天的多条记录顺序由 sort.Slice 的不稳定排序决定，
// 每次运行 list 看到的次序可能都不一样。
func (s *Store) List() []Transaction {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Transaction, len(s.data.Transactions))
	copy(out, s.data.Transactions)
	// copy 是浅拷贝，Tags 仍与内部数据共享底层数组，
	// 调用方改了返回值的 Tags 会污染 store。这里逐条复制一份。
	for i := range out {
		out[i] = cloneTx(out[i])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Date.Equal(out[j].Date) {
			return out[i].Date.After(out[j].Date)
		}
		return out[i].Created.After(out[j].Created)
	})
	return out
}

// Filter 按类别、类型、关键字、标签和月份筛选。空值表示不限制。
// tag 为需要全部匹配的标签；传空表示不按标签限制。
func (s *Store) Filter(category, typ, keyword, tag, month string) []Transaction {
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
		if tag != "" && !hasTag(t.Tags, tag) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// hasTag 判断标签切片中是否包含给定标签。
func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

// roundMoney 把金额规整到分。
//
// 账本用 float64 存金额本身就不理想（0.1+0.2 != 0.3，100 笔 0.01 累加得到
// 1.0000000000000007），最稳妥的做法是改用整数分。但那会改变磁盘上
// 已有 ledger.json 的字段语义，老数据全部要迁移，风险比收益大。
// 折中办法：所有入账金额先规整到分，汇总结果也规整一次，
// 把误差压在 0.005 以内不让它显示出来或继续累积。
func roundMoney(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*100) / 100
}

// RoundMoney 供其他包（report）复用同一套取整规则。
func RoundMoney(v float64) float64 { return roundMoney(v) }

// matchID 判断一条记录是否匹配用户给的 ID / ID 前缀。
// 空字符串一律不匹配 —— 原来靠 strings.HasPrefix 做前缀匹配，
// 而 HasPrefix(任意串, "") 恒为 true，导致 Get("") 和 Delete("")
// 都会命中第一条记录。探针实测 Delete("") 真的删掉了一条数据。
func matchID(recID, want string) bool {
	if want == "" {
		return false
	}
	return recID == want || strings.HasPrefix(recID, want)
}

// Get 按 ID 查找，支持前缀匹配（输入 ID 前几位即可）。
// 前缀命中多条时返回 false —— 原来是"返回碰到的第一条"，
// 在 del/edit 这类破坏性操作上等于随机挑一条改，非常危险。
func (s *Store) Get(id string) (Transaction, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getLocked(id)
}

// getLocked 是 Get 的内部版本，调用方必须已持有 s.mu。
func (s *Store) getLocked(id string) (Transaction, bool) {
	if id == "" {
		return Transaction{}, false
	}
	// 先找完全匹配。
	for _, t := range s.data.Transactions {
		if t.ID == id {
			return cloneTx(t), true
		}
	}
	// 再找前缀匹配，必须唯一。
	var hit Transaction
	n := 0
	for _, t := range s.data.Transactions {
		if strings.HasPrefix(t.ID, id) {
			hit = t
			n++
			if n > 1 {
				return Transaction{}, false
			}
		}
	}
	if n == 1 {
		return cloneTx(hit), true
	}
	return Transaction{}, false
}

// ErrAmbiguousID 表示 ID 前缀匹配到了多条记录。
var ErrAmbiguousID = errors.New("ID 前缀匹配到多条记录，请给出更长的前缀")

// ErrNotFound 表示没有找到对应记录。
var ErrNotFound = errors.New("未找到对应记录")

// resolveIndex 把用户给的 ID/前缀解析成唯一的下标。调用方必须已持有 s.mu。
func (s *Store) resolveIndex(id string) (int, error) {
	if id == "" {
		return -1, ErrNotFound
	}
	for i := range s.data.Transactions {
		if s.data.Transactions[i].ID == id {
			return i, nil
		}
	}
	idx := -1
	n := 0
	for i := range s.data.Transactions {
		if strings.HasPrefix(s.data.Transactions[i].ID, id) {
			idx = i
			n++
		}
	}
	switch {
	case n == 1:
		return idx, nil
	case n > 1:
		return -1, ErrAmbiguousID
	default:
		return -1, ErrNotFound
	}
}

// Update 修改一条已有记录，支持 ID 前缀（与 Get/Delete 行为一致；
// 原来只认完整 ID，同一个前缀 Get 得到记录、Update 却报找不到）。
// amount 传 <=0 表示不改金额；tags 非 nil 时整体覆盖标签。
func (s *Store) Update(id string, amount float64, category, note string, tags []string, date time.Time) error {
	// 原来用 amount >= 0 判断"要不要改"，负数被静默忽略却依然返回 true，
	// 用户以为改成功了（探针实测 Update(-50) 返回 true 而金额没变）。
	// 现在约定：amount <= 0 表示"不改这个字段"，NaN/Inf 直接报错。
	if math.IsNaN(amount) || math.IsInf(amount, 0) {
		return ErrBadAmount
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.resolveIndex(id)
	if err != nil {
		return err
	}

	t := &s.data.Transactions[i]
	backup := *t // 保存失败时回滚

	if amount > 0 {
		t.Amount = roundMoney(amount)
	}
	if c := strings.TrimSpace(category); c != "" {
		t.Category = c
	}
	if note != "" {
		t.Note = note
	}
	if tags != nil {
		t.Tags = normalizeTags(tags)
	}
	if !date.IsZero() {
		t.Date = date
	}

	if err := s.save(); err != nil {
		*t = backup
		return err
	}
	return nil
}

// RemoveTag 从某条记录上摘掉一个标签。
func (s *Store) RemoveTag(id, tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return errors.New("标签不能为空")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.resolveIndex(id)
	if err != nil {
		return err
	}
	t := &s.data.Transactions[i]

	// 原来写的是 kept := t.Tags[:0]，复用同一个底层数组做原地过滤。
	// 这会直接改掉调用方仍持有的那个 slice：
	// 探针实测 [a b c] 删掉 "b" 之后，调用方手里的 slice 变成了 [a c c]。
	// 这里改为分配新 slice，彻底切断共享。
	kept := make([]string, 0, len(t.Tags))
	hit := false
	for _, tg := range t.Tags {
		if tg == tag {
			hit = true
			continue
		}
		kept = append(kept, tg)
	}
	if !hit {
		return fmt.Errorf("记录上没有标签 %q", tag)
	}
	if len(kept) == 0 {
		kept = nil
	}

	backup := t.Tags
	t.Tags = kept
	if err := s.save(); err != nil {
		t.Tags = backup
		return err
	}
	return nil
}

// Delete 删除一条记录，支持前缀匹配（输入 ID 前几位即可）。
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.resolveIndex(id)
	if err != nil {
		return err
	}

	// 留一份副本，保存失败时能原样放回去。
	backup := make([]Transaction, len(s.data.Transactions))
	copy(backup, s.data.Transactions)

	s.data.Transactions = append(s.data.Transactions[:i], s.data.Transactions[i+1:]...)
	if err := s.save(); err != nil {
		s.data.Transactions = backup
		return err
	}
	return nil
}

// ValidMonth 校验月份字符串是否为合法的 2006-01 格式。
// 原来完全不校验：budget -month 2026-13 或 -month 下个月 都能存进去，
// 之后永远匹配不上任何记录，预算等于白设且没有任何提示。
func ValidMonth(m string) bool {
	_, err := time.Parse("2006-01", m)
	return err == nil
}

// SetBudget 设置某月预算。
func (s *Store) SetBudget(month string, limit, alertAt float64) error {
	if !ValidMonth(month) {
		return fmt.Errorf("月份格式应为 2006-01，得到 %q", month)
	}
	if !validAmount(limit) {
		return errors.New("预算上限必须是大于 0 的有限数")
	}
	// alertAt 是比例，原来不校验：传 80（想表示 80%）会导致永远不提醒。
	if math.IsNaN(alertAt) || alertAt < 0 || alertAt > 1 {
		return errors.New("提醒阈值应在 0~1 之间，如 0.8 表示 80%")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	limit = roundMoney(limit)
	for i := range s.data.Budgets {
		if s.data.Budgets[i].Month == month {
			backup := s.data.Budgets[i]
			s.data.Budgets[i].Limit = limit
			s.data.Budgets[i].AlertAt = alertAt
			if err := s.save(); err != nil {
				s.data.Budgets[i] = backup
				return err
			}
			return nil
		}
	}
	s.data.Budgets = append(s.data.Budgets, Budget{Month: month, Limit: limit, AlertAt: alertAt})
	if err := s.save(); err != nil {
		s.data.Budgets = s.data.Budgets[:len(s.data.Budgets)-1]
		return err
	}
	return nil
}

// GetBudget 返回某月预算，无则返回 false。
func (s *Store) GetBudget(month string) (Budget, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.data.Budgets {
		if b.Month == month {
			return b, true
		}
	}
	return Budget{}, false
}

// ListBudgets 返回全部已设置的预算，按月份升序排列。
func (s *Store) ListBudgets() []Budget {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Budget, len(s.data.Budgets))
	copy(out, s.data.Budgets)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Month < out[j].Month
	})
	return out
}

// Reset 清空全部记录与预算。
func (s *Store) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = Data{}
	return s.save()
}

// RenameCategory 把所有某旧类别的记录改成新名字，返回改了几条。
func (s *Store) RenameCategory(old, new string) (int, error) {
	old = strings.TrimSpace(old)
	new = strings.TrimSpace(new)
	if old == "" || new == "" {
		return 0, errors.New("原类别与新类别都不能为空")
	}
	if old == new {
		return 0, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	var changed []int
	for i := range s.data.Transactions {
		if s.data.Transactions[i].Category == old {
			s.data.Transactions[i].Category = new
			changed = append(changed, i)
		}
	}
	if len(changed) == 0 {
		return 0, nil
	}
	if err := s.save(); err != nil {
		for _, i := range changed {
			s.data.Transactions[i].Category = old
		}
		return 0, err
	}
	return len(changed), nil
}

// Categories 列出全部已用类别（去重），按类型分组返回。
func (s *Store) Categories() map[string][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 原来用一个共享的 seen 去重，导致同名类别只会出现在先遇到的那一组里：
	// 若"其他"既有收入又有支出，第二种类型下就看不到它了。
	// 改为每种类型各自去重。
	seenIncome := map[string]bool{}
	seenExpense := map[string]bool{}
	out := map[string][]string{"income": {}, "expense": {}}
	for _, t := range s.data.Transactions {
		if t.Type == "income" {
			if !seenIncome[t.Category] {
				seenIncome[t.Category] = true
				out["income"] = append(out["income"], t.Category)
			}
			continue
		}
		if !seenExpense[t.Category] {
			seenExpense[t.Category] = true
			out["expense"] = append(out["expense"], t.Category)
		}
	}
	sort.Strings(out["income"])
	sort.Strings(out["expense"])
	return out
}

// Stats 返回整体统计概览。
func (s *Store) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.data.Transactions
	st := Stats{}
	if len(items) == 0 {
		return st
	}
	daySet := map[string]bool{}
	var first, last time.Time
	for i, t := range items {
		if t.Type == "income" {
			st.Income += t.Amount
		} else {
			st.Expense += t.Amount
		}
		daySet[t.Date.Format("2006-01-02")] = true
		if i == 0 || t.Date.Before(first) {
			first = t.Date
		}
		if i == 0 || t.Date.After(last) {
			last = t.Date
		}
	}
	st.Count = len(items)
	// 浮点累加会带出 1.0000000000000007 这种尾巴，规整一次再对外。
	st.Income = roundMoney(st.Income)
	st.Expense = roundMoney(st.Expense)
	st.Balance = roundMoney(st.Income - st.Expense)
	st.Days = len(daySet)
	st.FirstDate = first
	st.LastDate = last
	if st.Days > 0 {
		st.AvgExpensePerDay = roundMoney(st.Expense / float64(st.Days))
	}
	return st
}

// AllTags 返回所有出现过的标签（去重，按字典序）。
func (s *Store) AllTags() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	var out []string
	for _, t := range s.data.Transactions {
		for _, tg := range t.Tags {
			if !seen[tg] {
				seen[tg] = true
				out = append(out, tg)
			}
		}
	}
	sort.Strings(out)
	return out
}

// ExportJSON 将记录导出为 JSON 文件。
// 与 ExportCSV 一样走原子写：os.WriteFile 会先截断目标文件，
// 覆盖导出时中途失败就把上一次的导出结果也弄没了。
func (s *Store) ExportJSON(path string, items []Transaction) error {
	if items == nil {
		items = []Transaction{}
	}
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, func(f *os.File) error {
		_, err := f.Write(b)
		return err
	})
}

// contains 判断 s 是否包含 sub，大小写不敏感。
// 原来手写了一个 O(n*m) 的 indexOf 来做这件事，
// 标准库的 strings.Contains 用的是 Rabin-Karp，更快也更不容易写错；
// 顺带修掉「搜索关键字必须大小写完全一致」的体验问题。
func contains(s, sub string) bool {
	if sub == "" {
		return true
	}
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
