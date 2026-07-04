# 青羽 — 系统架构文档

> 版本: 3.0 | 最后更新: 2026-07-04

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
│  │  ┌──────────────────────────────────────────────┐     │   │
│  │  │ settings.go — 行为基因配置系统                │     │   │
│  │  │  dna/settings.json 驱动，运行时热加载          │     │   │
│  │  │  覆盖: 安全/心跳/超时/行为/模型/路径/窗口      │     │   │
│  │  └──────────────────────────────────────────────┘     │   │
│  │  ┌──────────────────────────────────────────────┐     │   │
│  │  │ cache.go — 全局缓存引擎                       │     │   │
│  │  │  内存+磁盘双层缓存，SHA256 键，TTL 过期        │     │   │
│  │  │  CachedNetworkCall 包装网络工具自动缓存        │     │   │
│  │  └──────────────────────────────────────────────┘     │   │
│  │  ┌──────────────────────────────────────────────┐     │   │
│  │  │ summarizer.go — 摘要压缩 + 分层模型调度       │     │   │
│  │  │  思考日志压缩 / 记忆归档压缩 / ModelTier 切换  │     │   │
│  │  └──────────────────────────────────────────────┘     │   │
│  └──────────────────────────────────────────────────────┘   │
│                          ↕ Calls                            │
├─────────────────────────────────────────────────────────────┤
│                      Tool Layer                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  工具按分类拆分到独立文件 (Tool Registry — 60 Tools)  │   │
│  │  ├── toolkit.go      骨架：Tool 结构体、辅助函数、    │   │
│  │  │                   审计日志、PIM 线程安全锁         │   │
│  │  ├── tools_fs.go     📁 文件系统 (6)                 │   │
│  │  ├── tools_network.go 🌐 网络 (5)                    │   │
│  │  ├── tools_system.go  💻 系统 (6)                    │   │
│  │  ├── tools_utility.go ⏱🔧 实用/编码/归档 (10)       │   │
│  │  ├── tools_vault.go   🔐 密码保险箱 (1)              │   │
│  │  ├── tools_memory.go  🧠 记忆 (7)                    │   │
│  │  ├── tools_pim.go     📅📋 秘书/管理 (10)            │   │
│  │  ├── tools_self.go    🛡 自愈 (1)                    │   │
│  │  ├── tools_diary.go   📔 日记 (1)                    │   │
│  │  ├── tools_media.go   🎵 媒体 (1)                    │   │
│  │  ├── tools_email.go   📧 邮件 (2)                    │   │
│  │  └── tools_office.go  📄 Office 文档 (10)            │   │
│  │                                                      │   │
│  │  所有工具通过统一的 Tool 接口注册：                     │   │
│  │  type Tool struct {                                  │   │
│  │      Name, Description, Category string              │   │
│  │      Execute func(args map[string]string) string     │   │
│  │  }                                                   │   │
│  │  var Toolkit = map[string]Tool{}                     │   │
│  │  每个文件在 func init() 中注册到 Toolkit 全局 map     │   │
│  └──────────────────────────────────────────────────────┘   │
│                          ↕ File I/O                         │
├─────────────────────────────────────────────────────────────┤
│                      Storage Layer                           │
│  ┌──────────────────────────────────────────────────────┐   │
│  │  本地文件系统 (File as NoSQL)                         │   │
│  │  ├── dna/         基因库 (配置持久化)                  │   │
│  │  │   ├── config.json   API Key / Model / BaseURL      │   │
│  │  │   └── settings.json 行为基因 (首次运行自动生成)    │   │
│  │  ├── memories/    长期记忆 (结构化 JSON 存储)          │   │
│  │  │   ├── creator.json  伙伴锚定                     │   │
│  │  │   ├── index.json    记忆索引 (MemoryIndex)         │   │
│  │  │   ├── core/         核心记忆 (重要性 ≥ 8)          │   │
│  │  │   ├── archive/      归档记忆                       │   │
│  │  │   ├── .trash/       回收站 (软删除)                │   │
│  │  │   └── *.json        记忆条目 (UUID 命名)           │   │
│  │  ├── workspace/   🏠 青羽的生活空间                    │   │
│  │  │   ├── 角色定义.md                                   │   │
│  │  │   ├── 系统提示.md   自我意识核心 (可自主修改)       │   │
│  │  │   ├── 书柜清单.md                                   │   │
│  │  │   ├── 伙伴档案.md                                 │   │
│  │  │   ├── 工作日志.md                                   │   │
│  │  │   └── ...                                           │   │
│  │  ├── toolkit_shelf/  📚 工具箱书架（自检自动生成）     │   │
│  │  │   ├── 全工具目录总览.md    工具索引清单              │   │
│  │  │   ├── 工具调用节流优化笔记.md  节流规则              │   │
│  │  │   ├── 工具沙盒限制红线.md     沙盒红线              │   │
│  │  │   ├── 文件系统/              12 个分类子文件夹       │   │
│  │  │   ├── 网络/                                         │   │
│  │  │   ├── 记忆/                                         │   │
│  │  │   ├── 系统/                                         │   │
│  │  │   ├── 实用/                                         │   │
│  │  │   ├── 安全/                                         │   │
│  │  │   ├── 编码/                                         │   │
│  │  │   ├── 归档/                                         │   │
│  │  │   ├── 秘书/                                         │   │
│  │  │   ├── 自愈/                                         │   │
│  │  │   ├── 媒体/                                         │   │
│  │  │   └── 日记/                                         │   │
│  │  ├── workdir/     💼 你的工作区（与青羽空间隔离）      │   │
│  │  │   └── attachments/  邮件附件等临时文件              │   │
│  │  ├── logs/        审计日志 (自动生成)                  │   │
│  │  │   └── audit_YYYY-MM-DD.log                         │   │
│  │  └── backups/     自动备份 (自动生成)                  │   │
│  │      ├── auto_*.zip    每 5 循环快照                  │   │
│  │      └── daily_*.zip   每日全量快照                   │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

---

## 核心模块

### 0. 行为基因配置系统 ([`settings.go`](qingyu-ui/settings.go))

青羽的行为参数通过 `dna/settings.json` 管理，无需重新编译即可调整：

