# AGENTS.md — 本仓库对 AI 助手的工作要求

## 1. 每次改完代码，必须依次完成以下检查（禁止跳过）

```bash
source ~/.g/env                  # go / golangci-lint 由 ~/.g 管理，先加载环境
gofmt -w cmd internal            # 1. 格式化（改完先跑）
go vet ./...                     # 2. 静态检查
golangci-lint run ./...          # 3. lint（配置见 .golangci.yml，必须 0 issues）
go test ./...                    # 4. 测试（命令见第 3 节）
```

- 若 `golangci-lint run ./...` 有输出，必须修复到 **0 issues** 才能收尾
- 若改动引入新测试文件，同样必须通过第 3 节中的全部测试

## 2. Git commit message 格式

首行：`[改动类型](改动核心模块): 细节描述`

**改动类型**（常用）：`feat` `fix` `perf` `chore` `test` `refactor` `docs` `style` `build` `revert`

**核心模块**：`fetch`（抓取）`analyze`（分析）`store`（存储）`report`（报告）`notify`（通知）`config`（配置）`cli`（命令行）`tests`（测试）

**规则**：
- 首行简洁，说清楚"改了什么、为什么"
- 单次改动内容较多时，首行之后空一行，用「- 」短横线分点列出

示例：

```
[feat](notify): 企业微信推送支持超长拆条与基线确认消息

- 按 4000 字节上限按行拆分，footer 追加到最后一条
- 首次运行推送「基线已建立」确认消息
- 去重指纹加入消息类型，避免安静/基线/报告互相误判
```

```
[fix](analyze): 修复同一条目多次出现同一词时重复计数
```

## 3. Go 测试命令（以本仓库现有测试为准）

**一键完整校验**（构建 + 格式化检查 + vet + lint + 全部测试）：

```bash
source ~/.g/env && gofmt -w cmd internal && go build ./... && go vet ./... && golangci-lint run ./... && go test ./...
```

**常用测试命令**：

| 目的 | 命令 |
|---|---|
| 运行全部测试 | `go test ./...` |
| 运行指定包 | `go test ./internal/analyze/` |
| 运行单个用例（带详细输出） | `go test -run TestAnalyzeNewEmerging -v ./internal/analyze/` |
| 绕过缓存强制重跑 | `go test -count=1 ./...` |

**现有测试清单**：

| 包 | 用例 | 覆盖内容 |
|---|---|---|
| `internal/analyze` | `TestExtractWords` | 分词、停用词、同文去重 |
| `internal/analyze` | `TestAnalyzeFirstRun` | 首次运行不产出热词 |
| `internal/analyze` | `TestAnalyzeNewEmerging` | 新冒头判定与证据 |
| `internal/analyze` | `TestAnalyzeSurge` | 暴增判定 |
| `internal/analyze` | `TestSameItemWordOnce` | 同条目只计一次 |
| `internal/fetch` | `TestParseList` / `TestParseSingleEntry` / `TestParseEmpty` / `TestParseAtom` | RSS/Atom 解析 |
| `internal/fetch` | `TestDedupKey` | 去重键 |
| `internal/notify` | `TestBuildMessagesShort` / `TestBuildMessagesSplit` / `TestNewEmptyWebhook` | 拆条与空 webhook |
| `internal/report` | `TestBuildBaseline` / `TestBuildSignals` | 报告结构 |
| `internal/avail` | `TestNormalizeDomain` / `TestDecide` / `TestCheckRDAPWithMock` | 域名规范化、状态判定、RDAP mock |
| `internal/candidates` | `TestGenerateBasic` / `TestGenerateMinCountAndDedupe` / `TestGenerateMaxPerWord` | 热词候选生成 |
