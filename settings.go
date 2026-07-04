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
// 所有 P0/P1 级别的硬编码值统一外移到 settings.json
// 安全策略和人格定义仍保留在代码中（防止篡改）
// ============================================

// Settings 青羽的完整行为配置
type Settings struct {
	Security  SecurityConfig  `json:"security"`
	Heartbeat HeartbeatConfig `json:"heartbeat"`
	Timeouts  TimeoutConfig   `json:"timeouts"`
	Behavior  BehaviorConfig  `json:"behavior"`
	Paths     PathsConfig     `json:"paths"`
	Window    WindowConfig    `json:"window"`
	Models    ModelsConfig    `json:"models"`
}

type SecurityConfig struct {
	AllowedCommands []string `json:"allowed_commands"`
}

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

type EmitRule struct {
	Pattern string `json:"pattern"` // "always", "mod", "every"
	Mod     int    `json:"mod,omitempty"`
	Offset  int    `json:"offset,omitempty"`
}

type TimeoutConfig struct {
	HTTPClient   int `json:"http_client"`
	IMAPSMTP     int `json:"imap_smtp"`
	NetworkFetch int `json:"network_fetch"`
}

type BehaviorConfig struct {
	AutonomicSleepSecs     int `json:"autonomic_sleep_seconds"`
	ReactMaxIterations     int `json:"react_max_iterations"`
	HeartbeatStartDelay    int `json:"heartbeat_start_delay"`
	AutonomicCheckInterval int `json:"autonomic_check_interval"`
	// 主动聊天冷却 & 情绪阈值
	ProactiveChatMinInterval int `json:"proactive_chat_min_interval"` // 主动聊天最小间隔（秒）
	ProactiveMoodThreshold   int `json:"proactive_mood_threshold"`    // 情绪阈值，仅低于此值才发起主动聊天
	// 摘要压缩
	SummarizeInterval int `json:"summarize_interval"` // 每 N 轮自律循环做一次摘要压缩
}

type PathsConfig struct {
	ConfigFile    string   `json:"config_file"`
	CriticalDirs  []string `json:"critical_dirs"`
	CriticalFiles []string `json:"critical_files"`
}

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
type ModelsConfig struct {
	LightModel   string `json:"light_model"`    // 轻量模型名（如 deepseek-chat, gpt-4o-mini）
	LightBaseURL string `json:"light_base_url"` // 轻量模型中转站地址（可选，默认同主模型）
}

// 全局单例
var (
	settings     *Settings
	settingsOnce sync.Once
	settingsMu   sync.RWMutex
)

// defaultSettings 返回出厂默认配置（当 settings.json 不存在或损坏时使用）
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
			AutonomicSleepSecs:       45,
			ReactMaxIterations:       3,
			HeartbeatStartDelay:      3,
			AutonomicCheckInterval:   3,
			ProactiveChatMinInterval: 300, // 5 分钟
			ProactiveMoodThreshold:   3,   // 情绪值 <= 3 才发起主动聊天
			SummarizeInterval:        5,   // 每 5 轮摘要一次
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
func GetSettings() *Settings {
	settingsOnce.Do(func() {
		settings = loadSettings()
	})
	settingsMu.RLock()
	defer settingsMu.RUnlock()
	return settings
}

// ReloadSettings 重新加载配置（运行时热更新）
func ReloadSettings() {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	settings = loadSettings()
}

// SaveSettings 将当前配置写入 dna/settings.json
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