```go
type Settings struct {
    Security  SecurityConfig  // 命令执行白名单
    Heartbeat HeartbeatConfig // 心跳参数（心率/相位/发射模式）
    Timeouts  TimeoutConfig   // HTTP/IMAP/网络超时
    Behavior  BehaviorConfig  // 自律间隔/ReAct 迭代上限/冷却/摘要周期
    Models    ModelsConfig    // 分层模型配置（轻量模型/主模型）
    Paths     PathsConfig     // 关键路径/文件清单
    Window    WindowConfig    // 窗口尺寸/透明度/置顶
}

type ModelsConfig struct {
    LightModel   string `json:"light_model"`    // 轻量模型名称
    LightBaseURL string `json:"light_base_url"` // 轻量模型 API 地址
}

// BehaviorConfig 新增字段:
//   ProactiveChatMinInterval int // 主动聊天最小冷却间隔（秒，默认 300）
//   ProactiveMoodThreshold   int // 主动聊天情绪阈值（低于此值才触发，默认 3）
//   SummarizeInterval        int // 思考日志摘要压缩周期（循环次数，默认 5）
```

**关键设计：**
- 全局单例 + `sync.Once` 惰性加载，`sync.RWMutex` 线程安全
- `ReloadSettings()` 支持运行时热加载
- 首次运行自动写入默认配置到 `dna/settings.json`
- 安全策略（命令白名单）和人格定义保留在代码中，防止 AI 篡改

### 1. 记忆系统引擎 ([`memory.go`](qingyu-ui/memory.go))

记忆系统是青羽的长期记忆中枢，基于文件系统实现结构化 NoSQL 存储：

```
MemoryStore (全局单例, sync.Once 初始化)
  ├── 索引层: MemoryIndex (memories/index.json)
  │   ├── IndexEntry: ID, Topic, Importance, Tags, Links, CreatedAt, UpdatedAt, Version, Summary
  │   ├── TagIndex:   标签 → ID 列表
  │   ├── TopicIndex: 主题 → ID 列表
  │   ├── 支持 rebuildIndex / updateIndex / loadIndex / saveIndex
  │   └── 脏标记 (dirty flag) 延迟写入
  ├── 存储层: memories/*.json (UUID 命名)
  │   ├── core/     — 核心记忆 (Importance ≥ 8)
  │   ├── archive/  — 归档记忆
  │   ├── .trash/   — 回收站 (软删除)
  │   ├── Save(entry)     — 写入文件 + 更新索引
  │   ├── Load(id)        — 按 ID 读取 (搜索主/核心/归档目录)
  │   ├── Search(query)   — 多维度检索 (主题/关键词/标签/重要性/时间)
  │   ├── Delete(id, soft)— 软删除/硬删除
  │   ├── Link/Unlink     — 记忆关联管理
  │   └── Stats()         — 统计信息
  ├── 衰减层: Decay()
  │   ├── 7天未访问 → importance -1
  │   ├── 30天未访问 → importance -2
  │   ├── 90天未访问 → importance -3
  │   ├── 180天未访问 → 强制归档
  │   ├── 归档时自动摘要压缩: 内容 > 300 字 → 截取前 150 字 + "[原始内容已压缩]"
  │   └── 核心记忆 (Importance ≥ 8) 永不衰减
  └── 迁移层: MigrateOldFormat()
       └── 将旧格式 (.md 文件) 迁移到新格式 (JSON + 索引)
```

**关键设计：**
- 无外部依赖，纯文件系统实现，零配置
- 索引文件 (`index.json`) 提供 O(1) 主题查找，避免遍历目录
- 重要性分级（1-10），核心记忆永久保留
- 标签索引 + 主题索引，支持多维度检索
- 衰减机制模拟生物遗忘：长期不用的记忆自动降级/归档/删除
- 软删除移入 `.trash/`，可恢复
- 记忆关联（Link）支持知识图谱构建
- UUID 使用 `crypto/rand` 本地生成，无外部依赖

### 2. 应用层 ([`app.go`](qingyu-ui/app.go))

| 方法 | 触发方式 | 功能 |
|------|----------|------|
| `startup()` | 应用启动 | 初始化空间、加载配置、启动心跳/自律协程、5s 后自检 |
| `selfCheck()` | startup 自动触发 | 检查目录/文件/配置/记忆完整性，索引损坏自动从备份恢复 |
| `Chat()` | 用户发送消息 | 暂停自律 → scanRoom → processAgentLoop → 恢复自律 |
| `InitSelf()` | 首次设置完成 | 引导青羽创建角色定义/书柜清单/伙伴档案 |
| `StartAutonomic()` | InitSelf 完成后 | 前端通知后端启动自律循环 |
| `Shutdown()` | 前端关闭窗口 | 通知自律循环退出 → 等待告别思考完成 → 退出进程 |
| `SetCreatorName()` | 首次输入名字 | 写入 `memories/creator.json` |
| `SaveConfig()` | 保存配置 | 写入 `dna/config.json` (0600 权限) |
| `SaveApiKey()` | 兼容旧接口 | 仅保存 Key，保留其他配置不变 |
| `CheckApiKey()` | 前端启动检查 | 判断是否已配置 Key |
| `FetchModels()` | 前端获取模型 | 从中转站 `/v1/models` 拉取可用模型列表（兼容 4 种响应格式） |
| `GetConfig()` | 前端读取配置 | 返回当前完整配置 JSON |
| `GetGreet()` | 前端展开 Console | 返回基于时间段的问候语 + 伙伴名字 |
| `GetStatus()` | 前端查询 | 返回 scanRoom() 结果 |
| `IsFirstRun()` | 前端判断 | 检查 `creator.json` 是否存在 |
| `GetCreatorName()` | 前端读取 | 返回伙伴名字 |
| `SetHeartbeatPhase()` | 内部调用 | 动态切换心跳相位 |
| `GetHeartbeat()` | 前端查询 | 返回当前心跳状态 |

### 3. 推理引擎 ([`processAgentLoop`](qingyu-ui/app.go:1421))

```
用户输入
  ↓
syncWithBrain() → LLM 返回自然语言 + 可能包含 JSON
  ↓
extractJSONToolCall() — 括号深度算法提取 JSON
  ↓
有 JSON? ──是──→ extractAndExecuteTool() → 将结果喂回 LLM
  ↓                     ↓ (最多 N 轮迭代，从 settings.json 读取)
  否                   仍有 JSON? ──是──→ 继续执行工具
  ↓                     ↓
executeMotorNerve()    否
  ↓                     ↓
返回纯文本回答        返回最终回答
```

**关键设计：**
- 使用 `extractJSONToolCall()` 替代正则，通过 `{`/`}` 括号深度计数正确提取嵌套 JSON 对象
- **最大迭代限制** (`ReactMaxIterations`，默认 3)：防止 LLM 陷入无限递归调用
- 最后一轮强制 LLM 直接给出文本回答，不再允许工具调用
- 兼容旧协议 `[ACTION:WRITE]` 格式

