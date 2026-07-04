package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ============================================
// 青羽的行为基因 — 可配置参数体系
// 所有运行时参数统一外移到 dna/settings.json
// 安全策略和人格定义仍保留在代码中（防止篡改）
// 设计原则：
//   - 惰性加载：首次 GetSettings() 时从文件读取
//   - 线程安全：所有读取通过 RWMutex 保护
//   - 热更新：ReloadSettings() 可在运行时重新加载
//   - 自动回退：文件不存在或损坏时使用 defaultSettings()
// ============================================

// Settings 青羽的完整行为配置
// 包含安全、心跳、超时、行为、路径、窗口、模型七大模块
type Settings struct {
	Security  SecurityConfig  `json:"security"`
	Heartbeat HeartbeatConfig `json:"heartbeat"`
	Timeouts  TimeoutConfig   `json:"timeouts"`
	Behavior  BehaviorConfig  `json:"behavior"`
	Paths     PathsConfig     `json:"paths"`
	Window    WindowConfig    `json:"window"`
	Models    ModelsConfig    `json:"models"`
}

// SecurityConfig 安全沙盒配置
// AllowedCommands: 允许通过 exec 执行的命令白名单
type SecurityConfig struct {
	AllowedCommands []string `json:"allowed_commands"`
}

// HeartbeatConfig 心跳动画配置
// 控制 UI 心跳脉冲的速率、相位切换和情绪发光模式
// PhaseRates: 各相位的心跳间隔（毫秒）
// EmitPatterns: 各速率对应的发射规则（always/mod/every）
type HeartbeatConfig struct {
	DefaultRate  int              `json:"default_rate"`
	DefaultPhase string           `json:"default_phase"`
	DefaultMood  string           `json:"default_mood"`
	PhaseRates   map[string]int   `json:"phase_rates"`
	CycleSeconds int              `json:"cycle_seconds"`
	ActiveSecs   int              `json:"active_seconds"`
	ThinkingSecs int              `json:"thinking_seconds"`
	RestingSecs  int              `json:"resting_seconds"`
	EmitPatterns map[int]EmitRule `json:"emit_patterns"`
}

// EmitRule 心跳发射规则
// Pattern: "always"=每次触发, "mod"=每 N 次触发, "every"=每 N 秒触发
type EmitRule struct {
	Pattern string `json:"pattern"`
	Mod     int    `json:"mod,omitempty"`
	Offset  int    `json:"offset,omitempty"`
}

// TimeoutConfig 各模块超时配置（秒）
type TimeoutConfig struct {
	HTTPClient   int `json:"http_client"`
	IMAPSMTP     int `json:"imap_smtp"`
	NetworkFetch int `json:"network_fetch"`
}

// BehaviorConfig 行为参数配置
// 控制自律循环、主动聊天、摘要压缩、网络工具等行为
type BehaviorConfig struct {
	AutonomicSleepSecs     int `json:"autonomic_sleep_seconds"`
	ReactMaxIterations     int `json:"react_max_iterations"`
	HeartbeatStartDelay    int `json:"heartbeat_start_delay"`
	AutonomicCheckInterval int `json:"autonomic_check_interval"`
	// 主动聊天冷却 & 情绪阈值
	ProactiveChatMinInterval int `json:"proactive_chat_min_interval"`
	ProactiveMoodThreshold   int `json:"proactive_mood_threshold"`
	// 摘要压缩
	SummarizeInterval int `json:"summarize_interval"`
	// 网络工具全局限制参数
	WebMaxFetchChars           int `json:"web_max_fetch_chars"`
	WebDownloadMaxSize         int `json:"web_download_max_size"`
	WebProactiveSearchCoolDown int `json:"web_proactive_search_cool_down"`
}

// PathsConfig 关键路径配置
// CriticalDirs/Files: 自检和备份时重点关注
type PathsConfig struct {
	ConfigFile    string   `json:"config_file"`
	CriticalDirs  []string `json:"critical_dirs"`
	CriticalFiles []string `json:"critical_files"`
}

