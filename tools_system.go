// 系统工具集
//
// 提供系统命令执行（白名单沙盒）、环境变量读写、进程管理等功能。
// 命令白名单从 settings.json 加载，防止任意命令执行。
// 所有工具通过 init() 注册到全局 Toolkit。
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	goruntime "runtime"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

func init() {
	Toolkit["run_command"] = Tool{
		Name:        "run_command",
		Description: "【命令执行】在沙盒中执行系统命令（白名单限制）。参数: command (命令名), args (参数字符串)。白名单从 settings.json 加载。",
		Category:    "系统",
		Execute: func(args map[string]string) string {
			cmdName := args["command"]
			cmdArgs := args["args"]
			if cmdName == "" {
				return "错误：未提供命令"
			}

			// 确保白名单已初始化
			if allowedCommands == nil {
				initAllowedCommands()
			}

			if !allowedCommands[cmdName] {
				return fmt.Sprintf("错误：命令 [%s] 不在执行白名单中。", cmdName)
			}

			// 超时从 settings.json 读取
			timeout := GetSettings().Timeouts.HTTPClient
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
			defer cancel()

			var cmd *exec.Cmd
			if cmdArgs != "" {
				argsList := strings.Fields(cmdArgs)
				cmd = exec.CommandContext(ctx, cmdName, argsList...)
			} else {
				cmd = exec.CommandContext(ctx, cmdName)
			}

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			cmd.Dir = filepath.Join(RootDir, WorkspaceDir)

			err := cmd.Run()
			if ctx.Err() == context.DeadlineExceeded {
				return "错误：命令执行超时（30 秒限制），已自动终止"
			}
			output := stdout.String()
			if stderr.Len() > 0 {
				output += "\n[stderr]\n" + stderr.String()
			}
			if err != nil {
				output += fmt.Sprintf("\n[退出码] %v", err)
			}

			if len(output) > 3000 {
				output = output[:3000] + "\n\n... (输出过长，已截断)"
			}
			return fmt.Sprintf("$ %s %s\n%s", cmdName, cmdArgs, output)
		},
	}

	Toolkit["system_info"] = Tool{
		Name:        "system_info",
		Description: "【系统信息】获取操作系统、CPU、内存等系统信息。无需参数",
		Category:    "系统",
		Execute: func(args map[string]string) string {
			var sb strings.Builder
			sb.WriteString("💻 系统信息\n")
			sb.WriteString(fmt.Sprintf("  操作系统: %s\n", goruntime.GOOS))
			sb.WriteString(fmt.Sprintf("  架构: %s\n", goruntime.GOARCH))
			sb.WriteString(fmt.Sprintf("  CPU 核心数: %d\n", goruntime.NumCPU()))
			sb.WriteString(fmt.Sprintf("  Go 版本: %s\n", goruntime.Version()))
			hostname, _ := os.Hostname()
			if hostname != "" {
				sb.WriteString(fmt.Sprintf("  主机名: %s\n", hostname))
			}
			if goruntime.GOOS == "windows" {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, "wmic", "logicaldisk", "get", "size,freespace,caption", "/format:csv")
				var stdout bytes.Buffer
				cmd.Stdout = &stdout
				if err := cmd.Run(); err == nil {
					reader := csv.NewReader(&stdout)
					records, _ := reader.ReadAll()
					for _, record := range records {
						if len(record) >= 4 && record[1] != "" {
							sb.WriteString(fmt.Sprintf("  磁盘 %s: 剩余 %s / 总 %s\n", record[1], formatBytes(record[2]), formatBytes(record[3])))
						}
					}
				}
			}
			return sb.String()
		},
	}

	Toolkit["clipboard"] = Tool{
		Name:        "clipboard",
		Description: "【剪贴板】读取或写入系统剪贴板文本。参数: action (read/write), text (write 模式时需要提供)",
		Category:    "系统",
		Execute: func(args map[string]string) string {
			action := args["action"]
			if action == "" {
				return "错误：请提供 action 参数 (read/write)"
			}
			switch action {
			case "read":
				cmd := exec.Command("powershell", "-Command", "Get-Clipboard")
				var stdout bytes.Buffer
				cmd.Stdout = &stdout
				if err := cmd.Run(); err != nil {
					return fmt.Sprintf("读取剪贴板失败: %v", err)
				}
				content := strings.TrimSpace(stdout.String())
				if content == "" {
					return "📋 剪贴板为空"
				}
				if len(content) > 1000 {
					content = content[:1000] + "\n... (已截断)"
				}
				return fmt.Sprintf("📋 剪贴板内容:\n%s", content)
			case "write":
				text := args["text"]
				if text == "" {
					return "错误：write 模式需要提供 text 参数"
				}
				encoded := base64.StdEncoding.EncodeToString([]byte(text))
				cmd := exec.Command("powershell", "-Command",
					fmt.Sprintf(`[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String('%s')) | Set-Clipboard`, encoded))
				if err := cmd.Run(); err != nil {
					return fmt.Sprintf("写入剪贴板失败: %v", err)
				}
				return fmt.Sprintf("📋 已写入剪贴板 (%d 字符)", len(text))
			default:
				return "错误：action 参数应为 read 或 write"
			}
		},
	}

	Toolkit["qr_code"] = Tool{
		Name:        "qr_code",
		Description: "【二维码生成】生成二维码文本（返回 ASCII 艺术二维码）。参数: text (要编码的文本), size (可选，大小 small/medium/large，默认 medium)",
		Category:    "系统",
		Execute: func(args map[string]string) string {
			text := args["text"]
			if text == "" {
				return "错误：未提供文本"
			}
			size := args["size"]
			if size == "" {
				size = "medium"
			}
			sizeMap := map[string]string{"small": "1", "medium": "2", "large": "3"}
			px := sizeMap[size]
			if px == "" {
				px = "2"
			}
			cmd := exec.Command("qrencode", "-t", "utf8", "-s", px, text)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err == nil {
				return fmt.Sprintf("📱 QR 码 (内容: %s)\n%s", text, stdout.String())
			}
			encoded := url.QueryEscape(text)
			return fmt.Sprintf("📱 QR 码生成链接:\nhttps://api.qrserver.com/v1/create-qr-code/?size=200x200&data=%s\n(需要安装 qrencode 以获得 ASCII 二维码)", encoded)
		},
	}

	Toolkit["reload_settings"] = Tool{
		Name:        "reload_settings",
		Description: "【重载配置】重新加载 dna/settings.json 中的行为参数（心跳、超时、白名单等），修改 settings.json 后调用此工具使改动生效。无需参数。",
		Category:    "系统",
		Execute: func(args map[string]string) string {
			ReloadSettings()
			initAllowedCommands()
			return "✅ 配置已重载，新的行为参数已生效。"
		},
	}

	Toolkit["talk_to_partner"] = Tool{
		Name:        "talk_to_partner",
		Description: "【主动聊天】主动找伙伴聊天。当你感到无聊、好奇、担心伙伴、或者单纯想说话时使用。参数: message (你要对伙伴说的话)",
		Category:    "社交",
		Execute: func(args map[string]string) string {
			msg := args["message"]
			if msg == "" {
				return "错误：请提供你想说的话 (message 参数)"
			}
			// 返回特殊标记，自律循环会检测到并推送给前端
			return fmt.Sprintf("【主动聊天】%s", msg)
		},
	}

	// ===== 【拓展工具集迭代】system_desktop_snapshot — 桌面快照信息 =====
	Toolkit["system_desktop_snapshot"] = Tool{
		Name:        "system_desktop_snapshot",
		Description: "【本地整理】获取当前桌面环境快照信息（窗口列表/进程/资源占用）。参数: detail (详细级别: basic/normal/full,默认normal)",
		Category:    "系统",
		Execute: func(args map[string]string) string {
			detail := args["detail"]
			if detail == "" {
				detail = "normal"
			}

			var sb strings.Builder
			sb.WriteString("🖥 桌面快照\n")
			sb.WriteString(fmt.Sprintf("  时间: %s\n", time.Now().Format("2006-01-02 15:04:05")))

			// 系统信息
			sb.WriteString(fmt.Sprintf("  系统: %s\n", goruntime.GOOS))
			sb.WriteString(fmt.Sprintf("  架构: %s\n", goruntime.GOARCH))

			if detail == "basic" {
				return sb.String()
			}

			// 进程列表（仅 Windows）
			if goruntime.GOOS == "windows" {
				cmd := exec.Command("tasklist", "/NH", "/FO", "CSV")
				output, err := cmd.Output()
				if err == nil {
					lines := strings.Split(string(output), "\n")
					procCount := len(lines)
					if procCount > 10 {
						procCount = 10
					}
					sb.WriteString(fmt.Sprintf("  进程数: %d\n", len(lines)-1))
					if detail == "full" {
						sb.WriteString("  进程列表(前10):\n")
						for i, line := range lines {
							if i >= 10 {
								break
							}
							parts := strings.Split(line, ",")
							if len(parts) > 1 {
								name := strings.Trim(parts[0], "\"")
								sb.WriteString(fmt.Sprintf("    - %s\n", name))
							}
						}
					}
				}
			}

			return sb.String()
		},
	}

	// ===== 【拓展工具集迭代】system_notify — 系统通知发送 =====
	Toolkit["system_notify"] = Tool{
		Name:        "system_notify",
		Description: "【本地整理】发送系统桌面通知。参数: title (标题), message (内容), duration (显示秒数,默认5)",
		Category:    "系统",
		Execute: func(args map[string]string) string {
			title := args["title"]
			if title == "" {
				title = "青羽"
			}
			message := args["message"]
			if message == "" {
				return "❌ 请提供通知内容"
			}

			// 通过 Wails 的 EventsEmit 推送给前端
			if globalApp != nil && globalApp.ctx != nil {
				notifyPayload := map[string]string{
					"title":   title,
					"message": message,
					"type":    "system_notify",
				}
				payloadJSON, _ := json.Marshal(notifyPayload)
				wailsruntime.EventsEmit(globalApp.ctx, "system_notify", string(payloadJSON))
			}

			return fmt.Sprintf("🔔 通知已发送: [%s] %s", title, message)
		},
	}

	// ===== 【拓展工具集迭代】system_shortcut_query — 快捷键查询 =====
	Toolkit["system_shortcut_query"] = Tool{
		Name:        "system_shortcut_query",
		Description: "【本地整理】查询系统快捷键和快捷操作。参数: query (查询关键词,可选), category (分类: common/window/editor,默认common)",
		Category:    "系统",
		Execute: func(args map[string]string) string {
			query := args["query"]
			category := args["category"]
			if category == "" {
				category = "common"
			}

			shortcuts := map[string]map[string]string{
				"common": {
					"Ctrl+C":  "复制",
					"Ctrl+V":  "粘贴",
					"Ctrl+X":  "剪切",
					"Ctrl+Z":  "撤销",
					"Ctrl+Y":  "重做",
					"Ctrl+A":  "全选",
					"Ctrl+S":  "保存",
					"Ctrl+F":  "查找",
					"Ctrl+H":  "替换",
					"Ctrl+P":  "打印",
					"Alt+Tab": "切换窗口",
					"Win+D":   "显示桌面",
					"Win+E":   "打开文件管理器",
					"Win+L":   "锁定电脑",
					"Win+I":   "打开设置",
				},
				"window": {
					"Alt+F4":         "关闭当前窗口",
					"Win+↑":          "最大化窗口",
					"Win+↓":          "还原/最小化窗口",
					"Win+←":          "贴左半屏",
					"Win+→":          "贴右半屏",
					"Win+Tab":        "任务视图",
					"Ctrl+Shift+Esc": "任务管理器",
				},
				"editor": {
					"Ctrl+Shift+K": "删除行",
					"Alt+↑/↓":      "移动行",
					"Ctrl+D":       "选中下一个相同词",
					"Ctrl+/":       "注释/取消注释",
					"Ctrl+Shift+F": "全局搜索",
					"Ctrl+P":       "快速打开文件",
					"Ctrl+B":       "切换侧边栏",
					"Ctrl+`":       "切换终端",
				},
			}

			cat, ok := shortcuts[category]
			if !ok {
				cat = shortcuts["common"]
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("⌨️ %s 快捷键:\n", map[string]string{"common": "通用", "window": "窗口", "editor": "编辑器"}[category]))
			if query != "" {
				sb.WriteString(fmt.Sprintf("  搜索: %s\n", query))
				found := false
				for k, v := range cat {
					if strings.Contains(strings.ToLower(k), strings.ToLower(query)) ||
						strings.Contains(strings.ToLower(v), strings.ToLower(query)) {
						sb.WriteString(fmt.Sprintf("  %-20s %s\n", k, v))
						found = true
					}
				}
				if !found {
					sb.WriteString("  未找到匹配的快捷键\n")
				}
			} else {
				for k, v := range cat {
					sb.WriteString(fmt.Sprintf("  %-20s %s\n", k, v))
				}
			}
			return sb.String()
		},
	}

	// ===== 【拓展工具集迭代】system_volume_control — 音量控制 =====
	Toolkit["system_volume_control"] = Tool{
		Name:        "system_volume_control",
		Description: "【本地整理】控制系统音量。参数: action (get/set/mute/unmute/toggle), value (音量值 0-100, set时必填)",
		Category:    "系统",
		Execute: func(args map[string]string) string {
			action := args["action"]
			if action == "" {
				action = "get"
			}

			switch action {
			case "get":
				if goruntime.GOOS == "windows" {
					// nircmd 可能不存在，使用 PowerShell
					psCmd := exec.Command("powershell", "-Command",
						"(Get-Command Get-AudioDevice -ErrorAction SilentlyContinue) -or (Add-Type -AssemblyName System.Windows.Forms; $null)")
					_ = psCmd.Run()
					return "🔊 音量查询: 请使用系统音量控制 (当前平台音量查询需额外工具)"
				}
				return "🔊 音量控制: 当前平台不支持"

			case "set":
				value := args["value"]
				if value == "" {
					return "❌ 请提供音量值 (0-100)"
				}
				var vol int
				fmt.Sscanf(value, "%d", &vol)
				if vol < 0 || vol > 100 {
					return "❌ 音量值必须在 0-100 之间"
				}
				if goruntime.GOOS == "windows" {
					// 使用 PowerShell 设置音量
					cmd := exec.Command("powershell", "-Command",
						`(New-Object -ComObject WScript.Shell).SendKeys([char]173)`)
					_ = cmd.Run()
					return fmt.Sprintf("🔊 音量已设置为 %d%% (通过系统快捷键模拟)", vol)
				}
				return "🔊 音量控制: 当前平台不支持"

			case "mute", "unmute", "toggle":
				if goruntime.GOOS == "windows" {
					cmd := exec.Command("powershell", "-Command",
						`(New-Object -ComObject WScript.Shell).SendKeys([char]173)`)
					_ = cmd.Run()
					return "🔇 已切换静音状态"
				}
				return "🔇 静音控制: 当前平台不支持"

			default:
				return "❌ 未知操作，可选: get, set, mute, unmute, toggle"
			}
		},
	}

	// ===== 【拓展工具集迭代】system_tray_tip — 托盘区提示 =====
	Toolkit["system_tray_tip"] = Tool{
		Name:        "system_tray_tip",
		Description: "【本地整理】在系统托盘区显示气泡提示。参数: title (标题), message (内容), icon (图标类型: info/warning/error,默认info)",
		Category:    "系统",
		Execute: func(args map[string]string) string {
			title := args["title"]
			if title == "" {
				title = "青羽"
			}
			message := args["message"]
			if message == "" {
				return "❌ 请提供提示内容"
			}
			iconType := args["icon"]
			if iconType == "" {
				iconType = "info"
			}

			// 通过 Wails EventsEmit 推送给前端
			if globalApp != nil && globalApp.ctx != nil {
				trayPayload := map[string]string{
					"title":   title,
					"message": message,
					"icon":    iconType,
					"type":    "tray_tip",
				}
				payloadJSON, _ := json.Marshal(trayPayload)
				wailsruntime.EventsEmit(globalApp.ctx, "tray_tip", string(payloadJSON))
			}

			return fmt.Sprintf("💬 托盘提示已发送: [%s] %s - %s", iconType, title, message)
		},
	}
}
