package main

import (
	"fmt"
	"strings"
	"time"
)

func init() {
	// memorize - 存储记忆
	Toolkit["memorize"] = Tool{
		Name:        "memorize",
		Description: "存储一条记忆到长期记忆系统。支持设置重要性(1-10)、话题标签和关联链接。",
		Category:    "记忆",
		Execute: func(args map[string]string) string {
			content := args["content"]
			if content == "" {
				return "❌ 记忆内容不能为空"
			}
			topic := args["topic"]
			if topic == "" {
				topic = "general"
			}

			importance := 5
			fmt.Sscanf(args["importance"], "%d", &importance)
			if importance < 1 {
				importance = 1
			}
			if importance > 10 {
				importance = 10
			}

			links := strings.Split(args["links"], ",")
			var cleanLinks []string
			for _, l := range links {
				if l = strings.TrimSpace(l); l != "" {
					cleanLinks = append(cleanLinks, l)
				}
			}

			store := GetMemoryStore()
			entry := &MemoryEntry{
				ID:          newUUID(),
				Topic:       topic,
				Content:     content,
				Importance:  importance,
				Tags:        strings.Fields(args["tags"]),
				Links:       cleanLinks,
				CreatedAt:   time.Now().Unix(),
				UpdatedAt:   time.Now().Unix(),
				AccessCount: 1,
				Version:     1,
			}

			if err := store.Save(entry); err != nil {
				return fmt.Sprintf("❌ 记忆存储失败: %v", err)
			}
			return fmt.Sprintf("✅ 记忆已存储 (ID: %s, 话题: %s, 重要性: %d)", entry.ID, topic, importance)
		},
	}

	// recall - 检索记忆
	Toolkit["recall"] = Tool{
		Name:        "recall",
		Description: "从长期记忆中检索。支持关键词搜索、话题筛选、重要性过滤。",
		Category:    "记忆",
		Execute: func(args map[string]string) string {
			query := SearchQuery{
				Topic:         args["topic"],
				Keyword:       args["keyword"],
				Tags:          strings.Fields(args["tags"]),
				ImportanceMin: 0,
				ImportanceMax: 0,
				Limit:         10,
				SortBy:        args["sort"],
			}
			fmt.Sscanf(args["min_importance"], "%d", &query.ImportanceMin)
			fmt.Sscanf(args["max_importance"], "%d", &query.ImportanceMax)
			fmt.Sscanf(args["limit"], "%d", &query.Limit)
			if query.Limit <= 0 {
				query.Limit = 10
			}
			if query.Limit > 100 {
				query.Limit = 100
			}
			if query.SortBy == "" {
				query.SortBy = "importance"
			}

			store := GetMemoryStore()
			results, err := store.Search(query)
			if err != nil {
				return fmt.Sprintf("❌ 记忆检索失败: %v", err)
			}

			if len(results) == 0 {
				return "📭 未找到匹配的记忆"
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("📖 找到 %d 条记忆:\n\n", len(results)))
			for i, mem := range results {
				sb.WriteString(fmt.Sprintf("【%d】[%s] %s\n", i+1, mem.Topic, mem.Content))
				sb.WriteString(fmt.Sprintf("    ID: %s | 重要性: %d | 访问: %d次\n", mem.ID, mem.Importance, mem.AccessCount))
				sb.WriteString(fmt.Sprintf("    创建: %s | 更新: %s\n",
					time.Unix(mem.CreatedAt, 0).Format("2006-01-02 15:04"),
					time.Unix(mem.UpdatedAt, 0).Format("2006-01-02 15:04")))
				if len(mem.Tags) > 0 {
					sb.WriteString(fmt.Sprintf("    标签: %s\n", strings.Join(mem.Tags, ", ")))
				}
				if len(mem.Links) > 0 {
					sb.WriteString(fmt.Sprintf("    关联: %s\n", strings.Join(mem.Links, ", ")))
				}
				sb.WriteString("\n")
			}
			return sb.String()
		},
	}

	// forget - 删除记忆
	Toolkit["forget"] = Tool{
		Name:        "forget",
		Description: "删除指定ID的记忆，或按话题删除一组记忆。默认软删除(移入回收站)。",
		Category:    "记忆",
		Execute: func(args map[string]string) string {
			id := args["id"]
			topic := args["topic"]
			hard := args["hard"] == "true"

			store := GetMemoryStore()

			if id != "" {
				if err := store.Delete(id, !hard); err != nil {
					return fmt.Sprintf("❌ 删除失败: %v", err)
				}
				mode := "软删除"
				if hard {
					mode = "永久删除"
				}
				return fmt.Sprintf("✅ 记忆 %s 已%s", id, mode)
			}

			if topic != "" {
				count, err := store.DeleteByTopic(topic, !hard)
				if err != nil {
					return fmt.Sprintf("❌ 批量删除失败: %v", err)
				}
				mode := "软删除"
				if hard {
					mode = "永久删除"
				}
				return fmt.Sprintf("✅ 已%s话题「%s」下的 %d 条记忆", mode, topic, count)
			}

			return "❌ 请指定 id 或 topic"
		},
	}
}

