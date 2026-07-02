package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

				entry := map[string]string{
					"date":    date,
					"mood":    mood,
					"content": content,
					"created": time.Now().Format("2006-01-02 15:04:05"),
				}
				data, _ := json.MarshalIndent(entry, "", "  ")
				path := filepath.Join(diaryDir, date+".json")
				os.WriteFile(path, data, 0644)

				emoji := moodEmoji[mood]
				if emoji == "" {
					emoji = "📝"
				}
				return fmt.Sprintf("📔 %s 日记已记录 %s", date, emoji)

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
