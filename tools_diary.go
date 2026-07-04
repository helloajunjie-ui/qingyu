package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ============================================
// 心情日记工具
// 以 JSON 格式存储每日心情和感悟
// 文件路径: memories/diary/{YYYY-MM-DD}.json
// 支持操作: write(写), read(读), today(今天), search(搜索), list(列表)
// ============================================

// diary — 心情日记
// 参数:
//
//	action: write/read/today/search/list
//	mood: happy/sad/calm/excited/anxious/tired/angry/loved/confused
//	content: 日记内容（write 必填）
//	date: 日期 YYYY-MM-DD（默认今天）
//	keyword: 搜索关键词（search 必填）
//
// 设计要点：
//   - 未指定 mood 时自动注入运行时全局情绪状态
//   - 日记文件按日期命名，便于按时间线浏览
//   - 搜索时全文匹配（大小写不敏感）
func init() {
	Toolkit["diary"] = Tool{
		Name:        "diary",
		Description: "📔 心情日记：记录每日心情和感悟。参数: action (write/read/today/search/list), mood (心情: happy/sad/calm/excited/anxious/tired/angry), content (内容), date (日期 YYYY-MM-DD, 默认今天), keyword (搜索关键词)",
		Category:    "日记",
		Execute: func(args map[string]string) string {
			diaryDir := filepath.Join(RootDir, "memories", "diary")
			os.MkdirAll(diaryDir, 0755)

			action := args["action"]
			date := args["date"]
			if date == "" {
				date = time.Now().Format("2006-01-02")
			}

			moodEmoji := map[string]string{
				"happy": "😊", "sad": "😢", "calm": "😌",
				"excited": "🎉", "anxious": "😰", "tired": "😴",
				"angry": "😤", "loved": "🥰", "confused": "🤔",
			}

			switch action {
			case "write":
				mood := args["mood"]
				content := args["content"]
				if content == "" {
					return "❌ 日记内容不能为空"
				}
				if mood == "" {
					mood = "calm"
				}

				// ===== human_ext 增量优化：日记自动绑定当前情绪标签 =====
				// 若用户未指定 mood，自动注入运行时全局情绪状态
				if args["mood"] == "" && globalApp != nil && globalApp.moodState != "" {
					mood = globalApp.moodState
				}

				diaryMu.Lock()
				path := filepath.Join(diaryDir, date+".json")

				// 读取已有日记（如果存在），追加新内容而非覆盖
				var existingContent string
				var existingMood string
				if data, err := os.ReadFile(path); err == nil {
					var existing map[string]string
					if json.Unmarshal(data, &existing) == nil {
						existingContent = existing["content"]
						existingMood = existing["mood"]
					}
				}

				now := time.Now().Format("2006-01-02 15:04:05")
				var finalContent string
				if existingContent != "" {
					// 已有日记：追加新段落，用时间戳分隔
					finalContent = existingContent + "\n\n---\n[" + now + "]\n" + content
					// 保留最早的情绪标签，除非显式指定了新 mood
					if args["mood"] == "" {
						mood = existingMood
					}
				} else {
					finalContent = content
				}

				entry := map[string]string{
					"date":    date,
					"mood":    mood,
					"content": finalContent,
					"created": now,
				}
				data, _ := json.MarshalIndent(entry, "", "  ")
				os.WriteFile(path, data, 0644)
				diaryMu.Unlock()

				// 日记是 AI 内部行为，不产生对话可见输出
				return ""

			case "read":
				path := filepath.Join(diaryDir, date+".json")
				data, err := os.ReadFile(path)
				if err != nil {
					return fmt.Sprintf("❌ %s 没有日记", date)
				}
				var entry map[string]string
				json.Unmarshal(data, &entry)
				emoji := moodEmoji[entry["mood"]]
				if emoji == "" {
					emoji = "📝"
				}
				return fmt.Sprintf("📔 %s 的心情: %s %s\n%s", date, emoji, entry["mood"], entry["content"])

			case "today":
				path := filepath.Join(diaryDir, date+".json")
				data, err := os.ReadFile(path)
				if err != nil {
					return "今天还没有写日记呢 📔"
				}
				var entry map[string]string
				json.Unmarshal(data, &entry)
				emoji := moodEmoji[entry["mood"]]
				if emoji == "" {
					emoji = "📝"
				}
				return fmt.Sprintf("📔 今天的心情: %s %s\n%s", emoji, entry["mood"], entry["content"])

			case "list":
				entries, _ := os.ReadDir(diaryDir)
				if len(entries) == 0 {
					return "📔 还没有写过日记呢"
				}
				var results []string
				for _, e := range entries {
					if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
						continue
					}
					data, _ := os.ReadFile(filepath.Join(diaryDir, e.Name()))
					var entry map[string]string
					json.Unmarshal(data, &entry)
					emoji := moodEmoji[entry["mood"]]
					if emoji == "" {
						emoji = "📝"
					}
					dateName := strings.TrimSuffix(e.Name(), ".json")
					contentPreview := entry["content"]
					if len([]rune(contentPreview)) > 40 {
						contentPreview = string([]rune(contentPreview)[:40]) + "..."
					}
					results = append(results, fmt.Sprintf("📔 %s %s [%s] %s", emoji, dateName, entry["mood"], contentPreview))
				}
				// 按日期倒序（最新的在前）
				for i := 0; i < len(results); i++ {
					for j := i + 1; j < len(results); j++ {
						if results[j] > results[i] {
							results[i], results[j] = results[j], results[i]
						}
					}
				}
				return "📔 我的日记本:\n" + strings.Join(results, "\n")

			case "search":
				keyword := args["keyword"]
				if keyword == "" {
					return "❌ 请提供搜索关键词"
				}
				entries, _ := os.ReadDir(diaryDir)
				var results []string
				for _, e := range entries {
					if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
						continue
					}
					data, _ := os.ReadFile(filepath.Join(diaryDir, e.Name()))
					if strings.Contains(strings.ToLower(string(data)), strings.ToLower(keyword)) {
						dateName := strings.TrimSuffix(e.Name(), ".json")
						var entry map[string]string
						json.Unmarshal(data, &entry)
						contentPreview := entry["content"]
						if len([]rune(contentPreview)) > 50 {
							contentPreview = string([]rune(contentPreview)[:50]) + "..."
						}
						results = append(results, fmt.Sprintf("📔 %s [%s]: %s", dateName, entry["mood"], contentPreview))
					}
				}
				if len(results) == 0 {
					return fmt.Sprintf("没有找到包含「%s」的日记", keyword)
				}
				return "📔 搜索结果:\n" + strings.Join(results, "\n")

			default:
				return "❌ 未知操作，可选: write, read, today, list, search"
			}
		},
	}
}
