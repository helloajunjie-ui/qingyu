package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 空间常量
const (
	RootDir      = "."
	MemoryDir    = "memories"  // 青羽的记忆空间
	WorkspaceDir = "workspace" // 青羽的生活空间（角色定义、日记、知识体系）
	WorkDir      = "workdir"   // 你的工作区（临时文件、附件、下载等，与青羽空间隔离）
)

// Tool 定义了"书柜"里的每一本书（工具）的标准接口
type Tool struct {
	Name        string
	Description string
	Category    string // 分类：文件系统/网络/记忆/系统/实用/安全/编码/归档/秘书/自愈/媒体/日记
	Execute     func(args map[string]string) string
}

// 安全沙盒：允许执行的命令白名单（从 settings.json 加载）
var allowedCommands map[string]bool

// initAllowedCommands 从配置初始化命令白名单
func initAllowedCommands() {
	allowedCommands = make(map[string]bool)
	cmds := GetSettings().Security.AllowedCommands
	for _, cmd := range cmds {
		allowedCommands[cmd] = true
	}
}

// Toolkit 青羽的专属书柜（各工具通过 init() 在独立文件中注册）
var Toolkit = map[string]Tool{}

// pimMu 保护 PIM 数据文件 (todo/schedule/reminder/timer/note/contacts/recurring) 的并发读写
var pimMu sync.Mutex

// diaryMu 保护日记文件的并发写入安全
var diaryMu sync.Mutex

// ============================================
// 线程安全的文件读写辅助函数
// ============================================

// pimRead 加锁读取 PIM 数据文件
func pimRead(path string) ([]byte, error) {
	pimMu.Lock()
	defer pimMu.Unlock()
	return os.ReadFile(path)
}

// pimWrite 加锁写入 PIM 数据文件
func pimWrite(path string, data []byte, perm os.FileMode) error {
	pimMu.Lock()
	defer pimMu.Unlock()
	return os.WriteFile(path, data, perm)
}

// pimRemove 加锁删除 PIM 数据文件
func pimRemove(path string) error {
	pimMu.Lock()
	defer pimMu.Unlock()
	return os.Remove(path)
}

// ============================================
// 审计日志系统
// ============================================

// auditMu 保护审计日志的并发写入
var auditMu sync.Mutex

// AuditEntry 单条审计日志
type AuditEntry struct {
	Time   string `json:"time"`
	Type   string `json:"type"`   // "tool_call" | "llm_request" | "system_event"
	Action string `json:"action"` // 工具名 / "chat" / "autonomic" / 事件名
	Detail string `json:"detail"` // 简要描述，不超过 200 字
}

// ===== human_ext 增量优化：审计日志注入当前情绪标记 =====
// logAudit 异步写入审计日志（按天轮转，自动清理 30 天前的日志）
// 每条日志自动追加 current_mood=xxx 字段，用于情绪行为溯源排查
func logAudit(entryType, action, detail string) {
	go func() {
		auditMu.Lock()
		defer auditMu.Unlock()

		logDir := filepath.Join(RootDir, "logs")
		os.MkdirAll(logDir, 0755)

		today := time.Now().Format("2006-01-02")
		logPath := filepath.Join(logDir, fmt.Sprintf("audit_%s.log", today))

		// human_ext：追加当前情绪标记
		moodTag := ""
		if globalApp != nil && globalApp.moodState != "" {
			moodTag = "current_mood=" + globalApp.moodState
		}
		enrichedDetail := detail
		if moodTag != "" {
			enrichedDetail = detail + " | " + moodTag
		}

		entry := AuditEntry{
			Time:   time.Now().Format("2006-01-02 15:04:05.000"),
			Type:   entryType,
			Action: action,
			Detail: enrichedDetail,
		}
		data, _ := json.Marshal(entry)

		// 追加写入，每条一行 JSON
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		defer f.Close()
		f.Write(data)
		f.Write([]byte("\n"))

		// 清理 30 天前的日志文件
		cutoff := time.Now().AddDate(0, 0, -30)
		entries, err := os.ReadDir(logDir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasPrefix(e.Name(), "audit_") || !strings.HasSuffix(e.Name(), ".log") {
				continue
			}
			// 文件名格式: audit_2006-01-02.log
			dateStr := strings.TrimPrefix(e.Name(), "audit_")
			dateStr = strings.TrimSuffix(dateStr, ".log")
			if t, err := time.Parse("2006-01-02", dateStr); err == nil {
				if t.Before(cutoff) {
					os.Remove(filepath.Join(logDir, e.Name()))
				}
			}
		}
	}()
}

