package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func init() {
	Toolkit["run_command"] = Tool{
		Name:        "run_command",
		Description: "【命令执行】在沙盒中执行系统命令（白名单限制）。参数: command (命令名), args (参数字符串)。可用命令: dir, echo, type, find, findstr, where, git, node, npm, npx, go, python, pip, ipconfig, systeminfo, tasklist",
		Execute: func(args map[string]string) string {
			cmdName := args["command"]
			cmdArgs := args["args"]
			if cmdName == "" {
				return "错误：未提供命令"
			}

			if !allowedCommands[cmdName] {
				return fmt.Sprintf("错误：命令 [%s] 不在执行白名单中。允许的命令: dir, echo, type, find, findstr, where, git, node, npm, npx, go, python, pip, ipconfig, systeminfo, tasklist", cmdName)
			}

			// 30 秒超时，防止命令挂死
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
		Execute: func(args map[string]string) string {
			var sb strings.Builder
			sb.WriteString("💻 系统信息\n")
			sb.WriteString(fmt.Sprintf("  操作系统: %s\n", runtime.GOOS))
			sb.WriteString(fmt.Sprintf("  架构: %s\n", runtime.GOARCH))
			sb.WriteString(fmt.Sprintf("  CPU 核心数: %d\n", runtime.NumCPU()))
			sb.WriteString(fmt.Sprintf("  Go 版本: %s\n", runtime.Version()))
			hostname, _ := os.Hostname()
			if hostname != "" {
				sb.WriteString(fmt.Sprintf("  主机名: %s\n", hostname))
			}
			if runtime.GOOS == "windows" {
				cmd := exec.Command("wmic", "logicaldisk", "get", "size,freespace,caption", "/format:csv")
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
}
