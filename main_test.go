package main

import (
	"math"
	"testing"
	"time"
)

func TestParseAmountPlain(t *testing.T) {
	cases := map[string]float64{
		"0.01":   0.01,
		"1":      1,
		"38.5":   38.5,
		"  42  ": 42,
		"1000":   1000,
	}
	for in, want := range cases {
		got, err := parseAmount(in)
		if err != nil {
			t.Fatalf("parseAmount(%q): %v", in, err)
		}
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("parseAmount(%q) = %v, 期望 %v", in, got, want)
		}
	}
}

// 中文单位后缀原来完全失效：s[len(s)-1:] 取的是字节不是字符，
// "千"/"万" 是 3 字节，永远匹配不上 case，"3千" 会直接报错。
func TestParseAmountChineseUnits(t *testing.T) {
	cases := map[string]float64{
		"3千":    3000,
		"5万":    50000,
		"2.5万":  25000,
		"0.5千":  500,
		"1亿":    1e8,
		"1.23万": 12300,
	}
	for in, want := range cases {
		got, err := parseAmount(in)
		if err != nil {
			t.Fatalf("parseAmount(%q) 应成功（中文单位曾经完全失效）: %v", in, err)
		}
		if math.Abs(got-want) > 1e-6 {
			t.Fatalf("parseAmount(%q) = %v, 期望 %v", in, got, want)
		}
	}
}

func TestParseAmountLatinUnits(t *testing.T) {
	cases := map[string]float64{
		"1k":   1000,
		"1K":   1000,
		"2.5w": 25000,
		"2.5W": 25000,
	}
	for in, want := range cases {
		got, err := parseAmount(in)
		if err != nil {
			t.Fatalf("parseAmount(%q): %v", in, err)
		}
		if math.Abs(got-want) > 1e-6 {
			t.Fatalf("parseAmount(%q) = %v, 期望 %v", in, got, want)
		}
	}
}

// 粘贴过来的金额常带千位分隔符或货币符号。
func TestParseAmountToleratesSeparators(t *testing.T) {
	cases := map[string]float64{
		"1,234.56": 1234.56,
		"￥800":     800,
		"$99.9":    99.9,
		"1,0k":     10000,
	}
	for in, want := range cases {
		got, err := parseAmount(in)
		if err != nil {
			t.Fatalf("parseAmount(%q): %v", in, err)
		}
		if math.Abs(got-want) > 1e-6 {
			t.Fatalf("parseAmount(%q) = %v, 期望 %v", in, got, want)
		}
	}
}

// NaN / Inf 一旦入账，之后所有汇总都会永久变成 NaN。
func TestParseAmountRejectsBad(t *testing.T) {
	bad := []string{
		"", "   ", "abc", "十块", "1.2.3",
		"NaN", "nan", "Inf", "+Inf", "-Inf", "inf",
		"1e400",       // 溢出成 +Inf
		"k", "万", "千", // 只有单位没有数字
	}
	for _, in := range bad {
		if v, err := parseAmount(in); err == nil {
			t.Fatalf("parseAmount(%q) 应报错, 实际返回 %v", in, v)
		}
	}
}

func TestParseTags(t *testing.T) {
	cases := map[string][]string{
		"":          nil,
		"   ":       nil,
		"旅行":        {"旅行"},
		"旅行|团建":     {"旅行", "团建"},
		"旅行、团建":     {"旅行", "团建"},
		"旅行,团建":     {"旅行", "团建"},
		" 旅行 | 团建 ": {"旅行", "团建"},
		"旅行||团建":    {"旅行", "团建"},
		"旅行|  |团建":  {"旅行", "团建"},
		"a|b、c,d":   {"a", "b", "c", "d"},
	}
	for in, want := range cases {
		got := parseTags(in)
		if len(got) != len(want) {
			t.Fatalf("parseTags(%q) = %v, 期望 %v", in, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("parseTags(%q) = %v, 期望 %v", in, got, want)
			}
		}
	}
}

func TestStringSlice(t *testing.T) {
	var s stringSlice
	if s.String() != "" {
		t.Fatalf("空切片应返回空串, 实际 %q", s.String())
	}
	_ = s.Set("a")
	_ = s.Set("b")
	if len(s) != 2 || s[0] != "a" || s[1] != "b" {
		t.Fatalf("Set 结果错误: %v", s)
	}
	if s.String() != "a,b" {
		t.Fatalf("String 应为 a,b, 实际 %q", s.String())
	}
}

func TestParseDate(t *testing.T) {
	d := parseDate("2026-03-15")
	if d.Format("2006-01-02") != "2026-03-15" {
		t.Fatalf("日期解析错误: %v", d)
	}
	// 空串与非法输入都回退到今天。
	today := time.Now().Format("2006-01-02")
	for _, in := range []string{"", "不是日期", "2026-13-99"} {
		if got := parseDate(in).Format("2006-01-02"); got != today {
			t.Fatalf("parseDate(%q) 应回退到今天 %s, 实际 %s", in, today, got)
		}
	}
}

