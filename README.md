# LedgerFlow

我自己是个记账半途而废的人——不是不想记，是每次打开 App 都被那一堆分类和按钮劝退。所以我写了个能在终端三秒钟记完一笔的东西，纯键盘、不碰鼠标、数据就在本地一个 JSON 里。

用了一阵子，发现它真能让我坚持下来，就整理出来给你用。

## 它解决了我哪些痛点

- **懒得输数字**：`1k`、`2.5w`、`3千`、`5万` 甚至 `1亿` 都能直接写，贴过来的 `￥8,000`、`1,234.56` 也能认
- **固定开销不想月月记**：`-repeat monthly` 一次生成未来 12 个月的房租/工资，不用操心
- **想知道钱花哪了**：`top` 按类别排支出，`chart` 画文本柱状图，`stats` 看结余和日均
- **怕超支**：给某月设个预算，接近上限会自动提醒你收手
- **怕乱**：每笔能打多个标签（`旅行`、`团建`），随时按标签汇总
- **怕丢 / 怕错**：原子写盘（写到一半断电也不会把账本弄坏），导入导出容错很松，从别的工具导的表基本能直接吃

## 装上就能跑

```bash
go build -o ledgerflow .
# 或者
go run . <命令>
```

需要 Go 1.21 往上。

## 怎么用

### 记一笔收入 / 支出

```bash
ledgerflow add -type income -amount 8000 -cat 工资 -note 月薪
ledgerflow add -type expense -amount 38.5 -cat 餐饮 -note 午饭
```

### 金额怎么写都行

```bash
ledgerflow add -type income -amount 8k -cat 工资      # 8000
ledgerflow add -type expense -amount 2.5w -cat 房租   # 25000
ledgerflow add -type expense -amount 3千 -cat 购物     # 3000
ledgerflow add -type income -amount 1亿 -cat 中奖      # 100000000
```

贴过来的金额也能直接用，千位分隔符和货币符号会自动忽略：

```bash
ledgerflow add -type expense -amount "1,234.56" -cat 餐饮
ledgerflow add -type income  -amount ￥8000 -cat 工资
```

### 固定开销一次记一年

```bash
# 生成本月及未来 11 个月的房租
ledgerflow add -type expense -amount 2.5w -cat 房租 -repeat monthly
```

月末的坑它替你想了：1 月 31 号的房租，2 月会落在 28 号（闰年 29 号），不会溢出到 3 月把统计搞乱。

### 打标签

```bash
ledgerflow add -type expense -amount 1200 -cat 旅行 -note 机票 -tag 旅行 -tag 团建
# 也支持 -tag 旅行|团建
```

### 翻账本

```bash
ledgerflow list                       # 全部，按日期倒序
ledgerflow list -cat 餐饮             # 只看餐饮
ledgerflow list -type expense         # 只看支出
ledgerflow list -month 2026-08        # 看某月
ledgerflow list -q 咖啡               # 关键字搜
ledgerflow list -tag 旅行             # 按标签筛
```

### 总览 / 排行 / 图表

```bash
ledgerflow stats                      # 结余、日均支出、记账天数
ledgerflow top                        # 支出类别金额排行（前 5）
ledgerflow top -n 10                  # 前 10
ledgerflow summary -month 2026-08     # 某月汇总
ledgerflow month                      # 按月趋势
ledgerflow chart                      # 收支柱状图
```

### 预算管理

```bash
# 设 2026-08 预算 3000，用到 80% 提醒
ledgerflow budget -month 2026-08 -limit 3000 -alert 0.8
ledgerflow budget -month 2026-08      # 看执行情况
```

加支出时接近或超预算会自动提示。

### 改 / 删 / 清空

```bash
ledgerflow edit <id> -amount 40 -cat 餐饮
ledgerflow del <id>
ledgerflow del --all --yes             # 批量删（二次确认）
ledgerflow reset --yes                 # 清空全部（危险，确认后执行）
```

记录 ID 跑 `list` 时能看到。ID 不用输全，前几位能唯一识别就行。

### 导入导出

```bash
ledgerflow export -o ledger.csv          # 默认 CSV（带标签列）
ledgerflow export -o ledger.json -f json # JSON
ledgerflow import -o ledger.csv          # 导入恢复
```

导出的 CSV 带 UTF-8 BOM，Excel 直接开中文不乱码。导入端很宽容：中英文列名都认、日期写法随便、`1,234.56`/`￥800` 都吃、缺列多列不会整文件挂掉、坏行会跳过并数出来。

## 数据存哪

默认 `~/.ledgerflow/ledger.json`。换地方：

```bash
export LEDGERFLOW_HOME=/path/to/data
ledgerflow list
```

## 你的数据不会丢

写盘用的是原子写：先写临时文件、`fsync` 落盘再 `rename` 覆盖。写到一半断电或磁盘写满，老账本还是好的。所有写操作（增改删、改类别、设预算、导入）出错都会上报并回滚内存，不会出现"提示成功其实没存下"。

## 这一版修掉的坑

都是能被测试稳定复现的真实问题：

| 问题 | 影响 |
| --- | --- |
| `3千` / `5万` 从未生效 | 取的是末字节不是末字符，中文单位永远匹配不上，README 写的功能是坏的 |
| 空 ID 会命中第一条记录 | `del ""` 真能删掉一条数据 |
| ID 前缀命中多条时随便挑 | `edit`/`del` 变成随机改一条 |
| 写盘失败被静默吞 | 磁盘满照样说"已记录"，重启数据没了 |
| 非原子写 + Windows 回退覆盖写 | 写一半崩溃整本账丢 |
| 负数 / `NaN` / `Inf` 能入账 | 进去之后所有汇总永久变 `NaN` 或负数 |
| 非法月份 `2026-13` 设置"成功" | 返回值被丢，永远匹配不到 |
| `-alert 80` 被接受 | 阈值要 0~1，写 80 永远不提醒 |
| `-repeat monthly` 月末溢出 | 1.31 房租跑到 3 月初，统计全错位 |
| 浮点累加误差 | 100 笔 0.01 汇总出 `1.0000000000000007`，正好用完预算被判超支 |
| 读方法不加锁 | 并发读到撕裂数据 |
| 关键字搜索大小写敏感 | 搜 `starbucks` 找不到 `Starbucks` |

## 跑测试

```bash
go test ./...
```

73 个测试，上表每一项改之前都能稳定复现失败。

## License

MIT
