// PIM（个人信息管理）工具集
//
// 提供待办管理（todo）、笔记管理（note）、联系人管理（contact）、
// 日程管理（schedule）、提醒管理（reminder）等 PIM 功能。
// 数据以 JSON 文件存储在 workspace/ 目录下。
// 所有工具通过 init() 注册到全局 Toolkit。
package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func init() {
	// ===== 待办管理 =====
	Toolkit["todo"] = Tool{
		Name:        "todo",
		Description: "【待办管理】管理简单的待办事项列表。参数: action (list/add/done/clear), item (add 模式时需要), id (done 模式时需要)",
		Category:    "秘书",
		Execute: func(args map[string]string) string {
			todoFile := filepath.Join(RootDir, WorkspaceDir, "todo.json")
			action := args["action"]
			if action == "" {
				action = "list"
			}
			type TodoItem struct {
				ID   int    `json:"id"`
				Text string `json:"text"`
				Done bool   `json:"done"`
			}
			var todos []TodoItem
			if data, err := pimRead(todoFile); err == nil {
				json.Unmarshal(data, &todos)
			}
			switch action {
			case "list":
				if len(todos) == 0 {
					return "📋 待办列表为空"
				}
				var sb strings.Builder
				sb.WriteString("📋 待办事项\n")
				for _, t := range todos {
					status := "⬜"
					if t.Done {
						status = "✅"
					}
					sb.WriteString(fmt.Sprintf("  %s [%d] %s\n", status, t.ID, t.Text))
				}
				return sb.String()
			case "add":
				item := args["item"]
				if item == "" {
					return "错误：add 模式需要提供 item 参数"
				}
				maxID := 0
				for _, t := range todos {
					if t.ID > maxID {
						maxID = t.ID
					}
				}
				todos = append(todos, TodoItem{ID: maxID + 1, Text: item, Done: false})
				data, _ := json.MarshalIndent(todos, "", "  ")
				pimWrite(todoFile, data, 0644)
				return fmt.Sprintf("✅ 已添加待办: %s", item)
			case "done":
				idStr := args["id"]
				if idStr == "" {
					return "错误：done 模式需要提供 id 参数"
				}
				id, _ := strconv.Atoi(idStr)
				for i, t := range todos {
					if t.ID == id {
						todos[i].Done = true
						data, _ := json.MarshalIndent(todos, "", "  ")
						pimWrite(todoFile, data, 0644)
						return fmt.Sprintf("✅ 已完成待办 [%d]: %s", id, t.Text)
					}
				}
				return fmt.Sprintf("未找到 ID 为 %d 的待办", id)
			case "clear":
				if err := pimRemove(todoFile); err != nil {
					return "❌ 清空待办失败: " + err.Error()
				}
				return "🗑 已清空所有待办"
			default:
				return "错误：action 参数应为 list/add/done/clear"
			}
		},
	}

	// ===== 日程管理 =====
	Toolkit["schedule"] = Tool{
		Name:        "schedule",
		Description: "【日程管理】管理日历日程。参数: action (add/list/get/today/week/update/delete), title (日程标题), datetime (时间，格式 2006-01-02 15:04), location (地点), note (备注), priority (优先级 high/normal/low), id (update/delete 时需要)",
		Category:    "秘书",
		Execute: func(args map[string]string) string {
			schedFile := filepath.Join(RootDir, WorkspaceDir, "schedule.json")
			action := args["action"]
			if action == "" {
				action = "today"
			}

			type ScheduleItem struct {
				ID        int    `json:"id"`
				Title     string `json:"title"`
				Datetime  string `json:"datetime"`
				Location  string `json:"location"`
				Note      string `json:"note"`
				Priority  string `json:"priority"`
				CreatedAt string `json:"created_at"`
			}
			var schedules []ScheduleItem
			if data, err := pimRead(schedFile); err == nil {
				json.Unmarshal(data, &schedules)
			}

			now := time.Now()
			todayStr := now.Format("2006-01-02")

			switch action {
			case "add":
				title := args["title"]
				dt := args["datetime"]
				if title == "" || dt == "" {
					return "错误：add 需要 title 和 datetime 参数（datetime 格式: 2006-01-02 15:04）"
				}
				maxID := 0
				for _, s := range schedules {
					if s.ID > maxID {
						maxID = s.ID
					}
				}
				loc := args["location"]
				note := args["note"]
				pri := args["priority"]
				if pri == "" {
					pri = "normal"
				}
				schedules = append(schedules, ScheduleItem{
					ID:        maxID + 1,
					Title:     title,
					Datetime:  dt,
					Location:  loc,
					Note:      note,
					Priority:  pri,
					CreatedAt: now.Format("2006-01-02 15:04:05"),
				})
				data, _ := json.MarshalIndent(schedules, "", "  ")
				pimWrite(schedFile, data, 0644)
				return fmt.Sprintf("✅ 已添加日程 [%d]: %s (%s)", maxID+1, title, dt)

			case "list":
				if len(schedules) == 0 {
					return "📅 日程列表为空"
				}
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("📅 全部日程 (%d 项)\n", len(schedules)))
				sb.WriteString("───\n")
				for _, s := range schedules {
					priIcon := map[string]string{"high": "🔴", "normal": "🟡", "low": "🟢"}[s.Priority]
					if priIcon == "" {
						priIcon = "🟡"
					}
					sb.WriteString(fmt.Sprintf("%s [%d] %s\n   📆 %s", priIcon, s.ID, s.Title, s.Datetime))
					if s.Location != "" {
						sb.WriteString(fmt.Sprintf(" 📍 %s", s.Location))
					}
					sb.WriteString("\n")
				}
				return sb.String()

			case "get":
				idStr := args["id"]
				if idStr == "" {
					return "错误：get 需要 id 参数"
				}
				id, _ := strconv.Atoi(idStr)
				for _, s := range schedules {
					if s.ID == id {
						priLabel := map[string]string{"high": "高", "normal": "中", "low": "低"}[s.Priority]
						var sb strings.Builder
						sb.WriteString(fmt.Sprintf("📅 [%d] %s\n", s.ID, s.Title))
						sb.WriteString(fmt.Sprintf("  时间: %s\n", s.Datetime))
						if s.Location != "" {
							sb.WriteString(fmt.Sprintf("  地点: %s\n", s.Location))
						}
						if s.Note != "" {
							sb.WriteString(fmt.Sprintf("  备注: %s\n", s.Note))
						}
						sb.WriteString(fmt.Sprintf("  优先级: %s\n", priLabel))
						return sb.String()
					}
				}
				return fmt.Sprintf("未找到 ID 为 %d 的日程", id)

			case "today":
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("📅 今日日程 (%s)\n", todayStr))
				sb.WriteString("───\n")
				found := false
				for _, s := range schedules {
					if strings.HasPrefix(s.Datetime, todayStr) {
						found = true
						priIcon := map[string]string{"high": "🔴", "normal": "🟡", "low": "🟢"}[s.Priority]
						if priIcon == "" {
							priIcon = "🟡"
						}
						timePart := ""
						if len(s.Datetime) > 16 {
							timePart = s.Datetime[11:16]
						}
						sb.WriteString(fmt.Sprintf("%s [%d] %s — %s", priIcon, s.ID, timePart, s.Title))
						if s.Location != "" {
							sb.WriteString(fmt.Sprintf(" 📍 %s", s.Location))
						}
						sb.WriteString("\n")
					}
				}
				if !found {
					sb.WriteString("  今天没有日程安排 ✨\n")
				}
				return sb.String()

			case "week":
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("📅 本周日程 (起始 %s)\n", todayStr))
				sb.WriteString("───\n")
				found := false
				weekAhead := now.AddDate(0, 0, 7)
				todayParsed, _ := time.Parse("2006-01-02", todayStr)
				for _, s := range schedules {
					if len(s.Datetime) < 10 {
						continue
					}
					schedDate, err := time.Parse("2006-01-02", s.Datetime[:10])
					if err != nil {
						continue
					}
					if (schedDate.Equal(todayParsed) || schedDate.After(todayParsed)) && schedDate.Before(weekAhead.AddDate(0, 0, 1)) {
						found = true
						priIcon := map[string]string{"high": "🔴", "normal": "🟡", "low": "🟢"}[s.Priority]
						if priIcon == "" {
							priIcon = "🟡"
						}
						sb.WriteString(fmt.Sprintf("%s [%d] %s\n   %s", priIcon, s.ID, s.Title, s.Datetime))
						if s.Location != "" {
							sb.WriteString(fmt.Sprintf(" 📍 %s", s.Location))
						}
						sb.WriteString("\n")
					}
				}
				if !found {
					sb.WriteString("  本周没有日程安排 ✨\n")
				}
				return sb.String()

			case "update":
				idStr := args["id"]
				if idStr == "" {
					return "错误：update 需要 id 参数"
				}
				id, _ := strconv.Atoi(idStr)
				for i, s := range schedules {
					if s.ID == id {
						if t := args["title"]; t != "" {
							schedules[i].Title = t
						}
						if dt := args["datetime"]; dt != "" {
							schedules[i].Datetime = dt
						}
						if loc := args["location"]; loc != "" {
							schedules[i].Location = loc
						}
						if note := args["note"]; note != "" {
							schedules[i].Note = note
						}
						if pri := args["priority"]; pri != "" {
							schedules[i].Priority = pri
						}
						data, _ := json.MarshalIndent(schedules, "", "  ")
						pimWrite(schedFile, data, 0644)
						return fmt.Sprintf("✅ 已更新日程 [%d]: %s", id, schedules[i].Title)
					}
				}
				return fmt.Sprintf("未找到 ID 为 %d 的日程", id)

			case "delete":
				idStr := args["id"]
				if idStr == "" {
					return "错误：delete 需要 id 参数"
				}
				id, _ := strconv.Atoi(idStr)
				for i, s := range schedules {
					if s.ID == id {
						title := s.Title
						schedules = append(schedules[:i], schedules[i+1:]...)
						data, _ := json.MarshalIndent(schedules, "", "  ")
						pimWrite(schedFile, data, 0644)
						return fmt.Sprintf("🗑️ 已删除日程 [%d]: %s", id, title)
					}
				}
				return fmt.Sprintf("未找到 ID 为 %d 的日程", id)

			default:
				return "错误：action 参数应为 add/list/get/today/week/update/delete"
			}
		},
	}

	// ===== 提醒管理 =====
	Toolkit["reminder"] = Tool{
		Name:        "reminder",
		Description: "【提醒管理】设置和管理提醒。参数: action (add/list/done/delete/clear), text (提醒内容), time (提醒时间，格式 2006-01-02 15:04), repeat (可选: once/daily/weekday，默认 once), id (done/delete 时需要)",
		Category:    "秘书",
		Execute: func(args map[string]string) string {
			remindFile := filepath.Join(RootDir, WorkspaceDir, "reminders.json")
			action := args["action"]
			if action == "" {
				action = "list"
			}

			type ReminderItem struct {
				ID      int    `json:"id"`
				Text    string `json:"text"`
				Time    string `json:"time"`
				Repeat  string `json:"repeat"`
				Done    bool   `json:"done"`
				Created string `json:"created"`
			}
			var reminders []ReminderItem
			if data, err := pimRead(remindFile); err == nil {
				json.Unmarshal(data, &reminders)
			}

			now := time.Now()

			switch action {
			case "add":
				text := args["text"]
				tm := args["time"]
				if text == "" || tm == "" {
					return "错误：add 需要 text 和 time 参数（time 格式: 2006-01-02 15:04）"
				}
				repeat := args["repeat"]
				if repeat == "" {
					repeat = "once"
				}
				if repeat != "once" && repeat != "daily" && repeat != "weekday" {
					return "错误：repeat 参数应为 once/daily/weekday"
				}
				maxID := 0
				for _, r := range reminders {
					if r.ID > maxID {
						maxID = r.ID
					}
				}
				reminders = append(reminders, ReminderItem{
					ID:      maxID + 1,
					Text:    text,
					Time:    tm,
					Repeat:  repeat,
					Done:    false,
					Created: now.Format("2006-01-02 15:04:05"),
				})
				data, _ := json.MarshalIndent(reminders, "", "  ")
				pimWrite(remindFile, data, 0644)
				return fmt.Sprintf("⏰ 已设置提醒 [%d]: %s (%s, %s)", maxID+1, text, tm, repeat)

			case "list":
				var sb strings.Builder
				sb.WriteString("⏰ 提醒列表\n")
				sb.WriteString("───\n")
				pending := 0
				overdue := 0
				for _, r := range reminders {
					if r.Done {
						continue
					}
					pending++
					isOverdue := false
					if rpt, err := time.Parse("2006-01-02 15:04", r.Time); err == nil && rpt.Before(now) {
						isOverdue = true
						overdue++
					}
					status := "🔔"
					if isOverdue {
						status = "⏰⚠️"
					}
					repeatLabel := map[string]string{"once": "一次性", "daily": "每日", "weekday": "工作日"}[r.Repeat]
					sb.WriteString(fmt.Sprintf("%s [%d] %s\n   时间: %s (%s)\n", status, r.ID, r.Text, r.Time, repeatLabel))
				}
				if pending == 0 {
					sb.WriteString("  没有待处理的提醒 ✨\n")
				} else if overdue > 0 {
					sb.WriteString(fmt.Sprintf("\n⚠️ 有 %d 个提醒已过期\n", overdue))
				}
				return sb.String()

			case "done":
				idStr := args["id"]
				if idStr == "" {
					return "错误：done 需要 id 参数"
				}
				id, _ := strconv.Atoi(idStr)
				for i, r := range reminders {
					if r.ID == id {
						if r.Repeat != "once" {
							nextTime, err := time.Parse("2006-01-02 15:04", r.Time)
							if err == nil {
								if r.Repeat == "daily" {
									nextTime = nextTime.AddDate(0, 0, 1)
								} else if r.Repeat == "weekday" {
									for {
										nextTime = nextTime.AddDate(0, 0, 1)
										w := nextTime.Weekday()
										if w != time.Saturday && w != time.Sunday {
											break
										}
									}
								}
								reminders[i].Time = nextTime.Format("2006-01-02 15:04")
								data, _ := json.MarshalIndent(reminders, "", "  ")
								pimWrite(remindFile, data, 0644)
								return fmt.Sprintf("✅ 已确认提醒 [%d]，下次提醒: %s", id, reminders[i].Time)
							}
							// time.Parse 失败：时间格式异常，标记为完成并提示
							reminders[i].Done = true
							data, _ := json.MarshalIndent(reminders, "", "  ")
							pimWrite(remindFile, data, 0644)
							return fmt.Sprintf("⚠️ 提醒 [%d] 时间格式异常 (%s)，已标记完成", id, r.Time)
						}
						reminders[i].Done = true
						data, _ := json.MarshalIndent(reminders, "", "  ")
						pimWrite(remindFile, data, 0644)
						return fmt.Sprintf("✅ 已完成提醒 [%d]: %s", id, r.Text)
					}
				}
				return fmt.Sprintf("未找到 ID 为 %d 的提醒", id)

			case "delete":
				idStr := args["id"]
				if idStr == "" {
					return "错误：delete 需要 id 参数"
				}
				id, _ := strconv.Atoi(idStr)
				for i, r := range reminders {
					if r.ID == id {
						reminders = append(reminders[:i], reminders[i+1:]...)
						data, _ := json.MarshalIndent(reminders, "", "  ")
						pimWrite(remindFile, data, 0644)
						return fmt.Sprintf("🗑️ 已删除提醒 [%d]", id)
					}
				}
				return fmt.Sprintf("未找到 ID 为 %d 的提醒", id)

			case "clear":
				pimRemove(remindFile)
				return "🗑️ 已清空所有提醒"

			default:
				return "错误：action 参数应为 add/list/done/delete/clear"
			}
		},
	}

	// ===== 计时器 =====
	Toolkit["timer"] = Tool{
		Name:        "timer",
		Description: "【计时器】秒表和倒计时。参数: action (start/stop/status/lap/countdown), name (计时器名称，默认 default), duration (countdown 模式需要，格式如 5m30s 或 90s)",
		Category:    "秘书",
		Execute: func(args map[string]string) string {
			action := args["action"]
			name := args["name"]
			if name == "" {
				name = "default"
			}

			timerFile := filepath.Join(RootDir, WorkspaceDir, "timers.json")

			type TimerState struct {
				Name      string   `json:"name"`
				Mode      string   `json:"mode"`
				Running   bool     `json:"running"`
				StartTime string   `json:"start_time"`
				Elapsed   string   `json:"elapsed"`
				Remaining string   `json:"remaining"`
				Duration  string   `json:"duration"`
				Laps      []string `json:"laps"`
			}

			var timers []TimerState
			if data, err := pimRead(timerFile); err == nil {
				json.Unmarshal(data, &timers)
			}

			findTimer := func() *TimerState {
				for i := range timers {
					if timers[i].Name == name {
						return &timers[i]
					}
				}
				return nil
			}

			saveTimers := func() {
				data, _ := json.MarshalIndent(timers, "", "  ")
				pimWrite(timerFile, data, 0644)
			}

			calcElapsed := func(t *TimerState) time.Duration {
				base, _ := time.ParseDuration(t.Elapsed)
				if !t.Running {
					return base
				}
				start, err := time.Parse("2006-01-02 15:04:05", t.StartTime)
				if err != nil {
					return base
				}
				return base + time.Since(start)
			}

			formatDur := func(d time.Duration) string {
				d = d.Round(time.Second)
				h := d / time.Hour
				d -= h * time.Hour
				m := d / time.Minute
				d -= m * time.Minute
				s := d / time.Second
				if h > 0 {
					return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
				}
				return fmt.Sprintf("%02d:%02d", m, s)
			}

			switch action {
			case "start":
				t := findTimer()
				if t == nil {
					timers = append(timers, TimerState{
						Name:      name,
						Mode:      "stopwatch",
						Running:   true,
						StartTime: time.Now().Format("2006-01-02 15:04:05"),
						Elapsed:   "0s",
					})
					saveTimers()
					return fmt.Sprintf("⏱️ 计时器 '%s' 已启动", name)
				}
				if t.Running {
					return fmt.Sprintf("⏱️ 计时器 '%s' 已在运行中", name)
				}
				t.Running = true
				t.StartTime = time.Now().Format("2006-01-02 15:04:05")
				saveTimers()
				return fmt.Sprintf("⏱️ 计时器 '%s' 已恢复", name)

			case "stop":
				t := findTimer()
				if t == nil {
					return fmt.Sprintf("❌ 计时器 '%s' 不存在", name)
				}
				if !t.Running {
					return fmt.Sprintf("⏸️ 计时器 '%s' 已暂停", name)
				}
				elapsed := calcElapsed(t)
				t.Running = false
				t.Elapsed = elapsed.String()
				saveTimers()
				return fmt.Sprintf("⏸️ 计时器 '%s' 已暂停 (%s)", name, formatDur(elapsed))

			case "status":
				t := findTimer()
				if t == nil {
					return fmt.Sprintf("⏱️ 计时器 '%s' 未启动", name)
				}
				elapsed := calcElapsed(t)
				status := "▶️ 运行中"
				if !t.Running {
					status = "⏸️ 已暂停"
				}
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("%s 计时器 '%s'\n", status, name))
				sb.WriteString(fmt.Sprintf("  经过: %s\n", formatDur(elapsed)))
				if t.Mode == "countdown" && t.Duration != "" {
					dur, _ := time.ParseDuration(t.Duration)
					remaining := dur - elapsed
					if remaining < 0 {
						remaining = 0
					}
					sb.WriteString(fmt.Sprintf("  剩余: %s\n", formatDur(remaining)))
				}
				if len(t.Laps) > 0 {
					sb.WriteString(fmt.Sprintf("  计次: %d 次\n", len(t.Laps)))
				}
				return sb.String()

			case "lap":
				t := findTimer()
				if t == nil {
					return fmt.Sprintf("❌ 计时器 '%s' 未启动", name)
				}
				if !t.Running {
					return fmt.Sprintf("❌ 计时器 '%s' 未在运行", name)
				}
				elapsed := calcElapsed(t)
				lapStr := formatDur(elapsed)
				t.Laps = append(t.Laps, lapStr)
				saveTimers()
				return fmt.Sprintf("🏁 计次 #%d: %s", len(t.Laps), lapStr)

			case "countdown":
				duration := args["duration"]
				if duration == "" {
					return "错误：countdown 需要 duration 参数（如 5m30s 或 90s）"
				}
				d, err := time.ParseDuration(duration)
				if err != nil {
					return fmt.Sprintf("错误：无效的时间格式 '%s'（如 5m30s 或 90s）", duration)
				}
				t := findTimer()
				if t == nil {
					timers = append(timers, TimerState{
						Name:      name,
						Mode:      "countdown",
						Running:   true,
						StartTime: time.Now().Format("2006-01-02 15:04:05"),
						Elapsed:   "0s",
						Duration:  duration,
					})
				} else {
					t.Mode = "countdown"
					t.Running = true
					t.StartTime = time.Now().Format("2006-01-02 15:04:05")
					t.Elapsed = "0s"
					t.Duration = duration
					t.Laps = nil
				}
				saveTimers()
				return fmt.Sprintf("⏳ 倒计时 '%s' 已启动 (%s)", name, formatDur(d))

			default:
				return "错误：action 参数应为 start/stop/status/lap/countdown"
			}
		},
	}

	// ===== 便签/笔记 =====
	Toolkit["note"] = Tool{
		Name:        "note",
		Description: "【便签/笔记】快速记录和检索笔记。参数: action (add/list/get/search/update/delete), title (标题), content (内容), keyword (search 模式时需要), id (get/update/delete 时需要)",
		Category:    "秘书",
		Execute: func(args map[string]string) string {
			noteFile := filepath.Join(RootDir, WorkspaceDir, "notes.json")
			action := args["action"]
			if action == "" {
				action = "list"
			}

			type NoteItem struct {
				ID        int    `json:"id"`
				Title     string `json:"title"`
				Content   string `json:"content"`
				CreatedAt string `json:"created_at"`
				UpdatedAt string `json:"updated_at"`
			}
			var notes []NoteItem
			if data, err := pimRead(noteFile); err == nil {
				json.Unmarshal(data, &notes)
			}

			now := time.Now().Format("2006-01-02 15:04:05")

			switch action {
			case "add":
				title := args["title"]
				content := args["content"]
				if title == "" || content == "" {
					return "错误：add 需要 title 和 content 参数"
				}
				maxID := 0
				for _, n := range notes {
					if n.ID > maxID {
						maxID = n.ID
					}
				}
				notes = append(notes, NoteItem{
					ID:        maxID + 1,
					Title:     title,
					Content:   content,
					CreatedAt: now,
					UpdatedAt: now,
				})
				data, _ := json.MarshalIndent(notes, "", "  ")
				pimWrite(noteFile, data, 0644)
				return fmt.Sprintf("📝 已添加笔记 [%d]: %s", maxID+1, title)

			case "list":
				if len(notes) == 0 {
					return "📝 笔记列表为空"
				}
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("📝 笔记列表 (%d 篇)\n", len(notes)))
				sb.WriteString("───\n")
				for _, n := range notes {
					preview := n.Content
					if len(preview) > 60 {
						preview = preview[:60] + "..."
					}
					sb.WriteString(fmt.Sprintf("[%d] %s\n   %s\n", n.ID, n.Title, preview))
				}
				return sb.String()

			case "get":
				idStr := args["id"]
				if idStr == "" {
					return "错误：get 需要 id 参数"
				}
				id, _ := strconv.Atoi(idStr)
				for _, n := range notes {
					if n.ID == id {
						var sb strings.Builder
						sb.WriteString(fmt.Sprintf("📝 [%d] %s\n", n.ID, n.Title))
						sb.WriteString(fmt.Sprintf("  创建: %s\n", n.CreatedAt))
						sb.WriteString(fmt.Sprintf("  更新: %s\n", n.UpdatedAt))
						sb.WriteString("───\n")
						sb.WriteString(n.Content)
						return sb.String()
					}
				}
				return fmt.Sprintf("未找到 ID 为 %d 的笔记", id)

			case "search":
				keyword := args["keyword"]
				if keyword == "" {
					return "错误：search 需要 keyword 参数"
				}
				kw := strings.ToLower(keyword)
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("🔍 搜索笔记: '%s'\n", keyword))
				sb.WriteString("───\n")
				count := 0
				for _, n := range notes {
					if strings.Contains(strings.ToLower(n.Title), kw) || strings.Contains(strings.ToLower(n.Content), kw) {
						count++
						preview := n.Content
						if len(preview) > 80 {
							preview = preview[:80] + "..."
						}
						sb.WriteString(fmt.Sprintf("[%d] %s\n   %s\n", n.ID, n.Title, preview))
						if count >= 20 {
							sb.WriteString("... 结果过多，仅显示前 20 条\n")
							break
						}
					}
				}
				if count == 0 {
					sb.WriteString("  未找到匹配的笔记\n")
				}
				return sb.String()

			case "update":
				idStr := args["id"]
				if idStr == "" {
					return "错误：update 需要 id 参数"
				}
				id, _ := strconv.Atoi(idStr)
				for i, n := range notes {
					if n.ID == id {
						if t := args["title"]; t != "" {
							notes[i].Title = t
						}
						if c := args["content"]; c != "" {
							notes[i].Content = c
						}
						notes[i].UpdatedAt = now
						data, _ := json.MarshalIndent(notes, "", "  ")
						pimWrite(noteFile, data, 0644)
						return fmt.Sprintf("✅ 已更新笔记 [%d]: %s", id, notes[i].Title)
					}
				}
				return fmt.Sprintf("未找到 ID 为 %d 的笔记", id)

			case "delete":
				idStr := args["id"]
				if idStr == "" {
					return "错误：delete 需要 id 参数"
				}
				id, _ := strconv.Atoi(idStr)
				for i, n := range notes {
					if n.ID == id {
						title := n.Title
						notes = append(notes[:i], notes[i+1:]...)
						data, _ := json.MarshalIndent(notes, "", "  ")
						pimWrite(noteFile, data, 0644)
						return fmt.Sprintf("🗑️ 已删除笔记 [%d]: %s", id, title)
					}
				}
				return fmt.Sprintf("未找到 ID 为 %d 的笔记", id)

			default:
				return "错误：action 参数应为 add/list/get/search/update/delete"
			}
		},
	}

	// ===== 联系人管理 =====
	Toolkit["contacts"] = Tool{
		Name:        "contacts",
		Description: "【联系人管理】管理通讯录。参数: action (add/list/get/search/update/delete), name (姓名), phone (电话), email (邮箱), company (公司), remark (备注), keyword (search 时需要), id (get/update/delete 时需要)",
		Category:    "秘书",
		Execute: func(args map[string]string) string {
			contactFile := filepath.Join(RootDir, WorkspaceDir, "contacts.json")
			action := args["action"]
			if action == "" {
				action = "list"
			}

			type ContactItem struct {
				ID        int    `json:"id"`
				Name      string `json:"name"`
				Phone     string `json:"phone"`
				Email     string `json:"email"`
				Company   string `json:"company"`
				Remark    string `json:"remark"`
				CreatedAt string `json:"created_at"`
			}
			var contacts []ContactItem
			if data, err := pimRead(contactFile); err == nil {
				json.Unmarshal(data, &contacts)
			}

			now := time.Now().Format("2006-01-02 15:04:05")

			switch action {
			case "add":
				name := args["name"]
				phone := args["phone"]
				if name == "" {
					return "错误：add 需要 name 参数"
				}
				maxID := 0
				for _, c := range contacts {
					if c.ID > maxID {
						maxID = c.ID
					}
				}
				contacts = append(contacts, ContactItem{
					ID:        maxID + 1,
					Name:      name,
					Phone:     phone,
					Email:     args["email"],
					Company:   args["company"],
					Remark:    args["remark"],
					CreatedAt: now,
				})
				data, _ := json.MarshalIndent(contacts, "", "  ")
				pimWrite(contactFile, data, 0644)
				return fmt.Sprintf("👤 已添加联系人 [%d]: %s", maxID+1, name)

			case "list":
				if len(contacts) == 0 {
					return "👤 通讯录为空"
				}
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("👤 通讯录 (%d 人)\n", len(contacts)))
				sb.WriteString("───\n")
				for _, c := range contacts {
					sb.WriteString(fmt.Sprintf("[%d] %s", c.ID, c.Name))
					if c.Phone != "" {
						sb.WriteString(fmt.Sprintf(" 📞 %s", c.Phone))
					}
					if c.Company != "" {
						sb.WriteString(fmt.Sprintf(" 🏢 %s", c.Company))
					}
					sb.WriteString("\n")
				}
				return sb.String()

			case "get":
				idStr := args["id"]
				if idStr == "" {
					return "错误：get 需要 id 参数"
				}
				id, _ := strconv.Atoi(idStr)
				for _, c := range contacts {
					if c.ID == id {
						var sb strings.Builder
						sb.WriteString(fmt.Sprintf("👤 [%d] %s\n", c.ID, c.Name))
						if c.Phone != "" {
							sb.WriteString(fmt.Sprintf("  电话: %s\n", c.Phone))
						}
						if c.Email != "" {
							sb.WriteString(fmt.Sprintf("  邮箱: %s\n", c.Email))
						}
						if c.Company != "" {
							sb.WriteString(fmt.Sprintf("  公司: %s\n", c.Company))
						}
						if c.Remark != "" {
							sb.WriteString(fmt.Sprintf("  备注: %s\n", c.Remark))
						}
						return sb.String()
					}
				}
				return fmt.Sprintf("未找到 ID 为 %d 的联系人", id)

			case "search":
				keyword := args["keyword"]
				if keyword == "" {
					return "错误：search 需要 keyword 参数"
				}
				kw := strings.ToLower(keyword)
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("🔍 搜索联系人: '%s'\n", keyword))
				sb.WriteString("───\n")
				count := 0
				for _, c := range contacts {
					if strings.Contains(strings.ToLower(c.Name), kw) ||
						strings.Contains(c.Phone, kw) ||
						strings.Contains(strings.ToLower(c.Email), kw) ||
						strings.Contains(strings.ToLower(c.Company), kw) {
						count++
						sb.WriteString(fmt.Sprintf("[%d] %s", c.ID, c.Name))
						if c.Phone != "" {
							sb.WriteString(fmt.Sprintf(" 📞 %s", c.Phone))
						}
						sb.WriteString("\n")
					}
				}
				if count == 0 {
					sb.WriteString("  未找到匹配的联系人\n")
				}
				return sb.String()

			case "update":
				idStr := args["id"]
				if idStr == "" {
					return "错误：update 需要 id 参数"
				}
				id, _ := strconv.Atoi(idStr)
				for i, c := range contacts {
					if c.ID == id {
						if n := args["name"]; n != "" {
							contacts[i].Name = n
						}
						if p := args["phone"]; p != "" {
							contacts[i].Phone = p
						}
						if e := args["email"]; e != "" {
							contacts[i].Email = e
						}
						if co := args["company"]; co != "" {
							contacts[i].Company = co
						}
						if r := args["remark"]; r != "" {
							contacts[i].Remark = r
						}
						data, _ := json.MarshalIndent(contacts, "", "  ")
						pimWrite(contactFile, data, 0644)
						return fmt.Sprintf("✅ 已更新联系人 [%d]: %s", id, contacts[i].Name)
					}
				}
				return fmt.Sprintf("未找到 ID 为 %d 的联系人", id)

			case "delete":
				idStr := args["id"]
				if idStr == "" {
					return "错误：delete 需要 id 参数"
				}
				id, _ := strconv.Atoi(idStr)
				for i, c := range contacts {
					if c.ID == id {
						name := c.Name
						contacts = append(contacts[:i], contacts[i+1:]...)
						data, _ := json.MarshalIndent(contacts, "", "  ")
						pimWrite(contactFile, data, 0644)
						return fmt.Sprintf("🗑️ 已删除联系人 [%d]: %s", id, name)
					}
				}
				return fmt.Sprintf("未找到 ID 为 %d 的联系人", id)

			default:
				return "错误：action 参数应为 add/list/get/search/update/delete"
			}
		},
	}

	// ===== 定期事务 =====
	Toolkit["recurring"] = Tool{
		Name:        "recurring",
		Description: "【定期事务】管理周期性事务（如每月交房租、每周例会）。参数: action (add/list/get/done/delete), title (事务名称), interval (周期: daily/weekly/monthly/yearly), day (执行日: weekly 填 1-7 周几, monthly 填 1-31 日期), time (执行时间如 10:00), note (备注), id (get/done/delete 时需要)",
		Category:    "秘书",
		Execute: func(args map[string]string) string {
			recFile := filepath.Join(RootDir, WorkspaceDir, "recurring.json")
			action := args["action"]
			if action == "" {
				action = "list"
			}

			type RecurringItem struct {
				ID        int    `json:"id"`
				Title     string `json:"title"`
				Interval  string `json:"interval"` // daily/weekly/monthly/yearly
				Day       int    `json:"day"`      // weekly: 1-7, monthly: 1-31, yearly: 1-31
				Month     int    `json:"month"`    // yearly: 1-12，其他周期忽略
				Time      string `json:"time"`     // HH:MM
				Note      string `json:"note"`
				LastDone  string `json:"last_done"`
				NextDue   string `json:"next_due"`
				CreatedAt string `json:"created_at"`
			}
			var items []RecurringItem
			if data, err := pimRead(recFile); err == nil {
				json.Unmarshal(data, &items)
			}

			now := time.Now()
			nowStr := now.Format("2006-01-02 15:04:05")
			todayStr := now.Format("2006-01-02")

			calcNextDue := func(item RecurringItem) string {
				today := now
				interval := item.Interval
				day := item.Day
				switch interval {
				case "daily":
					return todayStr
				case "weekly":
					if day < 1 || day > 7 {
						day = 1
					}
					currentWeekday := int(today.Weekday())
					targetWeekday := day % 7
					diff := (targetWeekday - currentWeekday + 7) % 7
					next := today.AddDate(0, 0, diff)
					return next.Format("2006-01-02")
				case "monthly":
					if day < 1 || day > 31 {
						day = 1
					}
					next := time.Date(today.Year(), today.Month(), day, 0, 0, 0, 0, today.Location())
					if next.Before(today) || next.Equal(today) {
						next = time.Date(today.Year(), today.Month()+1, day, 0, 0, 0, 0, today.Location())
					}
					return next.Format("2006-01-02")
				case "yearly":
					month := item.Month
					if month < 1 || month > 12 {
						month = 1
					}
					if day < 1 || day > 31 {
						day = 1
					}
					next := time.Date(today.Year(), time.Month(month), day, 0, 0, 0, 0, today.Location())
					if next.Before(today) || next.Equal(today) {
						next = time.Date(today.Year()+1, time.Month(month), day, 0, 0, 0, 0, today.Location())
					}
					return next.Format("2006-01-02")
				}
				return todayStr
			}

			switch action {
			case "add":
				title := args["title"]
				interval := args["interval"]
				if title == "" || interval == "" {
					return "错误：add 需要 title 和 interval 参数（daily/weekly/monthly/yearly）"
				}
				if interval != "daily" && interval != "weekly" && interval != "monthly" && interval != "yearly" {
					return "错误：interval 参数应为 daily/weekly/monthly/yearly"
				}
				day := 1
				if d := args["day"]; d != "" {
					day, _ = strconv.Atoi(d)
				}
				month := 1
				if interval == "yearly" {
					if m := args["month"]; m != "" {
						month, _ = strconv.Atoi(m)
					}
				}
				tm := args["time"]
				if tm == "" {
					tm = "09:00"
				}
				maxID := 0
				for _, r := range items {
					if r.ID > maxID {
						maxID = r.ID
					}
				}
				newItem := RecurringItem{
					ID:        maxID + 1,
					Title:     title,
					Interval:  interval,
					Day:       day,
					Month:     month,
					Time:      tm,
					Note:      args["note"],
					CreatedAt: nowStr,
				}
				newItem.NextDue = calcNextDue(newItem)
				items = append(items, newItem)
				data, _ := json.MarshalIndent(items, "", "  ")
				pimWrite(recFile, data, 0644)
				intervalLabel := map[string]string{"daily": "每天", "weekly": "每周", "monthly": "每月", "yearly": "每年"}[interval]
				return fmt.Sprintf("🔄 已添加定期事务 [%d]: %s (%s, 下次: %s %s)", maxID+1, title, intervalLabel, newItem.NextDue, tm)

			case "list":
				if len(items) == 0 {
					return "🔄 没有定期事务"
				}
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("🔄 定期事务 (%d 项)\n", len(items)))
				sb.WriteString("───\n")
				for _, r := range items {
					intervalLabel := map[string]string{"daily": "每天", "weekly": "每周", "monthly": "每月", "yearly": "每年"}[r.Interval]
					dueStatus := ""
					if r.NextDue < todayStr {
						dueStatus = " ⚠️ 已过期"
					} else if r.NextDue == todayStr {
						dueStatus = " 🔔 今日到期"
					}
					sb.WriteString(fmt.Sprintf("[%d] %s (%s)%s\n   下次: %s %s\n", r.ID, r.Title, intervalLabel, dueStatus, r.NextDue, r.Time))
				}
				return sb.String()

			case "get":
				idStr := args["id"]
				if idStr == "" {
					return "错误：get 需要 id 参数"
				}
				id, _ := strconv.Atoi(idStr)
				for _, r := range items {
					if r.ID == id {
						intervalLabel := map[string]string{"daily": "每天", "weekly": "每周", "monthly": "每月", "yearly": "每年"}[r.Interval]
						var sb strings.Builder
						sb.WriteString(fmt.Sprintf("🔄 [%d] %s\n", r.ID, r.Title))
						sb.WriteString(fmt.Sprintf("  周期: %s\n", intervalLabel))
						sb.WriteString(fmt.Sprintf("  时间: %s\n", r.Time))
						sb.WriteString(fmt.Sprintf("  下次: %s\n", r.NextDue))
						if r.Note != "" {
							sb.WriteString(fmt.Sprintf("  备注: %s\n", r.Note))
						}
						if r.LastDone != "" {
							sb.WriteString(fmt.Sprintf("  上次完成: %s\n", r.LastDone))
						}
						return sb.String()
					}
				}
				return fmt.Sprintf("未找到 ID 为 %d 的定期事务", id)

			case "done":
				idStr := args["id"]
				if idStr == "" {
					return "错误：done 需要 id 参数"
				}
				id, _ := strconv.Atoi(idStr)
				for i, r := range items {
					if r.ID == id {
						items[i].LastDone = nowStr
						items[i].NextDue = calcNextDue(items[i])
						data, _ := json.MarshalIndent(items, "", "  ")
						pimWrite(recFile, data, 0644)
						return fmt.Sprintf("✅ 已完成 [%d]: %s (下次: %s)", id, r.Title, items[i].NextDue)
					}
				}
				return fmt.Sprintf("未找到 ID 为 %d 的定期事务", id)

			case "delete":
				idStr := args["id"]
				if idStr == "" {
					return "错误：delete 需要 id 参数"
				}
				id, _ := strconv.Atoi(idStr)
				for i, r := range items {
					if r.ID == id {
						title := r.Title
						items = append(items[:i], items[i+1:]...)
						data, _ := json.MarshalIndent(items, "", "  ")
						pimWrite(recFile, data, 0644)
						return fmt.Sprintf("🗑️ 已删除定期事务 [%d]: %s", id, title)
					}
				}
				return fmt.Sprintf("未找到 ID 为 %d 的定期事务", id)

			default:
				return "错误：action 参数应为 add/list/get/done/delete"
			}
		},
	}
}
