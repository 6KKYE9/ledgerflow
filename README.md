# LedgerFlow

一个用 Go 编写的轻量级个人记账与财务追踪命令行工具。零外部依赖，数据以 JSON 文件本地存储，开箱即用。

## 特性

- **收支记录**：支持收入 / 支出，可带类别与备注
- **本地存储**：数据保存在 `~/.ledgerflow/ledger.json`，无需数据库
- **多维度查询**：按类别、类型、月份、关键字筛选
- **统计报表**：月度汇总、结余、最大支出类别
- **趋势分析**：按月查看收支变化
- **预算管理**：为某月设置预算上限，超支 / 接近上限时提醒
- **CSV 导出**：一键导出全部记录，方便用表格软件分析
- **彩色终端界面**：清晰直观的表格输出

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

### 查看记录

```bash
ledgerflow list                       # 全部，按日期倒序
ledgerflow list -cat 餐饮             # 只看餐饮
ledgerflow list -type expense         # 只看支出
ledgerflow list -month 2026-08        # 看某月
ledgerflow list -q 咖啡               # 关键字搜索
```

### 汇总统计

```bash
ledgerflow summary                    # 全部汇总
ledgerflow summary -month 2026-08     # 某月汇总
```

### 按月趋势

```bash
ledgerflow month
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
ledgerflow edit <id> -amount 40 -cat 餐饮   # 修改金额 / 类别 / 备注
ledgerflow del <id>                          # 删除某条记录
```

记录 ID 在执行 `list` 时可见。

### 导出 CSV

```bash
ledgerflow export -o ledger.csv
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