// memory_stats - 记忆系统统计
func init() {
	Toolkit["memory_stats"] = Tool{
		Name:        "memory_stats",
		Description: "查看记忆系统的统计信息，包括总记忆数、话题分布、核心记忆数等。",
		Category:    "记忆",
		Execute: func(args map[string]string) string {
			store := GetMemoryStore()
			stats := store.Stats()

			var sb strings.Builder
			sb.WriteString("📊 记忆系统统计\n")
			sb.WriteString(strings.Repeat("─", 40) + "\n")
			sb.WriteString(fmt.Sprintf("总记忆数:    %d\n", stats.TotalEntries))
			sb.WriteString(fmt.Sprintf("核心记忆:    %d\n", stats.CoreEntries))
			sb.WriteString(fmt.Sprintf("已归档:      %d\n", stats.ArchivedCount))
			sb.WriteString(fmt.Sprintf("标签数:      %d\n", stats.TagCount))
			sb.WriteString(fmt.Sprintf("关联数:      %d\n", stats.TotalLinks))

			if len(stats.ByTopic) > 0 {
				sb.WriteString("\n📂 话题分布:\n")
				// 按计数排序
				type topicCount struct {
					topic string
					count int
				}
				var sorted []topicCount
				for t, c := range stats.ByTopic {
					sorted = append(sorted, topicCount{t, c})
				}
				for i := 0; i < len(sorted); i++ {
					for j := i + 1; j < len(sorted); j++ {
						if sorted[j].count > sorted[i].count {
							sorted[i], sorted[j] = sorted[j], sorted[i]
						}
					}
				}
				for _, tc := range sorted {
					bar := strings.Repeat("█", tc.count)
					if tc.count > 20 {
						bar = strings.Repeat("█", 20)
					}
					sb.WriteString(fmt.Sprintf("  %-12s %3d %s\n", tc.topic, tc.count, bar))
				}
			}

			if len(stats.RecentEntries) > 0 {
				sb.WriteString("\n🕐 最近记忆:\n")
				for _, e := range stats.RecentEntries {
					sb.WriteString(fmt.Sprintf("  [%s] %s\n", e.Topic, e.Summary))
				}
			}

			return sb.String()
		},
	}
}

// memory_link - 记忆关联管理
func init() {
	Toolkit["memory_link"] = Tool{
		Name:        "memory_link",
		Description: "管理记忆之间的关联链接。支持 link（建立关联）和 unlink（解除关联）操作。",
		Category:    "记忆",
		Execute: func(args map[string]string) string {
			action := args["action"]
			id1 := args["id1"]
			id2 := args["id2"]

			if id1 == "" || id2 == "" {
				return "❌ 需要两个记忆ID (id1, id2)"
			}

			store := GetMemoryStore()

			switch action {
			case "link":
				if err := store.Link(id1, id2, "related"); err != nil {
					return fmt.Sprintf("❌ 建立关联失败: %v", err)
				}
				return fmt.Sprintf("✅ 已建立关联: %s ↔ %s", id1, id2)
			case "unlink":
				if err := store.Unlink(id1, id2); err != nil {
					return fmt.Sprintf("❌ 解除关联失败: %v", err)
				}
				return fmt.Sprintf("✅ 已解除关联: %s ↔ %s", id1, id2)
			default:
				return "❌ 未知操作，请使用 link 或 unlink"
			}
		},
	}
}

// migrate_memory - 旧数据迁移
func init() {
	Toolkit["migrate_memory"] = Tool{
		Name:        "migrate_memory",
		Description: "将旧格式的 .md 记忆文件迁移到新的结构化 JSON 存储系统。",
		Category:    "记忆",
		Execute: func(args map[string]string) string {
			store := GetMemoryStore()
			count, err := store.MigrateOldFormat()
			if err != nil {
				return fmt.Sprintf("❌ 迁移失败: %v", err)
			}
			return fmt.Sprintf("✅ 迁移完成，共处理 %d 条旧记忆", count)
		},
	}
}

// decay_memory - 手动触发记忆衰减
func init() {
	Toolkit["decay_memory"] = Tool{
		Name:        "decay_memory",
		Description: "手动触发记忆衰减过程：长期未访问的记忆将降级或归档/删除。",
		Category:    "记忆",
		Execute: func(args map[string]string) string {
			store := GetMemoryStore()
			archived, deleted, err := store.Decay()
			if err != nil {
				return fmt.Sprintf("❌ 衰减过程出错: %v", err)
			}
			return fmt.Sprintf("✅ 衰减完成：归档 %d 条，删除 %d 条", archived, deleted)
		},
	}
}
