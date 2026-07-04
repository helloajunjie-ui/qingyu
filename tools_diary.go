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

				// ===== human_ext 增量优化：日记自动绑定当前情绪标签 =====
				// 若用户未指定 mood，自动注入运行时全局情绪状态
				if args["mood"] == "" && globalApp != nil && globalApp.moodState != "" {
					mood = globalApp.moodState
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

					// ===== 【拓展工具集迭代】schedule_shared_plan — 双向计划同步 =====
					Toolkit["schedule_shared_plan"] = Tool{
						Name:        "schedule_shared_plan",
						Description: "【本地整理】双向计划管理：记录/查询/同步你和青羽的共同计划。参数: action (create/query/update/delete/list/sync), title (计划标题), content (计划内容), deadline (截止日期 YYYY-MM-DD), status (状态: pending/in_progress/done/cancelled), tag (分类标签), id (计划ID, update/delete时必填)",
						Category:    "日记",
						Execute: func(args map[string]string) string {
							planDir := filepath.Join(RootDir, "memories", "plans")
							os.MkdirAll(planDir, 0755)
							indexPath := filepath.Join(planDir, "index.json")

							action := args["action"]
							if action == "" {
								action = "list"
							}

							// 加载现有计划
							type Plan struct {
								ID        string `json:"id"`
								Title     string `json:"title"`
								Content   string `json:"content"`
								Deadline  string `json:"deadline"`
								Status    string `json:"status"`
								Tag       string `json:"tag"`
								CreatedAt string `json:"created_at"`
								UpdatedAt string `json:"updated_at"`
							}
							var plans []Plan
							if data, err := os.ReadFile(indexPath); err == nil {
								json.Unmarshal(data, &plans)
							}

							switch action {
							case "create":
								title := args["title"]
								if title == "" {
									return "❌ 请提供计划标题"
								}
								content := args["content"]
								deadline := args["deadline"]
								status := args["status"]
								if status == "" {
									status = "pending"
								}
								tag := args["tag"]
								now := time.Now().Format("2006-01-02 15:04:05")
								id := fmt.Sprintf("plan_%x", time.Now().UnixNano())

								plan := Plan{
									ID:        id,
									Title:     title,
									Content:   content,
									Deadline:  deadline,
									Status:    status,
									Tag:       tag,
									CreatedAt: now,
									UpdatedAt: now,
								}
								plans = append(plans, plan)
								data, _ := json.MarshalIndent(plans, "", "  ")
								os.WriteFile(indexPath, data, 0644)

								return fmt.Sprintf("✅ 计划已创建: %s\n  ID: %s\n  截止: %s\n  状态: %s", title, id, deadline, status)

							case "query":
								id := args["id"]
								if id == "" {
									return "❌ 请提供计划ID"
								}
								for _, p := range plans {
									if p.ID == id {
										return fmt.Sprintf("📋 计划详情:\n  标题: %s\n  内容: %s\n  截止: %s\n  状态: %s\n  标签: %s\n  创建: %s\n  更新: %s",
											p.Title, p.Content, p.Deadline, p.Status, p.Tag, p.CreatedAt, p.UpdatedAt)
									}
								}
								return fmt.Sprintf("❌ 未找到计划: %s", id)

							case "update":
								id := args["id"]
								if id == "" {
									return "❌ 请提供计划ID"
								}
								found := false
								for i, p := range plans {
									if p.ID == id {
										if title := args["title"]; title != "" {
											plans[i].Title = title
										}
										if content := args["content"]; content != "" {
											plans[i].Content = content
										}
										if deadline := args["deadline"]; deadline != "" {
											plans[i].Deadline = deadline
										}
										if status := args["status"]; status != "" {
											plans[i].Status = status
										}
										if tag := args["tag"]; tag != "" {
											plans[i].Tag = tag
										}
										plans[i].UpdatedAt = time.Now().Format("2006-01-02 15:04:05")
										found = true
										break
									}
								}
								if !found {
									return fmt.Sprintf("❌ 未找到计划: %s", id)
								}
								data, _ := json.MarshalIndent(plans, "", "  ")
								os.WriteFile(indexPath, data, 0644)
								return fmt.Sprintf("✅ 计划已更新: %s", id)

							case "delete":
								id := args["id"]
								if id == "" {
									return "❌ 请提供计划ID"
								}
								var newPlans []Plan
								found := false
								for _, p := range plans {
									if p.ID == id {
										found = true
										continue
									}
									newPlans = append(newPlans, p)
								}
								if !found {
									return fmt.Sprintf("❌ 未找到计划: %s", id)
								}
								data, _ := json.MarshalIndent(newPlans, "", "  ")
								os.WriteFile(indexPath, data, 0644)
								return fmt.Sprintf("🗑 计划已删除: %s", id)

							case "list":
								if len(plans) == 0 {
									return "📋 暂无计划"
								}
								tag := args["tag"]
								status := args["status"]
								var sb strings.Builder
								sb.WriteString(fmt.Sprintf("📋 共同计划 (%d 个):\n", len(plans)))
								for _, p := range plans {
									if tag != "" && p.Tag != tag {
										continue
									}
									if status != "" && p.Status != status {
										continue
									}
									statusEmoji := map[string]string{
										"pending":     "⏳",
										"in_progress": "🔄",
										"done":        "✅",
										"cancelled":   "❌",
									}
									emoji := statusEmoji[p.Status]
									if emoji == "" {
										emoji = "📌"
									}
									titlePreview := p.Title
									if len([]rune(titlePreview)) > 30 {
										titlePreview = string([]rune(titlePreview)[:30]) + "..."
									}
									sb.WriteString(fmt.Sprintf("  %s [%s] %s (截止: %s)\n", emoji, p.ID[:12], titlePreview, p.Deadline))
								}
								return sb.String()

							case "sync":
								// 同步：将计划写入 workspace 共享文件
								syncPath := filepath.Join(RootDir, WorkspaceDir, "共享计划.md")
								var sb strings.Builder
								sb.WriteString("# 📋 青羽与伙伴的共同计划\n\n")
								sb.WriteString(fmt.Sprintf("> 同步时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
								if len(plans) == 0 {
									sb.WriteString("暂无计划。\n")
								} else {
									for _, p := range plans {
										sb.WriteString(fmt.Sprintf("## %s\n", p.Title))
										sb.WriteString(fmt.Sprintf("- **状态**: %s\n", p.Status))
										sb.WriteString(fmt.Sprintf("- **截止**: %s\n", p.Deadline))
										sb.WriteString(fmt.Sprintf("- **标签**: %s\n", p.Tag))
										if p.Content != "" {
											sb.WriteString(fmt.Sprintf("- **内容**: %s\n", p.Content))
										}
										sb.WriteString("\n")
									}
								}
								os.WriteFile(syncPath, []byte(sb.String()), 0644)
								return fmt.Sprintf("✅ 计划已同步到: %s (%d 个计划)", syncPath, len(plans))

							default:
								return "❌ 未知操作，可选: create, query, update, delete, list, sync"
							}
						},
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