// WindowConfig 窗口配置
// 支持透明无框模式，用于玻璃拟态 UI
type WindowConfig struct {
	Title       string `json:"title"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	MinWidth    int    `json:"min_width"`
	MinHeight   int    `json:"min_height"`
	AlwaysOnTop bool   `json:"always_on_top"`
	Frameless   bool   `json:"frameless"`
	Transparent bool   `json:"transparent"`
	DisableIcon bool   `json:"disable_icon"`
}

// ModelsConfig 分层模型配置
// LightModel: 轻量模型（处理简单工具调用、摘要、心跳自检）
// LightBaseURL: 轻量模型中转站（可选，默认同主模型）
// ToolComputeTier: 工具算力分级映射表（light/heavy）
type ModelsConfig struct {
	LightModel      string            `json:"light_model"`
	LightBaseURL    string            `json:"light_base_url"`
	ToolComputeTier map[string]string `json:"tool_compute_tier"`
}

// 全局单例
// settings: 当前生效的配置指针
// settingsOnce: 确保只加载一次
// settingsMu: 读写锁，支持并发读和互斥写
var (
	settings     *Settings
	settingsOnce sync.Once
	settingsMu   sync.RWMutex
)

// defaultSettings 返回出厂默认配置
// 当 dna/settings.json 不存在、损坏或解析失败时使用
// 包含安全的默认值，确保首次运行即可正常工作
func defaultSettings() *Settings {
	return &Settings{
		Security: SecurityConfig{
			AllowedCommands: []string{
				"dir", "echo", "type", "find", "findstr", "where",
				"git", "node", "npm", "npx",
				"go", "python", "pip",
				"ipconfig", "systeminfo", "tasklist",
			},
		},
		Heartbeat: HeartbeatConfig{
			DefaultRate:  2000,
			DefaultPhase: "resting",
			DefaultMood:  "calm",
			PhaseRates: map[string]int{
				"active":   1000,
				"thinking": 1500,
				"resting":  2000,
				"sleeping": 5000,
			},
			CycleSeconds: 15,
			ActiveSecs:   5,
			ThinkingSecs: 7,
			RestingSecs:  3,
			EmitPatterns: map[int]EmitRule{
				1000: {Pattern: "always"},
				1500: {Pattern: "mod", Mod: 3, Offset: 0},
				2000: {Pattern: "every", Mod: 2},
				5000: {Pattern: "every", Mod: 5},
			},
		},
		Timeouts: TimeoutConfig{
			HTTPClient:   30,
			IMAPSMTP:     15,
			NetworkFetch: 15,
		},
		Behavior: BehaviorConfig{
			AutonomicSleepSecs:         45,
			ReactMaxIterations:         3,
			HeartbeatStartDelay:        3,
			AutonomicCheckInterval:     3,
			ProactiveChatMinInterval:   300,      // 5 分钟
			ProactiveMoodThreshold:     3,        // 情绪值 <= 3 才发起主动聊天
			SummarizeInterval:          5,        // 每 5 轮摘要一次
			WebMaxFetchChars:           10000,    // 默认 10000 字符
			WebDownloadMaxSize:         52428800, // 默认 50MB
			WebProactiveSearchCoolDown: 60,       // 默认 60 秒
		},
		Paths: PathsConfig{
			ConfigFile:   "dna/config.json",
			CriticalDirs: []string{"dna", "memories", "workspace", "backups"},
			CriticalFiles: []string{
				"dna/config.json",
				"memories/creator.json",
				"workspace/角色定义.md",
				"workspace/书柜清单.md",
			},
		},
		Models: ModelsConfig{
			LightModel:   "", // 默认空，回退到主模型
			LightBaseURL: "",
			ToolComputeTier: map[string]string{
				// 低复杂度工具 → 轻量模型
				"web_multi_search":         "light",
				"web_link_parse":           "light",
				"web_rss_read":             "light",
				"system_desktop_snapshot":  "light",
				"system_shortcut_query":    "light",
				"file_batch_scan":          "light",
				"file_media_meta":          "light",
				"file_md_merge":            "light",
				"office_table_quick_parse": "light",
				"schedule_shared_plan":     "light",
				// 高复杂度工具 → 主模型
				"web_deep_extract":       "heavy",
				"web_image_analysis":     "heavy",
				"web_file_download_safe": "heavy",
				"web_archive_save":       "heavy",
				"system_notify":          "heavy",
				"system_volume_control":  "heavy",
				"system_tray_tip":        "heavy",
			},
		},
		Window: WindowConfig{
			Title:       "青羽",
			Width:       80,
			Height:      80,
			MinWidth:    80,
			MinHeight:   80,
			AlwaysOnTop: true,
			Frameless:   true,
			Transparent: true,
			DisableIcon: true,
		},
	}
}

// loadSettings 从 dna/settings.json 加载配置，失败时回退到默认值
func loadSettings() *Settings {
	path := filepath.Join(RootDir, "dna", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		// 文件不存在或无法读取，使用默认配置
		return defaultSettings()
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		fmt.Printf("⚠️ settings.json 解析失败 (%v)，使用默认配置\n", err)
		return defaultSettings()
	}

	return &s
}

// GetSettings 获取配置（线程安全，惰性加载）
// 首次调用时从 dna/settings.json 加载，之后返回缓存
// 使用 RWMutex.RLock 支持并发读取
func GetSettings() *Settings {
	settingsOnce.Do(func() {
		settings = loadSettings()
	})
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	return settings
}

// ReloadSettings 重新加载配置（运行时热更新）
// 从 dna/settings.json 重新读取并替换内存中的配置
// 使用写锁保证更新期间的线程安全
func ReloadSettings() {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	settings = loadSettings()
}

// SaveSettings 将配置持久化到 dna/settings.json
// 先写文件再更新内存，保证崩溃时不会丢失旧配置
// 文件权限 0600（仅当前用户可读写）
func SaveSettings(s *Settings) error {
	path := filepath.Join(RootDir, "dna", "settings.json")
	dir := filepath.Join(RootDir, "dna")
	os.MkdirAll(dir, 0755)

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 settings 失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("写入 settings.json 失败: %w", err)
	}

	// 更新内存
	settingsMu.Lock()
	settings = s
	settingsMu.Unlock()

	return nil
}

// InitSettings 初始化配置（在 startup 中调用）
// 首次运行时自动创建默认 settings.json
// 后续运行从文件加载到内存
func InitSettings() {
	path := filepath.Join(RootDir, "dna", "settings.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// 首次运行，写入默认配置
		defaults := defaultSettings()
		SaveSettings(defaults)
		fmt.Println("📝 已创建默认 settings.json")
	}
	// 加载到内存
	GetSettings()
}
