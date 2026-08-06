# LedgerFlow

一个用 Go 编写的轻量级个人记账与财务追踪命令行工具。零外部依赖，数据以 JSON 文件本地存储，开箱即用。

## 特性

- **收支记录**：支持收入 / 支出，可带类别与备注
- **金额简写**：支持 `1k`/`2.5w`/`3千`/`5万` 等写法，少敲键盘
- **重复记账**：`-repeat monthly` 一次生成未来 12 个月的固定收支
- **本地存储**：数据保存在 `~/.ledgerflow/ledger.json`，无需数据库
- **多维度查询**：按类别、类型、月份、关键字、标签筛选
- **标签系统**：为每笔记录打多个标签（如 `旅行`、`团建`），按标签筛选与汇总
- **整体总览**：`stats` 一眼看清累计结余、日均支出、记账天数与跨度
- **支出排行**：`top` 按类别金额排序，文本柱状图直观对比
- **统计报表**：月度汇总、结余、最大支出类别
- **趋势分析**：`month` 按月看收支；`chart` 文本柱状图直观对比
- **类别管理**：`categories` 查看已用类别，任意自定义类别
- **预算管理**：为某月设置预算上限，超支 / 接近上限时提醒
- **数据导入导出**：支持 CSV 与 JSON 两种格式，导入可恢复备份
- **数据安全**：`reset --yes` 一键清空，`del --all --yes` 批量删除（均需二次确认）
- **ID 前缀匹配**：`edit`/`del` 只需输入记录 ID 前几位即可
- **标签总览**：`tags` 一键查看所有已用标签（按频率排序）
- **彩色终端界面**：清晰直观的表格与图表输出

## 安装

```bash
# 从源码构建
go build -o ledgerflow .

# 或直接运行
go run . <命令>
```

> 需要 Go 1.21 及以上版本。

## 使用

### 记录一笔收入

```bash
ledgerflow add -type income -amount 8000 -cat 工资 -note 月薪
```

### 记录一笔支出

```bash
ledgerflow add -type expense -amount 38.5 -cat 餐饮 -note 午饭
```

### 打标签（可重复）

```bash
ledgerflow add -type expense -amount 1200 -cat 旅行 -note 机票 -tag 旅行 -tag 团建
# 也支持单个以 | 或 、 分隔：-tag 旅行|团建
```

### 金额简写

```bash
ledgerflow add -type income -amount 8k -cat 工资      # 8000
ledgerflow add -type expense -amount 2.5w -cat 房租   # 25000
ledgerflow add -type expense -amount 3千 -cat 购物     # 3000
```

### 重复记账（月度）

```bash
# 生成本月及未来 11 个月的房租记录
ledgerflow add -type expense -amount 2.5w -cat 房租 -repeat monthly
```

### 查看记录

```bash
ledgerflow list                       # 全部，按日期倒序
ledgerflow list -cat 餐饮             # 只看餐饮
ledgerflow list -type expense         # 只看支出
ledgerflow list -month 2026-08        # 看某月
ledgerflow list -q 咖啡               # 关键字搜索
ledgerflow list -tag 旅行             # 按标签筛选
```

### 整体总览与支出排行

```bash
ledgerflow stats                      # 累计结余、日均支出、记账天数等
ledgerflow top                        # 支出类别金额排行（默认前 5）
ledgerflow top -n 10                  # 显示前 10 个类别
ledgerflow top -month 2026-08         # 限定某月
```

### 汇总统计

```bash
ledgerflow summary                    # 全部汇总
ledgerflow summary -month 2026-08     # 某月汇总
ledgerflow summary -tag 旅行          # 按标签汇总
```

### 按月趋势

```bash
ledgerflow month
```

### 收支柱状图

```bash
ledgerflow chart
```

### 查看类别

```bash
ledgerflow categories
```

### 预算管理

```bash
# 设置 2026-08 预算上限 3000，使用到 80% 时提醒
ledgerflow budget -month 2026-08 -limit 3000 -alert 0.8

# 查看预算执行情况
ledgerflow budget -month 2026-08
```

添加支出时若接近或超出预算，会自动给出提示。

### 修改与删除

```bash
ledgerflow edit <id> -amount 40 -cat 餐饮          # 修改金额 / 类别 / 备注
ledgerflow edit <id> -tag 旅行 -tag 团建           # 整体覆盖标签
ledgerflow del <id>                                # 删除某条记录
ledgerflow del --all --yes                         # 批量删除全部记录
```

记录 ID 在执行 `list` 时可见。

### 导出与导入 CSV / JSON

```bash
ledgerflow export -o ledger.csv          # 默认 CSV（含标签列）
ledgerflow export -o ledger.json -f json # JSON 格式
ledgerflow import -o ledger.csv          # 从导出的 CSV 导入恢复
```

### 清空数据

```bash
ledgerflow reset --yes   # 危险操作，确认后清空全部记录与预算
```

## 数据存储位置

默认保存在用户主目录下的 `.ledgerflow/ledger.json`。
可通过环境变量 `LEDGERFLOW_HOME` 自定义数据目录：

```bash
export LEDGERFLOW_HOME=/path/to/data
ledgerflow list
```

## 项目结构

```
ledgerflow/
├── main.go                 # 命令行入口与命令分发
├── go.mod
├── internal/
│   ├── store/              # 数据模型、持久化、筛选、CSV 导出
│   ├── report/             # 汇总、按月聚合、预算计算
│   └── ui/                 # 终端彩色表格与提示输出
└── README.md
```

## 测试

```bash
go test ./...
```

## License

MIT


最后更新：2026-08-06