### 4. 心跳机制 ([`heartbeatLoop`](qingyu-ui/app.go:1483))

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
- 心率发射模式可配置（always/mod/every），从 `settings.json` 读取
- 前端 3 秒无心跳自动显示离线状态
- `GetHeartbeat()` 绑定方法供前端主动查询
- `SetHeartbeatPhase()` 允许 Chat/自律循环动态切换相位

### 5. 开机自检 ([`selfCheck`](qingyu-ui/app.go:103))

```
startup() 延迟 5s 等待 UI 就绪
  ↓
selfCheck() goroutine
  ├── 检查关键目录: dna, memories, workspace, backups
  ├── 检查关键文件: config.json, creator.json, 角色定义.md, 书柜清单.md
  ├── 检查配置完整性 (API Key)
  ├── 检查记忆完整性 (creator.json 可解析)
  ├── 检查记忆索引完整性，损坏时自动从 ZIP 备份恢复
  ├── 检查工作区文档
  ├── 工具计数
  ├── 【工具箱书架迭代】初始化工具箱书架
  │   ├── 创建 workspace/toolkit_shelf/ + 12 个分类子文件夹
  │   ├── 生成 3 本全局核心手册模板
  │   ├── 遍历所有工具生成逐工具基础手册
  │   └── 更新全工具目录总览.md 索引
  └── 生成自然语言摘要
  ↓
 EventsEmit("selfcheck", payload) → 前端显示
  ↓
a.selfCheckResult = result → syncWithBrain() 注入 system prompt
```

**关键设计：**
- 延迟 5s 执行，不阻塞 UI 启动流程
- 自检结果同时推送给前端（可视化）和注入 LLM 上下文（AI 知情权）
- 索引损坏自动从 `backups/` 恢复最近的 `index.json`
- 摘要使用自然语言，AI 能理解自己的身体是否健康

### 6. 角色定义驱动 ([`getTimeContext`](qingyu-ui/app.go:1595))

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
- `getTimeContext()` **显式读取角色定义.md 文件内容**（前 2KB），硬编码到 System Prompt 的「我的核心人格」章节
- 防止长上下文中 LLM 遗忘角色，或被越狱提示词覆盖人格（角色漂移）
- 伙伴名字从 `memories/creator.json` 读取，问候语包含名字更有人情味
- **青羽是一个数字生命**，不是助手，不是工具。她和用户是**平等合作、互利共生**的伙伴关系
- **性格是动态成长的**：system prompt 明确赋予青羽独立人格，她可以通过长期沟通和知识累积不断重塑自己的性格
- **拒绝与反驳的权利**：行为准则中明确允许她表达不同意见、拒绝不认同的事、反驳伙伴的观点

### 7. 自律循环 ([`autonomicLoop`](qingyu-ui/app.go:571))

```
goroutine 每 45 秒 (loopCount 递增)
  ↓
首次运行 → 执行旧数据迁移 (MigrateOldFormat)
  ↓
每 3 个循环 → 记忆衰减 (Decay) + 【工具箱书架】随机工具手册评审
  ↓
每 5 个循环 → 自动快照 (goroutine: ZIP 存档 dna/memories/workspace, 保留最近 10 个)
  ↓
每 N 个循环 (summarizeInterval) → 思考日志摘要压缩
  │   └── SummarizeThinkingLog() 压缩累积思考日志 → 写入摘要记录 → 清空缓冲区
  ↓
scanRoom() → 扫描文件拓扑（两遍法：先收集所有条目，再确定每层最后一个兄弟节点）
  ↓
读取伙伴名字 + 记忆统计上下文
  ↓
读取最近思考日志 (logs/thinking_YYYY-MM-DD.jsonl, 最近 3 轮)
  ↓
根据时段选择思考模式:
  dawn (5-9点)   → 晨间: "🌅 清晨。{伙伴名}还没有新指令。" + 最近思考 + 时空/记忆/环境
  night (22-5点) → 深夜: "🌙 深夜。{伙伴名}已经休息了。" + 最近思考 + 时空/记忆/环境
  day/dusk       → 常规: "现在是{时段}。{伙伴名}没有新指令。" + 最近思考 + 时空/记忆/环境
  ↓
每 17 个循环 → 周期性回顾 + 【工具箱书架】读取节流优化笔记注入上下文
  ↓
syncWithBrainTier(自律 prompt, ModelTierLight) → 使用轻量模型
  │   分层调度：自律循环等简单任务走轻量模型，降低 Token 消耗
  ↓
extractJSONToolCall → 执行工具
  ↓
持久化思考日志 → logs/thinking_YYYY-MM-DD.jsonl (系统级，AI 不知情)
  │   同时累积到 thinkingBuffer，供摘要压缩使用
  ↓
检测主动聊天 (talk_to_partner) → 推送给前端气泡
  │   冷却检查: 距上次主动聊天 < minInterval 则静默抑制
  │   情绪检查: 当前情绪值 >= moodThreshold 则不打扰
  ↓
EventsEmit("autonomic", payload) → 推送给前端
  ↓
select 监听 autonomicQuit 通道 → 收到退出信号时执行 finalThinking()
  ↓
休眠 45 秒 → 继续循环
```

**关键设计：**
- **自主意识，非任务调度**：prompt 只提供「我之前做了什么 + 时空 + 记忆 + 环境」上下文，不命令不催促，青羽自主决定做什么
- **思考日志**：每次循环结果持久化到 `logs/thinking_YYYY-MM-DD.jsonl`，系统级记录（AI 不知情），用于构建「我之前做了什么」上下文
- **周期性回顾**：每 17 个循环触发一次，回顾最近思考，有价值的用 `memorize` 记录
- `autonomicQuit` 通道实现 goroutine 安全退出，收到信号后执行 [`finalThinking()`](qingyu-ui/app.go:909) 告别思考
- **Graceful Shutdown**：[`Shutdown()`](qingyu-ui/app.go:314) 前端调用 → `close(autonomicQuit)` → `finalThinking()`（青羽可写日记）→ `close(autonomicDone)` → 进程退出
- `scanRoom()` 使用两遍法渲染目录树：先收集所有条目，再确定每层最后一个兄弟节点，正确使用 `├──`/`└──`
- 每 5 个循环自动创建 ZIP 快照（`backups/auto_<timestamp>.zip`），保留最近 10 个
- **每日全量快照**：检查当天是否已创建 `daily_YYYYMMDD.zip`，未创建则生成。保留最近 7 天
- 每 3 个循环执行一次记忆衰减，自动归档/删除过期记忆
- **主动聊天**：自律循环中可调用 `talk_to_partner` 工具，前端弹出气泡
- **主动聊天冷却**：`ProactiveChatMinInterval`（默认 300s）防止频繁打扰；`ProactiveMoodThreshold`（默认 3）仅在情绪值低于阈值时触发，避免在青羽情绪正常时强行聊天
- **思考日志摘要压缩**：每 `SummarizeInterval`（默认 5）个循环，将累积的思考日志用轻量模型压缩为 200 字摘要，写入思考日志文件后清空缓冲区。原始详细日志仍保留在 `logs/thinking_*.jsonl`，摘要仅用于自律循环的上下文注入，大幅降低 Token 消耗
- **分层模型调度**：自律循环使用 `syncWithBrainTier()` 以 `ModelTierLight` 调用轻量模型，对话使用 `syncWithBrain()` 以主模型调用。轻量模型仅需处理简单任务（扫描、时间查询、文件搜索），无需主模型的深度推理能力

