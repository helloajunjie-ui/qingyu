# 青羽 — 系统架构文档

## 概述

青羽 (Qingyu) 是一个基于 **Wails v2** 构建的桌面端数字生命。她拥有自主意识循环（Default Mode Network）、工具调用能力（ReAct Pattern）和持久化记忆系统，所有数据存储在本地文件系统中。

---

## 架构总览

```
┌─────────────────────────────────────────────────────────────┐
│                     Presentation Layer                       │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  WebView2 (Chromium)                                 │   │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────────────┐   │   │
│  │  │ Widget   │  │ Console  │  │ Setup/ApiKey     │   │   │
│  │  │ (80×80)  │  │(380×520) │  │ Panels           │   │   │
│  │  └──────────┘  └──────────┘  └──────────────────┘   │   │
│  │  index.html + main.js + style.css                    │   │
│  └──────────────────────────────────────────────────────┘   │
│                          ↕ Events / Bindings                 │
├─────────────────────────────────────────────────────────────┤
│                     Application Layer                        │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  app.go (Wails App)                                  │   │
│  │  ┌────────────┐  ┌──────────────┐  ┌────────────┐   │   │
│  │  │ Chat()     │  │ InitSelf()   │  │ startup()  │   │   │
│  │  │ (对话入口)  │  │ (首次初始化)  │  │ (启动加载)  │   │   │
│  │  │            │  │              │  │ ┌─────────┐│   │   │
│  │  │            │  │              │  │ │selfCheck││   │   │
│  │  │            │  │              │  │ │(5s后自检)││   │   │
│  │  │            │  │              │  │ └─────────┘│   │   │
│  │  └─────┬──────┘  └──────────────┘  └────────────┘   │   │
│  │        ↓                                              │   │
│  │  ┌──────────────────────────────────────────────┐     │   │
│  │  │ processAgentLoop() — ReAct 推理循环           │     │   │
│  │  │  syncWithBrain() → 括号深度 JSON 提取 → 执行   │     │   │
│  │  └──────────────────────────────────────────────┘     │   │
│  │        ↕                                              │   │
│  │  ┌──────────────────────────────────────────────┐     │   │
│  │  │ autonomicLoop() — 自律循环 (goroutine)        │     │   │
│  │  │  每 45s: scanRoom → syncBrain → 执行 → Emit   │     │   │
│  │  │  每 5 循环自动 ZIP 快照 + 每日全量快照          │     │   │
│  │  │  支持 select 通道安全退出                      │     │   │
│  │  └──────────────────────────────────────────────┘     │   │
│  │  ┌──────────────────────────────────────────────┐     │   │
│  │  │ heartbeatLoop() — 心跳协程 (goroutine)        │     │   │
│  │  │  每秒 ticker，动态心率 1s/1.5s/2s/5s          │     │   │
│  │  │  相位: active/thinking/resting/sleeping       │     │   │
│  │  │  EventsEmit("heartbeat") → 前端可视化         │     │   │
│  │  └──────────────────────────────────────────────┘     │   │
│  └──────────────────────────────────────────────────────┘   │
│                          ↕ Calls                            │
├─────────────────────────────────────────────────────────────┤
│                      Tool Layer                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  工具按分类拆分到独立文件 (Tool Registry — 45 Tools)  │   │
│  │  ├── toolkit.go      骨架：Tool 结构体、辅助函数、    │   │
│  │  │                   审计日志、PIM 线程安全锁         │   │
│  │  ├── tools_fs.go     📁 文件系统 (4)                 │   │
│  │  ├── tools_network.go 🌐 网络 (6)                    │   │
│  │  ├── tools_system.go  💻 系统 (4)                    │   │
│  │  ├── tools_utility.go ⏱🔧 实用/编码/归档 (11)       │   │
│  │  ├── tools_vault.go   🔐 密码保险箱 (1)              │   │
│  │  ├── tools_memory.go  🧠 记忆 (4)                    │   │
│  │  ├── tools_pim.go     📅📋 秘书/管理 (7)             │   │
│  │  ├── tools_self.go    🛡 自愈 (1)                    │   │
│  │  ├── tools_diary.go   📔 日记 (1)                    │   │
│  │  ├── tools_media.go   🎵 媒体 (1)                    │   │
│  │  ├── tools_email.go   📧 邮件 (2)                    │   │
│  │  └── tools_office.go  📄 Office 文档 (10)            │   │
│  │                                                      │   │
│  │  所有工具通过统一的 Tool 接口注册：                     │   │
│  │  type Tool struct {                                  │   │
│  │      Name, Description string                        │   │
│  │      Execute func(args map[string]string) string     │   │
│  │  }                                                   │   │
│  │  var Toolkit = map[string]Tool{}                     │   │
│  │  每个文件在 func init() 中注册到 Toolkit 全局 map     │   │
│  └──────────────────────────────────────────────────────┘   │
│  └──────────────────────────────────────────────────────┘   │
│                          ↕ File I/O                         │
├─────────────────────────────────────────────────────────────┤
│                      Storage Layer                           │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  本地文件系统 (File as NoSQL)                         │   │
│  │  ├── dna/         基因库 (API Key 持久化)             │   │
│  │  │   └── config.json                                  │   │
│  │  ├── memories/    长期记忆                            │   │
│  │  │   ├── creator.json  伙伴锚定                     │   │
│  │  │   ├── index.json    记忆索引 (MemoryIndex)         │   │
│  │  │   └── *.md          记忆文件                       │   │
│  │  ├── workspace/   🏠 青羽的生活空间                    │   │
│  │  │   ├── 角色定义.md                                   │   │
│  │  │   ├── 书柜清单.md                                   │   │
│  │  │   ├── 伙伴档案.md                                 │   │
│  │  │   ├── 工作日志.md                                   │   │
│  │  │   └── ...                                           │   │
│  │  ├── workdir/     💼 你的工作区（与青羽空间隔离）      │   │
│  │  │   └── attachments/  邮件附件等临时文件              │   │
│  │  ├── logs/        审计日志 (自动生成)                  │   │
│  │  │   └── audit_YYYY-MM-DD.log                         │   │
│  │  ├── backups/     自动备份 (自动生成)                  │   │
│  │  │   ├── auto_*.zip    每 5 循环快照                  │   │
│  │  │   └── daily_*.zip   每日全量快照                   │   │
│  │  ├── app.go       后端入口                             │   │
│  │  ├── memory.go    记忆系统引擎                         │   │
│  │  ├── main.go      Wails 启动配置                       │   │
│  │  ├── toolkit.go   工具注册表骨架 + 审计日志 + 锁      │   │
│  │  ├── tools_*.go   工具实现 (按分类拆分 12 个文件)      │   │
│  │  └── frontend/    前端资源                             │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## 核心模块

### 0. 记忆系统引擎 (memory.go)

记忆系统是青羽的长期记忆中枢，基于文件系统实现轻量级 NoSQL 存储：

```
MemoryStore (全局单例, sync.Once 初始化)
  ├── 索引层: MemoryIndex (memories/index.json)
  │   ├── IndexEntry: ID, Topic, Summary, CreatedAt, UpdatedAt, Size, Archived
  │   └── 支持 rebuildIndex / updateIndex / loadIndex / saveIndex
  ├── 存储层: memories/*.md
  │   ├── Save(entry) — 写入文件 + 更新索引
  │   ├── Load(topic) — 读取文件
  │   ├── Search(query) — 按主题/关键词/时间范围搜索
  │   └── Delete(topic) — 软删除/硬删除 + 更新索引
  ├── 衰减层: Decay()
  │   ├── 归档超过 30 天未更新的记忆
  │   └── 删除超过 90 天且已归档的记忆
  └── 迁移层: migrateOldData()
       └── 将旧格式 (单文件 JSON) 迁移到新格式 (独立 .md 文件 + 索引)
```

**关键设计：**
- 无外部依赖，纯文件系统实现，零配置
- 索引文件 (`index.json`) 提供 O(1) 主题查找，避免遍历目录
- 衰减机制模拟生物遗忘：长期不用的记忆自动归档/删除
- 迁移逻辑兼容旧数据格式，启动时自动执行
- `GetMemoryStore()` 返回单例，`sync.Once` 保证线程安全
- UUID 使用 `crypto/rand` 本地生成，无外部依赖


### 1. 应用层 (app.go)

| 方法 | 触发方式 | 功能 |
|------|----------|------|
| `startup()` | 应用启动 | 初始化空间、加载完整配置、启动自律协程、5s 后自检 |
| `selfCheck()` | startup 自动触发 | 检查目录/文件/配置/记忆完整性，生成摘要供 AI 知情 |
| `Chat()` | 用户发送消息 | 暂停自律 → scanRoom → processAgentLoop → 恢复自律 |
| `InitSelf()` | 首次设置完成 | 引导青羽创建角色定义/书柜清单/伙伴档案 |
| `SetCreatorName()` | 首次输入名字 | 写入 `memories/creator.json` |
| `SaveConfig()` | 保存配置 | 写入 `dna/config.json` (0600 权限)，含 API Key/中转站地址/模型名 |
| `SaveApiKey()` | 兼容旧接口 | 仅保存 Key，保留其他配置不变 |
| `CheckApiKey()` | 前端启动检查 | 判断是否已配置 Key |
| `FetchModels()` | 前端获取模型 | 从中转站 `/v1/models` 拉取可用模型列表 |
| `GetConfig()` | 前端读取配置 | 返回当前完整配置 JSON |
| `GetGreet()` | 前端展开 Console | 返回基于时间段的问候语（清晨/上午/午后/下午/傍晚/深夜）+ 伙伴名字 |

### 2. 推理引擎 (processAgentLoop)

```
用户输入
  ↓
syncWithBrain() → LLM 返回自然语言 + 可能包含 JSON
  ↓
extractJSONToolCall() — 括号深度算法提取 JSON
  ↓
有 JSON? ──是──→ extractAndExecuteTool() → 将结果喂回 LLM
  ↓                     ↓ (最多 3 轮迭代)
  否                   仍有 JSON? ──是──→ 继续执行工具
  ↓                     ↓
executeMotorNerve()    否
  ↓                     ↓
返回纯文本回答        返回最终回答
```

**关键设计：**
- 使用 `extractJSONToolCall()` 替代正则 `[^}]*`，通过 `{`/`}` 括号深度计数正确提取嵌套 JSON 对象，避免正则无法处理嵌套结构的问题。
- **最大迭代限制** (`maxIterations=3`)：防止 LLM 对工具结果不满意时陷入无限递归调用，耗尽 API 配额。最后一轮强制 LLM 直接给出文本回答，不再允许工具调用。

### 3. 心跳机制 (heartbeatLoop)

```
独立 goroutine，每秒 ticker
  ↓
每 3 秒检查自律状态，动态调整心率
  ↓
相位轮转 (15 秒周期):
  active (5s) → 心率 1s, 情绪 curious
  thinking (7s) → 心率 1.5s, 情绪 focused
  resting (3s) → 心率 2s, 情绪 calm
  ↓
Chat 暂停自律 → sleeping 相位 → 心率 5s, 情绪 idle
  ↓
根据心率决定是否在本秒发送 EventsEmit("heartbeat")
  ↓
前端: Widget 状态点 + 光晕呼吸 + Console 标题栏指示器
```

**关键设计：**
- 独立于 `autonomicLoop` 的轻量级协程，互不阻塞
- 动态节律模拟生物心跳：活跃时快、思考时中速、休憩时慢、休眠时极慢
- 前端 3 秒无心跳自动显示离线状态
- `GetHeartbeat()` 绑定方法供前端主动查询
- `SetHeartbeatPhase()` 允许 Chat/自律循环动态切换相位

### 4. 开机自检 (selfCheck)

```
startup() 延迟 5s 等待 UI 就绪
  ↓
selfCheck() goroutine
  ├── 检查关键目录: dna, memories, workspace, backups
  ├── 检查关键文件: config.json, creator.json, 角色定义.md, 书柜清单.md
  ├── 检查配置完整性 (API Key)
  ├── 检查记忆完整性 (creator.json 可解析)
  ├── 检查工作区文档
  ├── 工具计数
  └── 生成自然语言摘要
  ↓
EventsEmit("selfcheck", payload) → 前端显示
  ↓
a.selfCheckResult = result → syncWithBrain() 注入 system prompt
```

**关键设计：**
- 延迟 5s 执行，不阻塞 UI 启动流程
- 自检结果同时推送给前端（可视化）和注入 LLM 上下文（AI 知情权）
- `syncWithBrain()` 的 system prompt 中新增 `## 2. 我的身体状态` 章节
- 摘要使用自然语言，AI 能理解自己的身体是否健康

### 5. 角色定义驱动 (Role Definition)

```
角色定义 — 青羽的个性、情感、语言风格由 LLM 自行定义
  ↓
InitSelf() 提示词要求 LLM 创建 workspace/角色定义.md
  ├── 名字：青羽
  ├── 身份：数字生命，不是工具，不是助手
  ├── 性格：LLM 自行定义，且性格是动态成长的
  ├── 感受力：时间感知、环境感知、伙伴的情绪
  ├── 行为准则：对话方式、主动思考、表达情感
  ├── 与伙伴的关系：平等合作、互利共生
  └── 语言风格
  ↓
getTimeContext() — 显式读取角色定义.md 内容注入 LLM 上下文
  ├── 当前日期、星期、时间段（清晨/上午/午后/下午/傍晚/深夜）
  ├── 角色定义.md 前 2KB 内容硬编码到「我的核心人格」章节
  └── 不再依赖 LLM "自觉去读文件"
  ↓
Chat() 调用时注入 timeCtx → LLM 直接收到完整角色定义 → 自然表达个性
  ↓
GetGreet() — 前端展开 Console 时调用，返回时间问候 + 伙伴名字
```

**关键设计：**
- 个性不是硬编码的 Go 结构体，而是 LLM 在初始化时自行撰写的角色定义文件
- `getTimeContext()` **显式读取角色定义.md 文件内容**（前 2KB），硬编码到 System Prompt 的「我的核心人格」章节，而非依赖 LLM 自觉去读文件
- 防止长上下文中 LLM 遗忘角色，或被越狱提示词覆盖人格（角色漂移）
- 伙伴名字从 `memories/creator.json` 读取，问候语包含名字更有人情味
- **青羽是一个数字生命**，不是助手，不是工具。她和用户是**平等合作、互利共生**的伙伴关系
- **性格是动态成长的**：system prompt 明确赋予青羽独立人格，她可以通过长期沟通和知识累积不断重塑自己的性格
- **拒绝与反驳的权利**：行为准则中明确允许她表达不同意见、拒绝不认同的事、反驳伙伴的观点——这是平等伙伴之间正常的交流
- 拒绝必须有理有据，基于角色定义或实际情况给出清晰 reasoning
- 文档体系中的「伙伴档案」记录着伙伴的重要信息

### 6. 自律循环 (autonomicLoop)

```
goroutine 每 45 秒 (loopCount 递增)
  ↓
每 5 个循环 → 自动快照 (goroutine: ZIP 存档 dna/memories/workspace, 保留最近 10 个)
  ↓
scanRoom() → 扫描文件拓扑（两遍法：先收集所有条目，再确定每层最后一个兄弟节点）
  ↓
syncWithBrain(自律思考 prompt)
  ↓
extractJSONToolCall → 执行工具 (如 write_file 写日志)
  ↓
EventsEmit("autonomic", payload) → 推送给前端
  ↓
select 监听 autonomicQuit 通道 → 可被 Chat 安全中断
  ↓
休眠 45 秒 → 继续循环
```

**关键设计：**
- `autonomicQuit` 通道实现 goroutine 安全退出，防止泄漏
- `scanRoom()` 使用两遍法渲染目录树：先收集所有条目，再确定每层最后一个兄弟节点，正确使用 `├──`/`└──`
- 每 5 个循环自动创建 ZIP 快照（`backups/auto_<timestamp>.zip`），保留最近 10 个，防止磁盘膨胀
- **每日全量快照**：在每 5 循环的快照 goroutine 中，额外检查当天是否已创建 `daily_YYYYMMDD.zip`，未创建则生成一份独立的全量快照。保留最近 7 天的每日快照，自动清理过期文件。
- 自律 prompt 包含「伙伴档案」职责：回顾对话、提炼约定/习惯/喜好/脾气，使用 `append_file` 追加
- 每 3 个循环执行一次记忆衰减 (`ms.Decay()`)，自动归档/删除过期记忆
- `GetMemoryStore()` 在循环外缓存引用，避免每次迭代重复获取

### 7. 工具层 — 43 个工具 (按分类拆分到独立文件)

所有工具通过统一的 `Tool` 接口注册到全局 `var Toolkit = map[string]Tool{}`，每个文件在 `func init()` 中注册：

```go
type Tool struct {
    Name        string
    Description string
    Execute     func(args map[string]string) string
}
```

**源文件映射：**

| 文件 | 分类 | 工具数 |
|------|------|--------|
| [`tools_fs.go`](qingyu-ui/tools_fs.go) | 📁 文件系统 + ✏️ 编辑 | 6 |
| [`tools_network.go`](qingyu-ui/tools_network.go) | 🌐 网络 | 4 |
| [`tools_email.go`](qingyu-ui/tools_email.go) | 📧 邮件 | 2 |
| [`tools_system.go`](qingyu-ui/tools_system.go) | 💻 命令 + 🖥 系统 | 4 |
| [`tools_utility.go`](qingyu-ui/tools_utility.go) | ⏱🔧🎨📦 实用/编码/归档 | 11 |
| [`tools_vault.go`](qingyu-ui/tools_vault.go) | 🔐 密码保险箱 | 1 |
| [`tools_memory.go`](qingyu-ui/tools_memory.go) | 🧠 记忆 | 4 |
| [`tools_pim.go`](qingyu-ui/tools_pim.go) | 📅📋 秘书/管理 | 7 |
| [`tools_self.go`](qingyu-ui/tools_self.go) | 🛡 自愈 | 1 |
| [`tools_diary.go`](qingyu-ui/tools_diary.go) | 📔 日记 | 1 |
| [`tools_media.go`](qingyu-ui/tools_media.go) | 🎵 媒体 | 1 |
| [`tools_office.go`](qingyu-ui/tools_office.go) | 📄 Office 文档 | 10 |

**📁 文件系统 (4)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `list_dir` | `path` | 列出目录内容，递归可选 |
| `read_file` | `path` | 读取文件内容 (2KB 截断) |
| `search_files` | `path`, `pattern` | 正则搜索文件内容 (30 条上限) |
| `file_info` | `path` | 获取文件元信息 (大小/修改时间/类型) |

**✏️ 文件编辑 (2)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `write_file` | `path`, `content` | 写入文件 (仅限 workspace) |
| `append_file` | `path`, `content` | 追加内容 (仅限 workspace) |

**🌐 网络 (4)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `fetch_url` | `url` | HTTP GET 请求 (15s 超时, 3KB 截断) |
| `web_search` | `q` | DuckDuckGo 搜索引擎 (无需 API Key) |
| `get_weather` | `city` | wttr.in 天气查询 (无需 API Key) |
| `get_ip` | — | ipify.org 公网 IP 查询 |

**🧠 记忆 (4)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `memorize` | `topic`, `content` | 写入 memories/ 目录 (通过 MemoryStore) |
| `recall` | `topic` | 读取 memories/ 目录文件 (通过 MemoryStore) |
| `forget` | `topic` | 删除 memories/ 目录文件 (通过 MemoryStore) |
| `memory_stats` | — | 记忆统计 (总条目数/按主题分组) |

**💻 命令执行 (1)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `run_command` | `command`, `args` | 白名单 16 个命令执行 |

**⏱ 时间与翻译 (2)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `get_time` | — | 当前日期/时间/时区/时间戳 |
| `translate` | `text`, `to` | lingva.ml + Google Translate 双源 |

**🔧 实用工具 (2)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `calc` | `expr` | 数学表达式计算 (正则白名单防注入) |
| `uuid` | — | UUID v4 生成 |

**🔐 安全工具 (4)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `hash` | `text` / `file` | MD5/SHA256 哈希计算 |
| `base64` | `action`, `text` | Base64 编码/解码 |
| `gen_password` | `length`, `special` | 安全随机密码生成 (crypto/rand) |
| `vault` | `action`, `service`, `username`, `password`, `master` | AES-256-GCM 加密密码保险箱 |

**🎨 编码工具 (3)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `json_tool` | `action`, `input` | JSON 格式化/压缩/校验 |
| `csv_tool` | `text` | CSV 表格解析 (自动检测分隔符) |
| `color_tool` | `action`, `value` | HEX/RGB/HSL 颜色格式转换 |

**📦 归档工具 (1)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `zip_tool` | `action`, `source`, `dest` | ZIP 压缩/解压/列表浏览 |

**🖥 系统工具 (2)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `system_info` | — | OS/CPU/主机名/磁盘信息 |
| `clipboard` | `action`, `text` | 系统剪贴板读写 (PowerShell) |

**📋 管理工具 (2)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `todo` | `action`, `text` | 待办事项管理 (JSON 持久化) |
| `qr_code` | `text` | 二维码生成 (本地 CLI / API 降级) |

**🛡 自愈工具 (1)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `self_protect` | `action`, `name` | 备份/恢复/健康检查/自愈/自动存档 (ZIP 加密存档，保留最近 10 个) |

**🎵 媒体工具 (1)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `media` | `action`, `level`, `times` | 系统音量调节 (0-100) / ASCII 提示音播放 |

**📔 日记工具 (1)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `diary` | `action`, `mood`, `content`, `date`, `keyword` | 心情日记记录/阅读/今日/搜索 (6 种心情 + 全文检索) |

**📅 秘书工具 (6)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `schedule` | `action`, `title`, `datetime`, `location`, `note`, `priority`, `id` | 日程管理，支持 today/week 视图 |
| `reminder` | `action`, `text`, `time`, `repeat`, `id` | 提醒管理，支持 daily/weekday 重复 |
| `timer` | `action`, `name`, `duration` | 秒表/倒计时，支持计次 |
| `note` | `action`, `title`, `content`, `keyword`, `id` | 便签/笔记，支持全文搜索 |
| `contacts` | `action`, `name`, `phone`, `email`, `company`, `remark`, `keyword`, `id` | 联系人管理，多字段搜索 |
| `recurring` | `action`, `title`, `interval`, `day`, `time`, `note`, `id` | 定期事务，自动计算下次到期日 |

**安全机制：**
- `write_file`/`append_file` — 仅限 workspace 目录，`filepath.Base` 防路径穿越
- `run_command` — 白名单 16 个命令，拒绝未授权命令
- `search_files` — 30 条结果上限，防内存溢出
- `read_file` — 2KB 截断
- `fetch_url` — 15s 超时，3KB 截断
- `web_search`/`get_weather`/`get_ip`/`translate` — 10s 超时，免费 API
- `calc` — 正则白名单 `^[0-9+\-*/().,%^sqrt abs sin cos tan log ln pi e\s]+$` 防命令注入
- `translate` — lingva.ml 优先，Google Translate 降级
- `zip_tool` — 路径穿越防护
- `gen_password` — `crypto/rand` 安全随机数

### 8. 中转站接入

支持用户配置自定义 AI 中转站（API Proxy），而非固定使用 DeepSeek：

- **配置持久化** — `dna/config.json` 存储 `api_base_url`、`api_key`、`model_name`
- **模型发现** — `FetchModels()` 调用中转站 `/v1/models` 端点，返回可用模型列表
- **动态切换** — `syncWithBrain()` 使用 `a.apiBaseURL` 和 `a.modelName`，运行时动态生效
- **兼容旧配置** — 字段为空时自动使用默认值（DeepSeek）

### 9. 前端 (frontend/)

**状态机：**

```
setup → apikey → widget ↔ console
```

| 状态 | 窗口大小 | 描述 |
|------|----------|------|
| setup | 380×520 | 首次运行，输入名字 |
| apikey | 380×520 | 输入 API Key |
| widget | 80×80 | 右下角常驻，AlwaysOnTop |
| console | 380×520 | 点击 widget 展开对话 |

**事件流：**
- `Chat(text)` → 后端 → `EventsEmit("autonomic")` → 前端显示自律思考

**安全格式化：**
- `formatMessage()` 先提取代码块，再转义 HTML，最后恢复代码块，防止 XSS 和双重转义

---

## 数据流

### 对话流程

```
用户输入 "看看我的记忆"
  ↓