// ============================================
// 辅助函数
// ============================================

// tryTranslateLingva 尝试从 lingva.ml 获取翻译结果
func tryTranslateLingva(url string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return ""
	}

	if translation, ok := result["translation"].(string); ok {
		return translation
	}
	return ""
}

func mathMaxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func mathMinFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func formatBytes(sizeStr string) string {
	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return sizeStr + "B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	sizeF := float64(size)
	for sizeF >= 1024 && i < len(units)-1 {
		sizeF /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", sizeF, units[i])
}

// decryptAES 解密 AES-256-CBC 加密的数据
func decryptAES(ciphertext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < aes.BlockSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext is not a multiple of the block size")
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ciphertext, ciphertext)
	// PKCS7 去填充
	padLen := int(ciphertext[len(ciphertext)-1])
	if padLen > len(ciphertext) || padLen == 0 {
		return nil, fmt.Errorf("invalid padding")
	}
	return ciphertext[:len(ciphertext)-padLen], nil
}

// encryptAES 加密数据（AES-256-CBC）
func encryptAES(plaintext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	// PKCS7 填充
	padLen := aes.BlockSize - len(plaintext)%aes.BlockSize
	padding := bytes.Repeat([]byte{byte(padLen)}, padLen)
	plaintext = append(plaintext, padding...)

	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	copy(ciphertext, iv)
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext[aes.BlockSize:], plaintext)
	return ciphertext, nil
}

// saveVault 保存保险库数据到文件（加密后写入）
func saveVault(path string, key []byte, data interface{}) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	encrypted, err := encryptAES(jsonData, key)
	if err != nil {
		return err
	}
	return os.WriteFile(path, encrypted, 0600)
}

// tryClipboardWrite 尝试将文本写入系统剪贴板
func tryClipboardWrite(text string) string {
	if runtime.GOOS == "windows" {
		encoded := url.QueryEscape(text)
		cmd := exec.Command("powershell", "-c",
			fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; $tb = New-Object System.Windows.Forms.TextBox; $tb.Multiline = $true; $tb.Text = [System.Net.WebUtility]::UrlDecode('%s'); $tb.SelectAll(); $tb.Copy()`, encoded))
		cmd.Run()
	}
	return ""
}

// GetAvailableTools 生成按分类组织的"书柜目录"
func GetAvailableTools() string {
	type categoryInfo struct {
		Icon  string
		Order int
	}
	categories := map[string]categoryInfo{
		"文件系统": {"📁", 1},
		"网络":   {"🌐", 2},
		"记忆":   {"🧠", 3},
		"系统":   {"💻", 4},
		"实用":   {"🔧", 5},
		"安全":   {"🔐", 6},
		"编码":   {"🎨", 7},
		"归档":   {"📦", 8},
		"秘书":   {"📅", 9},
		"自愈":   {"🛡", 10},
		"媒体":   {"🎵", 11},
		"日记":   {"📔", 12},
		"文档":   {"📄", 13},
		"社交":   {"💬", 14},
	}

	// 按分类分组
	grouped := make(map[string][]Tool)
	for _, tool := range Toolkit {
		cat := tool.Category
		if cat == "" {
			cat = "其他"
		}
		grouped[cat] = append(grouped[cat], tool)
	}

	// 按定义顺序输出
	var sb strings.Builder
	sb.WriteString("【📚 书柜目录】\n\n")
	for cat, info := range categories {
		tools, ok := grouped[cat]
		if !ok || len(tools) == 0 {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s %s\n", info.Icon, cat))
		for _, t := range tools {
			sb.WriteString(fmt.Sprintf("  · %s: %s\n", t.Name, t.Description))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}