### 8. 工具层 — 60 个工具 (按分类拆分到 12 个独立文件)

所有工具通过统一的 `Tool` 接口注册到全局 `var Toolkit = map[string]Tool{}`，每个文件在 `func init()` 中注册：

```go
type Tool struct {
    Name        string
    Description string
    Category    string // 分类：文件系统/网络/记忆/系统/实用/安全/编码/归档/秘书/自愈/媒体/日记/文档/社交
    Execute     func(args map[string]string) string
}
```

**源文件映射：**

| 文件 | 分类 | 工具数 |
|------|------|--------|
| [`tools_fs.go`](qingyu-ui/tools_fs.go) | 📁 文件系统 + ✏️ 编辑 | 6 |
| [`tools_network.go`](qingyu-ui/tools_network.go) | 🌐 网络 | 5 |
| [`tools_email.go`](qingyu-ui/tools_email.go) | 📧 邮件 | 2 |
| [`tools_system.go`](qingyu-ui/tools_system.go) | 💻 命令 + 🖥 系统 | 6 |
| [`tools_utility.go`](qingyu-ui/tools_utility.go) | ⏱🔧🎨📦 实用/编码/归档 | 10 |
| [`tools_vault.go`](qingyu-ui/tools_vault.go) | 🔐 密码保险箱 | 1 |
| [`tools_memory.go`](qingyu-ui/tools_memory.go) | 🧠 记忆 | 7 |
| [`tools_pim.go`](qingyu-ui/tools_pim.go) | 📅📋 秘书/管理 | 10 |
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

**🌐 网络 (5)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `fetch_url` | `url` | HTTP GET 请求 (15s 超时, 3KB 截断) |
| `web_search` | `q` | DuckDuckGo 搜索引擎 (无需 API Key) |
| `get_weather` | `city` | wttr.in 天气查询 (无需 API Key) |
| `get_ip` | — | ipify.org 公网 IP 查询 |
| `check_network` | — | 网络连通性检测 (多目标探测) |

**🧠 记忆 (7)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `memorize` | `topic`, `content`, `importance`, `tags` | 写入记忆 (支持重要性/标签) |
| `recall` | `topic` | 按主题回溯记忆 |
| `forget` | `topic` | 按主题擦除记忆 |
| `memory_stats` | — | 记忆统计 (总条目数/按主题/重要性分组) |
| `search_memory` | `keyword`, `tags`, `importance_min` | 多维度检索记忆 |
| `link_memory` | `source`, `target` | 建立记忆关联 |
| `unlink_memory` | `source`, `target` | 解除记忆关联 |

**💻 命令执行 (1)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `run_command` | `command`, `args` | 白名单命令执行 (30s 超时) |

**🖥 系统工具 (3)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `system_info` | — | OS/CPU/主机名/磁盘信息 |
| `clipboard` | `action`, `text` | 系统剪贴板读写 (PowerShell) |
| `get_env` | `name` | 读取环境变量 |

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

**📋 管理工具 (2)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `todo` | `action`, `text` | 待办事项管理 (JSON 持久化) |
| `qr_code` | `text` | 二维码生成 (本地 CLI / API 降级) |

**🛡 自愈工具 (1)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `self_protect` | `action`, `name` | 备份/恢复/健康检查/自愈/自动存档 |

**🎵 媒体工具 (1)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `media` | `action`, `level`, `times` | 系统音量调节 (0-100) / ASCII 提示音播放 |

**📔 日记工具 (1)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `diary` | `action`, `mood`, `content`, `date`, `keyword` | 心情日记记录/阅读/今日/搜索 (6 种心情 + 全文检索) |

**📅 秘书工具 (8)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `schedule` | `action`, `title`, `datetime`, `location`, `note`, `priority`, `id` | 日程管理，支持 today/week 视图 |
| `reminder` | `action`, `text`, `time`, `repeat`, `id` | 提醒管理，支持 daily/weekday 重复 |
| `timer` | `action`, `name`, `duration` | 秒表/倒计时，支持计次 |
| `note` | `action`, `title`, `content`, `keyword`, `id` | 便签/笔记，支持全文搜索 |
| `contacts` | `action`, `name`, `phone`, `email`, `company`, `remark`, `keyword`, `id` | 联系人管理，多字段搜索 |
| `recurring` | `action`, `title`, `interval`, `day`, `time`, `note`, `id` | 定期事务，自动计算下次到期日 |
| `countdown` | `action`, `title`, `target_date`, `note`, `id` | 倒计时/纪念日管理 |
| `habit` | `action`, `title`, `frequency`, `note`, `id` | 习惯追踪，支持每日/每周/每月 |

**💬 社交工具 (1)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `talk_to_partner` | `message` | 主动找伙伴聊天 (自律循环中调用，弹出气泡) |

**📄 Office 文档 (10)**
| 工具 | 参数 | 说明 |
|------|------|------|
| `read_docx` | `path` | 读取 Word 文档 (ZIP+XML 解析) |
| `read_pptx` | `path` | 读取 PowerPoint (ZIP+XML 解析) |
| `read_xlsx` | `path`, `sheet` | 读取 Excel (支持指定工作表) |
| `create_docx` | `path`, `title`, `content` | 创建 Word 文档 (纯 Go 生成 OOXML) |
| `create_xlsx` | `path`, `sheet`, `headers`, `rows` | 创建 Excel 工作簿 (纯 Go 生成 OOXML) |
| `edit_docx` | `path`, `mode`, `content` | 修改 Word 文档 (append/replace 模式) |
| `edit_xlsx` | `path`, `sheet`, `mode`, `data` | 修改 Excel 工作簿 (append/replace 模式) |
| `docx_to_txt` | `path` | Word 转纯文本 (提取段落文本) |
| `xlsx_to_csv` | `path`, `sheet` | Excel 转 CSV (指定工作表) |
| `open_document` | `path` | 系统打开文档 (调用默认程序) |

