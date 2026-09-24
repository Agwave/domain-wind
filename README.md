# domain-wind · 域名投资资讯观察工具

每天抓取 **DNJournal / Domain Name Wire / NamePros** 公开 RSS，对比近 N 日基线，
筛选出「新冒头 / 暴增」热词（如 agent、jev），生成 Markdown 报告并推送到企业微信。

> **首版定位**：资讯 + 社区热词信号。不接入 NameBio 付费 API / GoDaddy Auctions /
> 微博 / X。成交数据源接口已预留（`fetch.SalesSource`），二期可接。

## 功能特性

- 📡 **每日抓取**：三源公开 RSS（可在 `config.yaml` 增删/开关）
- 🔥 **热词判定**（阈值可配）：
  - 新冒头：今日独立条目提及 ≥ `new_min`，且基线日均 &lt; 0.5
  - 暴增：今日 ≥ `surge_min`，且今日 ≥ 基线日均 × `surge_ratio`
- 🗂️ **报告四段**：热词信号 → 今日资讯 → 社区讨论 → 抓取状态
- 📝 **报告落盘**：`data/reports/YYYY-MM-DD.md`，快照存 `data/YYYY-MM-DD.json`
- 📲 **企业微信推送**：完整报告，超长按行拆多条；无内容默认不打扰；同一天不重复推送

## 快速开始

```bash
# 1. 环境（Go 1.22，通过 g 管理）
source ~/.g/env
g install 1.22.12 && g use 1.22.12

# 2. 构建
go build -o bin/domainwind ./cmd/domainwind

# 3. 首次运行（建立基线，不推送）
./bin/domainwind run --dry-run

# 4. 配置企微后正式跑
./bin/domainwind run
```

## 命令参考

| 命令 | 说明 |
|---|---|
| `domainwind run [--dry-run] [--date YYYY-MM-DD]` | 抓取 → 分析 → 报告 → 推送 |
| `domainwind fetch` | 只抓取入库 |
| `domainwind report [--date] [--dry-run]` | 用已有快照生成报告/推送 |
| `domainwind list` | 列出已有快照 |
| `domainwind check <domains...>` | RDAP+DNS 探测是否已注册 |
| `--data-dir DIR` | 数据目录（默认 `data/`） |
| `--file PATH` | `check`：从文件读域名（每行一个，`#` 注释） |
| `--concurrency N` | `check`：并发数（默认 5） |

### 域名占用粗检

```bash
./bin/domainwind check example.com igaminghub.com getagentic.com
./bin/domainwind check --file candidates.txt --concurrency 8
```

结果三种：`已注册` / `可能可注册` / `未知`。基于公共 RDAP + DNS NS，**不等于**注册商购物车可买保证。

### 热词候选（写入日报第四节）

`run` / `report` 默认会：热词 → 模板生成候选（前后缀含 `ai`）→ 批量粗检 → 报告「四、候选域名粗检」。

```bash
./bin/domainwind report --date 2026-09-24 --dry-run
./bin/domainwind report --skip-candidates   # 跳过候选与粗检
```

模板与开关见 `config.yaml` 的 `candidates` 段。

## 配置

### 企业微信通知

1. 手机/电脑端注册[企业微信](https://work.weixin.qq.com/)（个人可免费注册）
2. 创建一个只有自己的群 → 群设置 → 群机器人 → 添加机器人 → 复制 webhook 地址
3. 写入 `config.local.yaml`（已被 gitignore，不会入库）：

```yaml
notify:
  webhook_url: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=xxx"
```

### 热词阈值与数据源

`config.yaml`（模板，入库）：

```yaml
sources:
  dnjournal:
    enabled: true
    url: "http://www.dnjournal.com/rss.xml"
  dnwire:
    enabled: true
    url: "https://domainnamewire.com/feed/"
  namepros:
    enabled: true
    url: "https://www.namepros.com/external.php?type=rss2"

analyze:
  baseline_days: 7
  new_min: 2
  surge_min: 3
  surge_ratio: 3
  top_k: 15

notify:
  notify_when_quiet: false
```

可选：在 `data/stopwords.txt` 追加停用词（每行一词，`#` 开头为注释）。

## 定时任务

工具本身**不内置定时**，用 crontab 定时触发 `domainwind run`。先构建二进制并建日志目录：

```bash
go build -o bin/domainwind ./cmd/domainwind
mkdir -p logs
```

每日 9:00 运行（路径换成你的项目实际路径）：

```bash
crontab -e
# 每天 9:00
0 9 * * * cd /path/to/domain-wind && ./bin/domainwind run >> logs/cron.log 2>&1
```

### WSL2

1. **cron 服务要常驻**：每次开机后 `sudo service cron start`
2. **WSL 到点必须运行中**，否则不触发

## 数据源

| 源 | URL | 用途 |
|---|---|---|
| DNJournal | `http://www.dnjournal.com/rss.xml` | 行业资讯 / 周报成交摘要 |
| Domain Name Wire | `https://domainnamewire.com/feed/` | 行业资讯 |
| NamePros | `https://www.namepros.com/external.php?type=rss2` | 社区讨论热词 |

RSS 全文仅供个人研究使用；请遵守各站服务条款。

## 常见问题

- **推送超长**：企业微信单条 markdown 上限约 4096 字节，超长按行拆分；完整报告在 `data/reports/`
- **重复推送**：同一天相同内容不会推送第二次（`data/state.json` 记录指纹）
- **首次运行**：无历史对比，只建立基线并推送确认消息
- **抓取失败**：单源失败重试 2 次，仍失败在报告注明，不影响其他源
- **NamePros 403**：NamePros 对部分网络启用 Cloudflare 挑战，直连 RSS 可能失败；报告会标注「抓取失败」，DNJournal / DNWire 仍正常。可暂时在 `config.yaml` 将 `namepros.enabled` 设为 `false`
- **热词噪声**：调高 `new_min` / `surge_min`，或把噪声词加入 `stopwords`

## 许可证

[MIT](LICENSE) © 2026 chenyinbo
