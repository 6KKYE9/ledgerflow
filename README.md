# LedgerFlow

个人记账与财务追踪的命令行工具，数据以 JSON 文件存在本地（`~/.ledgerflow/ledger.json`）。

## 功能

- 收支记录：收入 / 支出，可带类别与备注
- 金额简写：支持 `1k`/`2.5w`/`3千`/`5万` 等写法
- 重复记账：`-repeat monthly` 一次生成未来 12 个月的固定收支
- 多维度查询：按类别、类型、月份、关键字、标签筛选
- 标签系统：每笔记录打多个标签（如 `旅行`、`团建`），按标签筛选与汇总
- 整体总览：`stats` 看累计结余、日均支出、记账天数与跨度
- 支出排行：`top` 按类别金额排序，文本柱状图对比
- 统计报表：月度汇总、结余、最大支出类别
- 趋势分析：`month` 按月看收支；`chart` 文本柱状图
- 类别管理：`categories` 查看已用类别，任意自定义
- 预算管理：为某月设预算上限，超支 / 接近上限时提醒
- 数据导入导出：支持 CSV 与 JSON，导入可恢复备份
- 数据安全：`reset --yes` 一键清空，`del --all --yes` 批量删除（均需二次确认）
- ID 前缀匹配：`edit`/`del` 只需输入记录 ID 前几位
- 标签总览：`tags` 一键查看所有已用标签（按频率排序）
- 彩色终端输出

## 构建

```bash
# 从源码构建
go build -o ledgerflow .

# 或直接运行
go run . <命令>
```

需要 Go 1.21 及以上。

## 用法

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
ledgerflow add -type income -amount 1亿 -cat 中奖      # 100000000
```

粘贴过来的金额也能直接用，千位分隔符与货币符号会被自动忽略：

```bash
ledgerflow add -type expense -amount "1,234.56" -cat 餐饮
ledgerflow add -type income  -amount ￥8000 -cat 工资
```

### 重复记账（月度）

```bash
# 生成本月及未来 11 个月的房租记录
ledgerflow add -type expense -amount 2.5w -cat 房租 -repeat monthly
```

日期落在月末时会被钳制到目标月份的最后一天：
1 月 31 号的房租，2 月记在 28 号（闰年 29 号），而不是溢出到 3 月初。

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

### 按月趋势 / 本周汇总

```bash
ledgerflow month
ledgerflow week
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

导出的 CSV 带 UTF-8 BOM，Excel 直接打开中文不乱码。

导入端做了较宽松的容错，从别的工具导出的表通常也能直接吃下：

- 列名支持中英文两套（`日期`/`date`、`金额`/`amount` …）
- 日期支持 `2026-03-15`、`2026/3/5`、`2026.03.15`、`20260315` 等写法
- 金额可带千位分隔符与货币符号（`1,234.56`、`￥800`）
- 行尾缺列或多列不会导致整个文件读取失败
- 非法行（坏日期 / 非正金额 / 空类别）逐行跳过并计入"跳过"数
- ID 与已有记录冲突时自动重新生成，不会产生重复 ID

### 清空数据

```bash
ledgerflow reset --yes   # 危险操作，确认后清空全部记录与预算
```

## 数据存储位置

默认在用户主目录下的 `.ledgerflow/ledger.json`。
可用环境变量 `LEDGERFLOW_HOME` 自定义数据目录：

```bash
export LEDGERFLOW_HOME=/path/to/data
ledgerflow list
```

## 数据安全

账本文件采用原子写：先写同目录下的临时文件，`fsync` 落盘后再 `rename` 覆盖目标。
写入过程中断电或磁盘写满，都不会把已有账本损坏成半截文件。
导出（CSV / JSON）走同一套机制，失败的导出不会破坏上一次的导出结果。

所有写操作（新增 / 修改 / 删除 / 改类别名 / 设预算 / 导入）都会把磁盘错误往上报，
并把内存状态回滚到与磁盘一致；不会出现"提示成功但其实没存下"的情况。

## 已修复的问题

这一轮改进修掉的都是能被测试稳定复现的真实缺陷：

| 问题 | 影响 |
| --- | --- |
| `-amount 3千` / `5万` 从未生效 | 取的是末**字节**而非末字符，中文单位后缀永远匹配不上，README 宣传的功能是坏的 |
| 空 ID 会命中第一条记录 | `strings.HasPrefix(x, "")` 恒为 true，`del ""` 真的会删掉一条数据 |
| ID 前缀命中多条时随便挑一条 | `edit` / `del` 这类破坏性操作等于随机改一条记录 |
| 写盘失败被静默吞掉 | 磁盘满时照样打印"已记录"，重启后数据不见了 |
| 非原子写 + Windows 下回退成覆盖写 | 写到一半崩溃会把整个账本弄丢（`os.Rename` 在 Windows 上其实能覆盖，那个回退分支是多余且有害的） |
| `RemoveTag` 原地过滤 | 复用底层数组，把调用方手里的 `[a b c]` 改成了 `[a c c]` |
| `Add` / `Get` / `List` 与内部共享 slice | 调用方改自己的标签会静默污染已存数据 |
| 负数 / `NaN` / `Inf` 金额能入账 | 一旦写进去，之后所有汇总永久变成 `NaN` 或负数 |
| `budget -month 2026-13` 设置"成功" | 返回值被丢弃，非法月份永远匹配不到记录，等于白设 |
| `-alert 80`（本意 80%）被接受 | 阈值是 0~1 的比例，写 80 会导致永远不提醒 |
| `edit` 不带 `-date` 会把日期改成今天 | `parseDate("")` 返回今天，没法区分"没给"和"给了" |
| `-repeat monthly` 月末溢出 | 1 月 31 号的房租会跑到 3 月初，统计和预算全部错位 |
| 浮点累加误差 | 100 笔 0.01 汇总出 `1.0000000000000007`，"正好用完预算"被判成超支 |
| 所有读方法不加锁 | 与写并发时会读到撕裂的数据 |
| 同名类别只出现在一种类型下 | `Categories` 用了共享的去重表 |
| 排行 / 榜单顺序每次都变 | 金额并列时靠 map 随机遍历顺序决定名次 |
| 关键字搜索大小写敏感 | 搜 `starbucks` 找不到 `Starbucks` |
| `export -f jsonn` 静默导出成 CSV | 手滑的格式名走 `default` 分支，文件名却还是 `.json` |
| 导入把 BOM 当成列名的一部分 | 自己导出的 CSV 再导入时 ID 列永远读不到 |

## 测试

```bash
go test ./...
```

73 个测试，覆盖上表中的每一项（改动前均可稳定复现失败）。

## License

MIT