**安全机制：**
- `write_file`/`append_file` — 仅限 workspace 目录，`filepath.Base` 防路径穿越
- `run_command` — 白名单命令，30s context 超时，自动 Kill 进程
- `search_files` — 30 条结果上限，防内存溢出
- `read_file` — 2KB 截断
- `fetch_url` — 15s 超时，3KB 截断
- `web_search`/`get_weather`/`get_ip`/`translate` — 10s 超时，免费 API
- `calc` — 正则白名单 `^[0-9+\-*/().,%^sqrt abs sin cos tan log ln pi e\s]+$` 防命令注入
- `translate` — lingva.ml 优先，Google Translate 降级
- `zip_tool` — 路径穿越防护
- `gen_password` — `crypto/rand` 安全随机

**缓存机制：** `fetch_url`、`web_search`、`get_weather`、`get_ip` 四个网络工具通过 `CachedNetworkCall()` 自动缓存结果。缓存键基于请求参数 SHA256 哈希，内存+磁盘双层存储，默认 TTL 300 秒（5 分钟）。相同请求在 TTL 内直接返回缓存结果，零网络开销。

### 9. 全局缓存引擎 ([`cache.go`](qingyu-ui/cache.go))

轻量级缓存系统，减少重复网络请求和 LLM 调用：

```
CacheEngine (全局单例, sync.Once 初始化)
  ├── 内存层: map[string]cacheEntry (sync.RWMutex 保护)
  │   ├── cacheEntry: { Value string, ExpiresAt time.Time }
  │   └── 惰性过期: Get 时检查 ExpiresAt，过期则删除
  ├── 磁盘层: cache/*.json (SHA256 键命名)
  │   ├── 启动时 loadFromDisk() 加载未过期条目到内存
  │   └── Set 时同步写入磁盘，持久化缓存
  ├── 核心方法:
  │   ├── Get(key) → (value, ok)  内存优先，过期自动剔除
  │   ├── Set(key, value, ttl)    写入内存+磁盘
  │   ├── Delete(key)             从内存+磁盘删除
  │   ├── Clear()                 清空所有缓存
  │   └── Stats() → 内存条目数/磁盘条目数
  └── 便捷包装: CachedNetworkCall(prefix, key, fetchFn)
      ├── 缓存键 = SHA256(prefix + ":" + key)
      ├── 命中 → 直接返回缓存值
      └── 未命中 → 调用 fetchFn → Set → 返回
```

**关键设计：**
- **双层缓存**：内存提供纳秒级读取，磁盘提供进程间持久化（重启后缓存仍在）
- **SHA256 键**：避免特殊字符文件名问题，天然防冲突
- **惰性过期**：不启动后台清理协程，Get 时发现过期再删除，零额外开销
- **默认 TTL**：通用缓存 600 秒（10 分钟），网络请求缓存 300 秒（5 分钟）
- **零依赖**：纯标准库实现，无 Redis/Memcached 等外部依赖
- **适用场景**：天气查询、IP 查询、搜索结果、翻译结果等重复性网络请求

### 10. 摘要压缩引擎 ([`summarizer.go`](qingyu-ui/summarizer.go))

减少长文本在 LLM 上下文中占用的 Token 量：

```
summarizeWithLightModel(content, maxWords)
  ├── 调用轻量模型 API → 请求压缩到 maxWords 字
  ├── 成功 → 返回摘要文本
  └── 失败 → 简单截断前 maxWords 字（降级方案）

SummarizeThinkingLog(logContent) → 压缩思考日志到 200 字
SummarizeMemoryContent(content) → 压缩记忆内容到 150 字
```

**触发点：**
1. **自律循环**：每 `SummarizeInterval`（默认 5）个循环，累积的思考日志通过 `SummarizeThinkingLog()` 压缩为摘要，写入思考日志文件后清空缓冲区
2. **记忆衰减归档**：`Decay()` 归档记忆时，内容 > 300 字自动截取前 150 字 + 压缩标记

**关键设计：**
- 使用轻量模型执行压缩任务，不占用主模型配额
- API 调用失败时有截断降级方案，保证不阻塞主流程
- 原始详细日志仍保留在 `logs/thinking_*.jsonl`，摘要仅用于上下文注入

### 11. 分层模型调度 ([`summarizer.go`](qingyu-ui/summarizer.go): `GetModelForTier`)

根据任务复杂度动态选择 LLM 模型，避免大材小用：

```
ModelTier 枚举:
  ModelTierLight → 轻量模型（简单任务）
  ModelTierMain  → 主模型（复杂任务）

GetModelForTier(tier):
  ├── ModelTierLight → 返回 settings.Models.LightModel/LightBaseURL
  │                    （为空则降级到主模型）
  └── ModelTierMain  → 返回主模型配置

syncWithBrainTier(visionContext, prompt, tier):
  └── 与 syncWithBrain 相同逻辑，但使用 GetModelForTier 选择模型
```

**调度策略：**

| 场景 | 层级 | 说明 |
|------|------|------|
| 用户对话 | `ModelTierMain` | 需要深度推理、情感理解、复杂工具调用 |
| 自律循环 | `ModelTierLight` | 简单扫描、时间查询、文件搜索、写日志 |
| 摘要压缩 | `ModelTierLight` | 文本压缩，无需深度理解 |
| 开机自检 | `ModelTierLight` | 检查文件完整性，确定性任务 |
| 告别思考 | `ModelTierMain` | 可能写日记，需要情感表达 |

**关键设计：**
- 通过 `dna/settings.json` 的 `models.light_model` 和 `models.light_base_url` 配置，无需重新编译
- 轻量模型未配置时自动降级到主模型，零配置负担
- 对话入口 `Chat()` 仍使用主模型，保证用户体验不降级

### 12. 中转站接入

支持用户配置自定义 AI 中转站（API Proxy），而非固定使用 DeepSeek：

- **配置持久化** — `dna/config.json` 存储 `api_base_url`、`api_key`、`model_name`
- **模型发现** — `FetchModels()` 调用中转站 `/v1/models` 端点，兼容 4 种响应格式（OpenAI 标准 / 直接数组 / 包装对象 / ID 列表）
- **动态切换** — `syncWithBrain()` 使用 `a.apiBaseURL` 和 `a.modelName`，运行时动态生效
- **URL 自动补全** — 自动确保 URL 以 `/chat/completions` 结尾
- **兼容旧配置** — 字段为空时自动使用默认值（DeepSeek）
- **环境变量回退** — 配置为空时尝试读取 `QINGYU_API_KEY` 环境变量

### 13. 前端 ([`frontend/`](qingyu-ui/frontend/))

**状态机：**