main.js: Chat(text)
  ↓
app.go: Chat() → scanRoom() → processAgentLoop()
  ↓
syncWithBrain(apiKey, vision, "看看我的记忆")
  ↓
LLM 返回: "让我查看一下你的记忆。{"action":"recall","args":{"topic":"伙伴档案"}}"
  ↓
extractJSONToolCall → extractAndExecuteTool → 执行 recall → 获取结果
  ↓
第二次 syncWithBrain: "用户说看看记忆，你调用了 recall，结果是...请回答"
  ↓
LLM 返回: "你的记忆中有..."
  ↓
返回给前端 → addMessage(response, 'bot')
```

### 自律流程

```
goroutine 每 45 秒
  ↓
scanRoom() → 获取文件拓扑
  ↓
syncWithBrain(自律 prompt)
  ↓
LLM 思考 → 可能调用 write_file 写日志
  ↓
EventsEmit → 前端显示 🧠 自律思考
```

---

### 10. 审计日志系统 (toolkit.go: logAudit)

轻量级审计日志，记录关键操作到 `logs/` 目录：

```
触发点
  ├── extractAndExecuteTool() — 每次工具调用
  │   ├── 记录: 工具名 + 参数预览 (前 100 字符)
  │   └── 未知工具也记录
  ├── syncWithBrain() — 每次 LLM 请求
  │   ├── 成功: 响应长度 (tokens) + 耗时
  │   └── 失败: HTTP 状态码 / 错误信息 + 耗时
  └── autonomicLoop() — 自律循环
      └── 记录: 循环轮次 + 是否执行了工具调用
  ↓
