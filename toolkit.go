package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ============================================
// 空间目录常量
// 青羽的文件系统采用"隔离沙盒"设计：
//   MemoryDir    — 记忆数据（JSON 结构化存储）
//   WorkspaceDir — 角色定义、日记、知识体系
//   WorkDir      — 用户工作区（与青羽空间隔离）
// ============================================

// RootDir      项目根目录
const RootDir = "."

// MemoryDir    青羽的记忆存储目录（JSON 文件）
const MemoryDir = "memories"

// WorkspaceDir 青羽的生活空间（角色定义、日记、知识体系）
const WorkspaceDir = "workspace"

// WorkDir      用户工作区（临时文件、附件、下载等，与青羽空间隔离）
const WorkDir = "workdir"

// Tool 定义了"书柜"里的每一本书（工具）的标准接口
// 所有工具通过 init() 函数在各自文件中注册到 Toolkit map
type Tool struct {
	Name        string                              // 工具名（唯一标识，用于 LLM 调用）
	Description string                              // 工具描述（LLM 理解用途）
	Category    string                              // 分类：文件系统/网络/记忆/系统/实用/安全/编码/归档/秘书/自愈/媒体/日记
	Execute     func(args map[string]string) string // 执行函数，接收参数字典，返回结果字符串
}

// allowedCommands 安全沙盒：允许执行的命令白名单（从 settings.json 加载）
var allowedCommands map[string]bool

// initAllowedCommands 从配置初始化命令白名单
// 在 startup 阶段调用，仅允许白名单内的命令通过 exec 执行
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
// 线程安全的 PIM 文件读写辅助函数
// PIM（个人信息管理）数据包括：todo/schedule/reminder/timer/note/contacts/recurring
// 所有读写操作通过 pimMu 互斥锁保护，防止并发写入导致数据损坏
// ============================================

// pimRead 加锁读取 PIM 数据文件
// 使用 pimMu 保证读取期间没有写入操作
func pimRead(path string) ([]byte, error) {
	pimMu.Lock()
	defer pimMu.Unlock()
	return os.ReadFile(path)
}

// pimWrite 加锁写入 PIM 数据文件
// 使用 pimMu 保证写入期间没有其他读写操作
func pimWrite(path string, data []byte, perm os.FileMode) error {
	pimMu.Lock()
	defer pimMu.Unlock()
	return os.WriteFile(path, data, perm)
}

// pimRemove 加锁删除 PIM 数据文件
// 使用 pimMu 保证删除期间没有其他读写操作
func pimRemove(path string) error {
	pimMu.Lock()
	defer pimMu.Unlock()
	return os.Remove(path)
}

// ============================================
// 审计日志系统
// 记录所有工具调用、LLM 请求和系统事件
// 按天轮转文件，自动清理 30 天前的日志
// 每条日志自动注入当前情绪标记，用于行为溯源
// ============================================

// auditMu 保护审计日志的并发写入
var auditMu sync.Mutex

// AuditEntry 单条审计日志结构
//
//	Type:   "tool_call" | "llm_request" | "system_event"
//	Action: 工具名 / "chat" / "autonomic" / 事件名
//	Detail: 简要描述（自动追加 current_mood=xxx 标记）
type AuditEntry struct {
	Time   string `json:"time"`
	Type   string `json:"type"`
	Action string `json:"action"`
	Detail string `json:"detail"`
}

// logAudit 异步写入审计日志
// 日志文件按天轮转（audit_YYYY-MM-DD.log），自动清理 30 天前的旧日志
// 每条日志自动追加 current_mood=xxx 字段，用于情绪行为溯源排查
// 设计要点：
//   - 异步写入，不阻塞调用方
//   - 每条日志独立一行 JSON，便于 grep 检索
//   - 自动清理过期日志，防止磁盘占用
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

// tryTranslateLingva 通过 lingva.ml 服务获取翻译结果
// 用于 web_search 工具中对外文结果做简要翻译
// 超时 5 秒，失败静默返回空字符串
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

// mathMaxFloat 返回两个 float64 中的较大值
func mathMaxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// mathMinFloat 返回两个 float64 中的较小值
func mathMinFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// formatBytes 将字节数格式化为人类可读的字符串
// 例如：1024 → "1.0 KB", 1048576 → "1.0 MB"
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
// 数据格式：[16字节 IV] + [密文]
// 密文使用 PKCS7 填充，解密后自动去除填充
// 用于 vault（密码保险箱）的数据解密
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
	// PKCS7 去填充：取最后一个字节的值作为填充长度
	padLen := int(ciphertext[len(ciphertext)-1])
	if padLen > len(ciphertext) || padLen == 0 {
		return nil, fmt.Errorf("invalid padding")
	}
	return ciphertext[:len(ciphertext)-padLen], nil
}

// encryptAES 使用 AES-256-CBC 加密数据
// 自动生成随机 IV（16 字节），追加到密文头部
// 使用 PKCS7 填充，确保明文长度对齐到块大小
// 用于 vault（密码保险箱）的数据加密
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

// saveVault 保存保险库数据到文件
// 1. 将数据序列化为 JSON
// 2. 使用 AES-256-CBC 加密
// 3. 以 0600 权限写入文件（仅当前用户可读写）
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
// Windows 实现：通过 PowerShell 创建隐藏 TextBox，设置文本后调用 Copy()
// 使用 Base64 编码传递文本，避免特殊字符（%、+、&、引号等）被 shell 损坏
// 静默失败（不返回错误），仅用于 vault get 的便捷复制
func tryClipboardWrite(text string) string {
	if runtime.GOOS == "windows" {
		// 使用 Base64 编码传递文本，避免特殊字符（%、+、&、引号等）损坏
		encoded := base64.StdEncoding.EncodeToString([]byte(text))
		cmd := exec.Command("powershell", "-c",
			fmt.Sprintf(`Add-Type -AssemblyName System.Windows.Forms; $tb = New-Object System.Windows.Forms.TextBox; $tb.Multiline = $true; $tb.Text = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('%s')); $tb.SelectAll(); $tb.Copy()`, encoded))
		cmd.Run()
	}
	return ""
}

// GetAvailableTools 生成按分类组织的"书柜目录"字符串
// 遍历 Toolkit map，按 categories 定义的顺序和图标输出
// 返回格式化的工具清单，供 LLM 理解可用工具
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