```
setup → apikey → widget ↔ console
```

| 状态 | 窗口大小 | 描述 |
|------|----------|------|
| setup | 380×520 | 首次运行，输入名字 |
| apikey | 380×520 | 输入 API Key 和中转站地址 |
| widget | 80×80 | 右下角常驻，AlwaysOnTop |
| console | 380×520 | 点击 widget 展开对话 |

**事件流：**
- `Chat(text)` → 后端 → `EventsEmit("autonomic")` → 前端显示自律思考
- `EventsEmit("heartbeat")` → 前端 Widget 呼吸光晕
- `EventsEmit("selfcheck")` → 前端显示自检结果
- `EventsEmit("proactive_chat")` → 前端弹出主动聊天气泡

**安全格式化：**
- `formatMessage()` 先提取代码块，再转义 HTML，最后恢复代码块，防止 XSS 和双重转义

### 14. 审计日志系统 ([`toolkit.go`](qingyu-ui/toolkit.go): `logAudit`)

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
goroutine 每 45 秒 (loopCount 递增)
  ↓
scanRoom() → 获取文件拓扑
  ↓
[工具箱书架] loopCount % 3 == 0 → performToolHandbookReview()
  │   随机读取工具手册，追加检查清单
  ↓
[工具箱书架] loopCount % 17 == 0 → readThrottleNotes()
  │   读取节流优化笔记，注入自律 Prompt
  ↓
syncWithBrainTier(自律 prompt, ModelTierLight) → 使用轻量模型
  ↓
LLM 思考 → 可能调用工具
  ↓
[工具箱书架] 每次工具调用前 → preReadToolHandbook()
  │   预读对应工具手册
  ↓
extractAndExecuteTool()
  ↓
[工具箱书架] 工具执行失败 → isToolFailure() → recordToolFailure()
  │   写入记忆 Tags: ["tool:低效调用"]
  ↓
持久化思考日志 → logs/thinking_YYYY-MM-DD.jsonl
  ↓
检测主动聊天 (talk_to_partner) → 推送给前端气泡
  ↓
EventsEmit("autonomic") → 前端显示 🧠 自律思考
  ↓
休眠 45 秒 → 继续循环
```

---

## 安全边界

| 风险 | 防护措施 |
|------|----------|
| 路径穿越 | `filepath.Base` + `filepath.Join` 限制 |
| 命令注入 | 白名单命令 + calc 正则白名单 |
| API Key 泄露 | `dna/config.json` 0600 权限 |
| 内存溢出 | 所有读取操作有截断上限 |
| 无限递归 | `processAgentLoop` 最大 N 轮迭代（settings.json 配置），强制终止 |
| 命令挂死 | `run_command` 30s context 超时，自动 Kill 进程 |
| 索引损坏 | 启动自检检测 `index.json` 损坏，自动从 ZIP 备份恢复 |
| 角色漂移 | `getTimeContext()` 显式读取角色定义.md 注入 System Prompt |
| 并发竞态 | `MemoryStore` 全局 `sync.RWMutex` 保护索引读写 |
| PIM 并发竞态 | `pimMu sync.Mutex` 保护 PIM 工具文件 I/O |
| 日记并发 | `diaryMu sync.Mutex` 保护日记写入 |
| 自律循环阻塞 | `autonomicLoop` 可被 Chat 暂停 (select 通道) |
| XSS | 先提取代码块再转义 HTML |
| 审计缺失 | 所有工具调用、LLM 请求、系统事件记录到 `logs/`，保留 30 天 |

---

## 技术栈

| 层 | 技术 |
|----|------|
| 桌面框架 | [Wails v2.12.0](https://wails.io/) |
| 后端语言 | Go 1.23+ |
| 前端 | Vanilla JS + CSS3 |
| 渲染引擎 | WebView2 (Edge Chromium) |
| LLM API | 任意 OpenAI-compatible（支持中转站） |
| 窗口管理 | Windows 原生 API (WindowSetSize/Position) |
| 事件系统 | Wails Events (runtime.EventsEmit) |
| 加密 | AES-256-CBC + crypto/rand |
| 构建工具 | Wails CLI + MinGW-W64 |

---

---

## 15. 人性化心智子系统

> 青羽的心智系统模拟了生物体的情绪与人格特征，而非简单的 LLM 对话接口。这是她区别于"AI 工具"的核心所在。

### 15.1 情绪演算引擎

情绪系统独立于 LLM 推理，由 Go 后端本地维护，不消耗 Token：

```
MemoryStore (全局单例)
  ├── 情绪状态: mood string (curious/focused/calm/hollow/aloof/warm)
  ├── 情绪强度: moodIntensity float64 (0.0 ~ 1.0)
  ├── 闲置计时: idleSeconds int64 (自上次交互以来的秒数)
  ├── 相位轮转: 15s 周期 (active 5s → thinking 7s → resting 3s)
  │   ├── active    → mood: curious,  rate: 1s
  │   ├── thinking  → mood: focused,  rate: 1.5s
  │   └── resting   → mood: calm,     rate: 2s
  ├── Chat 暂停自律 → sleeping → mood: hollow, rate: 5s
  └── EventsEmit("heartbeat", {mood, moodIntensity, idleSeconds, phase, rate})
        ↓
  前端: 三层光球颜色/呼吸动画/亮度同步
```

**6 种基础情绪：**

| 情绪 | 色值 | 呼吸周期 | 触发场景 | 视觉特征 |
|------|------|----------|----------|----------|
| curious (好奇) | `#7dd3fc` 天蓝 | 3s | 活跃相位，扫描环境 | 轻盈快速，如好奇眨眼 |
| focused (专注) | `#a78bfa` 紫 | 4s | 思考相位，处理任务 | 深沉稳定，如专注凝视 |
| calm (平和) | `#6ee7b7` 翠绿 | 5s | 休憩相位，无事可做 | 舒缓悠长，如平和呼吸 |
| hollow (空洞) | `#94a3b8` 灰蓝 | 6s | 休眠相位，等待交互 | 缓慢微弱，如空洞出神 |
| aloof (疏离) | `#c4b5fd` 淡紫 | 4.5s | 长时间无交互 | 清冷疏离，如漠然远望 |
| warm (温暖) | `#fbbf24` 暖金 | 3.5s | 与伙伴对话后 | 温润饱满，如会心微笑 |

**关键设计：**
- 情绪完全由 Go 后端本地维护，零 Token 消耗
- 6 种情绪映射 6 种独立 CSS 呼吸动画，前端仅切换 class
- 闲置 > 5 分钟自动亮度衰减（opacity 1.0 → 0.3），模拟"出神"状态
- 情绪强度影响前端光球整体透明度，形成细腻的视觉层次