// AddDate 对月末是溢出语义：1-31 加一个月会得到 3-2/3-3。
// "每月 31 号的房租"因此会莫名跑到下下个月，统计和预算全部错位。
func TestAddMonthsClampedEndOfMonth(t *testing.T) {
	base := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		n    int
		want string
	}{
		{1, "2026-02-28"}, // 2026 非闰年
		{2, "2026-03-31"},
		{3, "2026-04-30"},
		{4, "2026-05-31"},
		{5, "2026-06-30"},
		{11, "2026-12-31"},
		{12, "2027-01-31"},
		{13, "2027-02-28"},
	}
	for _, c := range cases {
		got := addMonthsClamped(base, c.n).Format("2006-01-02")
		if got != c.want {
			t.Fatalf("addMonthsClamped(1-31, +%d) = %s, 期望 %s", c.n, got, c.want)
		}
	}
	// 对照：标准库的溢出行为确实存在，说明这个 helper 不是多余的。
	if overflow := base.AddDate(0, 1, 0).Format("2006-01-02"); overflow == "2026-02-28" {
		t.Skip("当前 Go 版本 AddDate 行为已变，跳过对照")
	}
}

func TestAddMonthsClampedLeapYear(t *testing.T) {
	// 2028 是闰年，2 月有 29 天。
	base := time.Date(2028, 1, 31, 0, 0, 0, 0, time.UTC)
	if got := addMonthsClamped(base, 1).Format("2006-01-02"); got != "2028-02-29" {
		t.Fatalf("闰年 2 月应钳制到 29 号, 实际 %s", got)
	}
	// 2 月 29 号加一年应落到 2 月 28 号，而不是 3 月 1 号。
	feb29 := time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC)
	if got := addMonthsClamped(feb29, 12).Format("2006-01-02"); got != "2029-02-28" {
		t.Fatalf("2-29 加 12 个月应为 2029-02-28, 实际 %s", got)
	}
}

func TestAddMonthsClampedNormalDays(t *testing.T) {
	// 月初/月中不受影响，行为应与 AddDate 一致。
	base := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	for n := 0; n <= 24; n++ {
		got := addMonthsClamped(base, n)
		want := base.AddDate(0, n, 0)
		if !got.Equal(want) {
			t.Fatalf("+%d 个月: %v, 期望 %v", n, got, want)
		}
	}
}

func TestAddMonthsClampedPreservesTimeAndLocation(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	base := time.Date(2026, 1, 31, 13, 45, 30, 0, loc)
	got := addMonthsClamped(base, 1)
	if got.Hour() != 13 || got.Minute() != 45 || got.Second() != 30 {
		t.Fatalf("时分秒应保留: %v", got)
	}
	if got.Location() != loc {
		t.Fatalf("时区应保留: %v", got.Location())
	}
}

func TestDaysInMonth(t *testing.T) {
	cases := []struct {
		y    int
		m    time.Month
		want int
	}{
		{2026, time.January, 31},
		{2026, time.February, 28},
		{2028, time.February, 29}, // 闰年
		{2000, time.February, 29}, // 能被 400 整除，是闰年
		{1900, time.February, 28}, // 能被 100 整除但不能被 400 整除，不是闰年
		{2026, time.April, 30},
		{2026, time.December, 31},
	}
	for _, c := range cases {
		if got := daysInMonth(c.y, c.m); got != c.want {
			t.Fatalf("daysInMonth(%d, %v) = %d, 期望 %d", c.y, c.m, got, c.want)
		}
	}
}

func TestPad(t *testing.T) {
	if pad(3) != "   " {
		t.Fatalf("pad(3) 应为 3 个空格, 实际 %q", pad(3))
	}
	if pad(0) != "" {
		t.Fatalf("pad(0) 应为空串")
	}
	// 负数不应 panic（strings.Repeat 对负数会 panic）。
	if pad(-5) != "" {
		t.Fatalf("pad(-5) 应为空串, 实际 %q", pad(-5))
	}
}

func TestDisplayLen(t *testing.T) {
	// 按 rune 计数，中文算 1 个字符（列宽由调用方处理）。
	if got := displayLen("餐饮"); got != 2 {
		t.Fatalf(`displayLen("餐饮") = %d, 期望 2`, got)
	}
	if got := displayLen("abc"); got != 3 {
		t.Fatalf(`displayLen("abc") = %d, 期望 3`, got)
	}
	if got := displayLen(""); got != 0 {
		t.Fatalf("空串长度应为 0, 实际 %d", got)
	}
}