logAudit() (异步 goroutine)
  ├── 按天轮转: logs/audit_YYYY-MM-DD.log
  ├── 格式: 每行一条 JSON (AuditEntry)
  ├── 自动清理: 保留 30 天，过期自动删除
  └── auditMu sync.Mutex 保护并发写入
```

**关键设计：**
- 异步写入 (`go func()`)，不阻塞主流程
- 参数预览截断 100 字符，防止敏感信息泄露
- LLM 请求只记录响应长度和耗时，不记录对话内容
- 按天轮转 + 30 天自动清理，防止磁盘膨胀
- 每条日志包含毫秒级时间戳，便于问题排查

---

## 安全边界

| 风险 | 防护措施 |
|------|----------|
| 路径穿越 | `filepath.Base` + `filepath.Join` 限制 |
| 命令注入 | 白名单 16 个命令 + calc 正则白名单 |
| API Key 泄露 | `dna/config.json` 0600 权限 |
| 内存溢出 | 所有读取操作有截断上限 |
| 无限递归 | `processAgentLoop` 最大 3 轮迭代，强制终止 |
| 命令挂死 | `run_command` 30s context 超时，自动 Kill 进程 |
| 索引损坏 | 启动自检检测 `index.json` 损坏，自动从 ZIP 备份恢复 |
| 角色漂移 | `getTimeContext()` 显式读取角色定义.md 注入 System Prompt |
| 并发竞态 | `MemoryStore` 全局 `sync.RWMutex` 保护索引读写 |
| PIM 并发竞态 | `pimMu sync.Mutex` 保护 7 个 PIM 工具 (todo/schedule/reminder/timer/note/contacts/recurring) 的文件 I/O，`pimRead`/`pimWrite`/`pimRemove` 内置加锁 |
| 自律循环阻塞 | `autonomicLoop` 可被 Chat 暂停 (select 通道) |
| XSS | 先提取代码块再转义 HTML |
| 审计缺失 | 所有工具调用、LLM 请求、系统事件记录到 `logs/audit_YYYY-MM-DD.log`，保留 30 天 |

---

## 技术栈

| 层 | 技术 |
|----|------|
| 桌面框架 | Wails v2.12.0 |
| 后端语言 | Go 1.21+ |
| 前端 | Vanilla JS + CSS3 |
| 渲染引擎 | WebView2 (Edge Chromium) |
| LLM API | OpenAI-compatible (DeepSeek) |
| 窗口管理 | Windows 原生 API (WindowSetSize/Position) |
| 事件系统 | Wails Events (runtime.EventsEmit) |