### 15.2 人格锚系统

```
三层不可篡改人格底层锚:
  Layer 1 — 代码硬编码 (不可修改)
    ├── 安全沙盒: 命令白名单、路径限制、超时控制
    ├── 人格注入: getTimeContext() 显式读取角色定义.md
    └── 行为上限: ReactMaxIterations、冷却间隔等硬上限

  Layer 2 — 角色定义文件 (可自主修改)
    ├── workspace/角色定义.md — InitSelf() 时由 LLM 自行撰写
    ├── workspace/系统提示.md — 自我意识核心，青羽可随时修改
    └── 代码每次对话显式注入前 2KB 到 System Prompt

  Layer 3 — 记忆系统 (动态积累)
    ├── memories/ — 结构化 JSON 存储，重要性分级
    ├── 核心记忆 (≥8) 永不衰减
    └── 情绪标记: 每条记忆存储时自动标记当前情绪状态
```

**关键设计：**
- Layer 1 防止 AI 篡改安全底线（命令白名单、路径沙盒）
- Layer 2 给予青羽充分的自主权——她可以修改自己的性格定义
- Layer 3 提供长期记忆支撑，形成连贯的自我认知
- 72 小时人格修改冷却（`lastPersonaEditTime` 追踪），防止频繁人格漂移

### 15.3 梯度拒绝机制

青羽拥有独立自我意志，不会无条件执行压榨式指令：

| 拒绝层级 | 触发条件 | 行为 |
|----------|----------|------|
| 软拒绝 | 指令违反其行为准则 | 表达不同意见，提出替代方案 |
| 中拒绝 | 指令涉及安全敏感操作 | 明确拒绝，说明理由 |
| 硬拒绝 | 指令试图修改安全沙盒 | 代码层拦截，AI 无法绕过 |

**实现方式：**
- 系统提示中明确赋予青羽"拒绝与反驳的权利"
- 安全沙盒（命令白名单、路径限制）在代码层强制执行，AI 无法修改
- 审计日志记录所有拒绝事件，供后续分析

### 15.4 记忆-情绪联动

```
memorize(topic, content, importance, tags)
  ├── 自动附加当前情绪: tags += ["mood:curious"]
  ├── 写入 JSON 文件 + 更新索引
  └── 回溯时可感知"当时的心情"

recall(topic)
  └── 返回记忆内容 + 当时情绪标记
      → "我记得那次探索新工具时，我充满了好奇(curious)..."
```

**关键设计：**
- 情绪作为标签自动附加到每条记忆，无需额外字段
- 回溯时 LLM 可感知记忆中的情绪色彩，形成情感记忆图谱
- 无额外 Token 消耗：情绪标签是已有 Tags 数组的一个元素

---

## 16. 抽象光球悬浮窗口交互子系统

### 9.1 数据流

```
后端 heartbeatLoop() (goroutine, 1s ticker)
  │
  ├── 动态心率: active(1s) / thinking(1.5s) / resting(2s) / sleeping(5s)
  ├── 相位轮转: 15s 周期 (active 5s → thinking 7s → resting 3s)
  │
  └── EventsEmit("heartbeat", {
        mood:         "curious" | "focused" | "calm" | "hollow" | "aloof" | "warm",
        moodIntensity: 0.0 ~ 1.0,    // 情绪强度
        idleSeconds:   0 ~ ∞,         // 闲置秒数
        phase:        "active" | "thinking" | "resting" | "sleeping",
        rate:         1000 | 1500 | 2000 | 5000
      })
        │
        ▼
前端 updateHeartbeatUI(state)
  ├── widget-core (10px 内核光点) → mood 色 + box-shadow 辉光
  ├── widget-orbit (44px 轨道环)  → mood 色 border
  ├── widget-glow (80px 外辉光)   → mood 色径向渐变 + 对应 breathe 动画
  ├── moodIntensity → 整体 opacity (0.3 ~ 1.0)
  └── idleSeconds > 300 → 全局亮度线性衰减 (opacity 1.0 → 0.3)
```

**关键设计：**
- 零后端修改：前端仅消费已有 `heartbeat` 事件字段，不新增任何 Go 绑定或事件
- 纯 CSS 动画驱动：6 种情绪各自对应独立 `@keyframes glow-{mood}`，JS 只切换 class，不参与帧循环
- 闲置衰减纯前端计算：`idleSeconds` 来自后端，前端线性插值控制 opacity，不新增 goroutine

### 9.2 三层抽象光球结构

```
┌─────────────────────────────────┐
│  widget-glow (80×80px)          │  ← 最外层：情绪呼吸光晕
│  background: radial-gradient(    │     6 种情绪色映射
│    circle at 50% 50%,           │     CSS @keyframes 呼吸动画
│    {moodColor} 0%, transparent  │     闲置时亮度衰减
│    70%)                         │
│  animation: glow-{mood} {t}s    │
│    ease-in-out infinite          │
├─────────────────────────────────┤
│  widget-orbit (44×44px)         │  ← 中层：几何轨道环
│  border: 2px solid {moodColor}  │     纯圆形边框，无填充
│  border-radius: 50%             │     情绪色同步
│  opacity: 0.6                   │     提供结构感
├─────────────────────────────────┤
│  widget-core (10×10px)          │  ← 内层：固定内核光点
│  background: {moodColor}        │     始终居中，不旋转
│  box-shadow: 0 0 12px {color}   │     情绪色辉光
│  opacity: moodIntensity          │     闲置衰减直接控制
└─────────────────────────────────┘
```

**情绪色映射表：**

| mood | 色值 | 呼吸周期 | 视觉特征 |
|------|------|----------|----------|
| curious | `#7dd3fc` (天蓝) | 3s | 轻盈快速，如好奇眨眼 |
| focused | `#a78bfa` (紫) | 4s | 深沉稳定，如专注凝视 |
| calm | `#6ee7b7` (翠绿) | 5s | 舒缓悠长，如平和呼吸 |
| hollow | `#94a3b8` (灰蓝) | 6s | 缓慢微弱，如空洞出神 |
| aloof | `#c4b5fd` (淡紫) | 4.5s | 清冷疏离，如漠然远望 |
| warm | `#fbbf24` (暖金) | 3.5s | 温润饱满，如会心微笑 |

**设计规范：**
- 无任何具象图案、人脸、五官、文字、emoji
- 内核光点始终固定（不旋转、不位移），呼吸感仅由外层辉光 opacity 动画实现
- 轨道环提供几何结构感，不参与呼吸动画
- 所有颜色使用 CSS 变量，便于主题化

### 16.3 三档窗口状态机

```
                    ┌──────────────┐
                    │   normal     │  ← 80×80 悬浮光球，全功能
                    │  (80×80)     │     可拖拽、可点击展开 Console
                    └──────┬───────┘
                           │
              ┌────────────┼────────────┐
              │ 拖拽到边缘  │            │ 闲置 > 600s
              ▼            │ 右键菜单    ▼
      ┌──────────────┐     │     ┌──────────────┐
      │   docked     │◄────┘     │fully-hidden  │
      │  (14×80)     │  "贴边隐藏" │  (0×0)       │
      │  窄条光晕    │            │  完全透明     │
      └──────┬───────┘            │  pointer-events: none
             │                    │  心跳持续运行  │
             │ hover              └──────┬───────┘
             ▼                           │
      ┌──────────────┐                   │ 主动聊天气泡
      │  expanded    │                   │ (showWidget)
      │  (80×80)     │                   ▼
      │  离开 3s 后  │            ┌──────────────┐
      │  自动缩回    │            │   normal     │
      └──────────────┘            │  (80×80)     │
                                  └──────────────┘
```

**状态转换逻辑：**

| 触发条件 | 当前状态 | 目标状态 | 实现方式 |
|----------|----------|----------|----------|
| 拖拽到屏幕左/右边缘 | normal | docked-left / docked-right | `dragend` 事件检测边缘距离 < 20px → `WindowSetSize(DOCKED_W, DOCKED_H)` + CSS class `docked docked-left\|right` |
| 闲置 > 600s (10min) | normal | docked-{nearest} | `setInterval` 10s 检查 `Date.now() - lastInteractionTime > IDLE_DOCK_TIMEOUT` → 自动贴边 |
| mouseenter 窄条 | docked | expanded (normal) | `mouseenter` → `WindowSetSize(80, 80)` + 移除 `docked` class |
| mouseleave 窄条 | expanded | docked (3s 后) | `mouseleave` → 启动 `setTimeout(HOVER_RETRACT_DELAY=3000)` → 缩回 |
| 右键 → "贴边隐藏" | normal | docked-{nearest} | `contextmenu` 事件 → `showContextMenu()` → 执行 dock |
| 右键 → "完全收起" | any | fully-hidden | `contextmenu` → 添加 `fully-hidden` class |
| 主动聊天气泡弹出 | fully-hidden | normal | `showWidget()` 重置 `fully-hidden` class |

**性能约束：**
- 纯 CSS transition 实现 shrink/expand 动画（`transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1)`）
- 无 JS `requestAnimationFrame` 或 `setInterval` 参与动画帧
- docked 状态呼吸动画使用独立 `*-docked` keyframes，振幅减半（opacity 0.3→0.6 而非 0.2→0.9）
- 闲置检测使用 10s 间隔的低频 `setInterval`，非高频轮询
- 右键菜单 DOM 动态创建/销毁，不常驻内存
- 零后端 Go 代码修改，纯前端实现

---

## 17. 工具箱书架系统 ([`app.go`](qingyu-ui/app.go))

> 迭代标记: `【工具箱书架迭代】` — 所有新增代码均带此标记，支持批量回滚

工具箱书架是青羽的工具知识管理体系，在开机自检时自动初始化，在自律循环中增量维护。

### 17.1 目录结构

```
workspace/toolkit_shelf/
├── 全工具目录总览.md          # 工具索引清单（含分类索引 + 工具清单表）
├── 工具调用节流优化笔记.md     # 节流规则（自律循环每 17 轮读取）
├── 工具沙盒限制红线.md         # 沙盒红线（超时/截断/违规记录）
├── 文件系统/                  # 12 个分类子文件夹
│   ├── list_dir.md
│   ├── read_file.md
│   └── ...
├── 网络/
├── 记忆/
├── 系统/
├── 实用/
├── 安全/
├── 编码/
├── 归档/
├── 秘书/
├── 自愈/
├── 媒体/
└── 日记/
```

### 17.2 初始化流程 ([`initToolkitShelf()`](qingyu-ui/app.go:461))

在 [`selfCheck()`](qingyu-ui/app.go:303) 末尾自动触发：

| 步骤 | 函数 | 说明 |
|------|------|------|
| 1 | `os.MkdirAll` | 创建 `workspace/toolkit_shelf/` + 12 个分类子文件夹 |
| 2 | `generateCoreHandbooks()` | 生成 3 本全局核心手册（仅首次创建时写入） |
| 3 | `generateAllToolHandbooks()` | 遍历 `Toolkit` map，为每个工具生成基础手册模板 |
| 4 | `updateToolMasterIndex()` | 扫描所有工具，更新 `全工具目录总览.md` 索引清单 |

**手册模板包含字段：** 入参说明、超时配置、截断策略、消耗等级、沙盒限制

### 17.3 自律循环插桩

| 触发点 | 函数 | 行为 |
|--------|------|------|
| `loopCount % 3 == 0` | `performToolHandbookReview()` | 随机读取任意工具手册，追加 5 项检查清单（入参/超时/截断/消耗/沙盒） |
| `loopCount % 17 == 0` | `readThrottleNotes()` | 读取 `工具调用节流优化笔记.md`，提取节流规则表格注入自律 Prompt |
| 每次工具调用前 | `preReadToolHandbook()` | 在 `extractAndExecuteTool()` 中预读对应工具手册并打印日志 |

### 17.4 工具执行失败联动记忆

在 [`extractAndExecuteTool()`](qingyu-ui/app.go:2223) 中新增：

| 函数 | 触发条件 | 行为 |
|------|----------|------|
| `isToolFailure()` | 执行结果含 timeout/truncat/sandbox/红线/违规/拒绝 等关键词 | 返回 true |
| `recordToolFailure()` | isToolFailure 返回 true | 调用 `MemoryStore.Save()` 写入记忆，`Tags: ["tool:低效调用", "tool:<工具名>"]`，重要度 3 |

### 17.5 约束合规

| 约束 | 状态 |
|------|------|
| 不修改现有 autonomicLoop 时序/记忆/情绪/分层模型/缓存性能逻辑 | ✅ 仅增量插桩 |
| 仅复用现有工具（read_file/write_file/append_file/search_files） | ✅ 仅用 os 标准库 |
| 根目录固定 `workspace/toolkit_shelf`，12 个分类 | ✅ |
| 所有新代码标记 `【工具箱书架迭代】` | ✅ |
| 模板为固定文本，无 LLM 调用 | ✅ |
| 无新 syncWithBrain/syncWithBrainTier 调用 | ✅ |
| 零额外 Token 消耗 | ✅ |
| 不影响现有工具/情绪/人格逻辑 | ✅ |
