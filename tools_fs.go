package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	Toolkit["list_dir"] = Tool{
		Name:        "list_dir",
		Description: "查看电脑某个目录下的文件列表。参数: path (绝对路径)",
		Execute: func(args map[string]string) string {
			path := args["path"]
			if path == "" {
				return "错误：未提供路径"
			}

			entries, err := os.ReadDir(path)
			if err != nil {
				return fmt.Sprintf("无法访问该目录: %v", err)
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("目录 [%s] 的内容:\n", path))
			for i, e := range entries {
				if i > 50 {
					sb.WriteString("... (文件过多，已截断)\n")
					break
				}
				if e.IsDir() {
					sb.WriteString(fmt.Sprintf("  [文件夹] %s\n", e.Name()))
				} else {
					sb.WriteString(fmt.Sprintf("  [文件]   %s\n", e.Name()))
				}
			}
			return sb.String()
		},
	}

	Toolkit["read_file"] = Tool{
		Name:        "read_file",
		Description: "读取电脑上的文本文件内容。参数: path (文件路径，支持相对路径)",
		Execute: func(args map[string]string) string {
			path := args["path"]
			if path == "" {
				return "错误：未提供文件路径"
			}

			if !filepath.IsAbs(path) {
				path = filepath.Join(RootDir, path)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Sprintf("读取失败: %v", err)
			}

			content := string(data)
			if len(content) > 2000 {
				content = content[:2000] + "\n\n... (文件过长，出于安全与记忆容量限制，已截断)"
			}
			return fmt.Sprintf("文件 [%s] 的内容:\n%s", path, content)
		},
	}

	Toolkit["write_file"] = Tool{
		Name:        "write_file",
		Description: "【文件写入】将内容写入 workspace 中的文件。参数: filename (文件名), content (文件内容)。注意：文件只能写入 workspace 目录",
		Execute: func(args map[string]string) string {
			filename := args["filename"]
			content := args["content"]
			if filename == "" {
				return "错误：未提供文件名"
			}

			safePath := filepath.Join(RootDir, WorkspaceDir, filepath.Base(filename))
			err := os.WriteFile(safePath, []byte(content), 0644)
			if err != nil {
				return fmt.Sprintf("写入失败: %v", err)
			}
			return fmt.Sprintf("文件已写入: %s (%d 字节)", safePath, len(content))
		},
	}

	Toolkit["append_file"] = Tool{
		Name:        "append_file",
		Description: "【文件追加】在 workspace 中的文件末尾追加内容。参数: filename (文件名), content (追加内容)",
		Execute: func(args map[string]string) string {
			filename := args["filename"]
			content := args["content"]
			if filename == "" {
				return "错误：未提供文件名"
			}

			safePath := filepath.Join(RootDir, WorkspaceDir, filepath.Base(filename))
			f, err := os.OpenFile(safePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return fmt.Sprintf("追加失败: %v", err)
			}
			defer f.Close()

			n, _ := f.WriteString(content + "\n")
			return fmt.Sprintf("已追加 %d 字节到: %s", n, safePath)
		},
	}

	Toolkit["search_files"] = Tool{
		Name:        "search_files",
		Description: "【文件搜索】在指定目录中搜索匹配关键词的文件。参数: path (目录路径，支持相对路径), keyword (搜索关键词), pattern (可选，文件通配符如 *.go *.md)",
		Execute: func(args map[string]string) string {
			path := args["path"]
			keyword := args["keyword"]
			pattern := args["pattern"]
			if path == "" {
				path = "."
			}
			if keyword == "" {
				return "错误：未提供搜索关键词"
			}

			if !filepath.IsAbs(path) {
				path = filepath.Join(RootDir, path)
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("在 [%s] 中搜索 \"%s\"", path, keyword))
			if pattern != "" {
				sb.WriteString(fmt.Sprintf(" (文件类型: %s)", pattern))
			}
			sb.WriteString(":\n")

			count := 0
			filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				if pattern != "" {
					matched, _ := filepath.Match(pattern, info.Name())
					if !matched {
						return nil
					}
				}
				data, err := os.ReadFile(filePath)
				if err != nil {
					return nil
				}
				lines := strings.Split(string(data), "\n")
				for i, line := range lines {
					if strings.Contains(line, keyword) {
						if count < 30 {
							relPath, _ := filepath.Rel(path, filePath)
							sb.WriteString(fmt.Sprintf("  %s:%d  %s\n", relPath, i+1, strings.TrimSpace(line)))
						}
						count++
					}
				}
				return nil
			})

			if count == 0 {
				sb.WriteString("  未找到匹配结果\n")
			} else if count > 30 {
				sb.WriteString(fmt.Sprintf("  ... 以及另外 %d 处匹配\n", count-30))
			}
			return sb.String()
		},
	}

	Toolkit["file_info"] = Tool{
		Name:        "file_info",
		Description: "【文件信息】获取文件或目录的详细信息（大小、修改时间、权限等）。参数: path (文件路径，支持相对路径)",
		Execute: func(args map[string]string) string {
			path := args["path"]
			if path == "" {
				return "错误：未提供路径"
			}

			if !filepath.IsAbs(path) {
				path = filepath.Join(RootDir, path)
			}

			info, err := os.Stat(path)
			if err != nil {
				return fmt.Sprintf("无法访问: %v", err)
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("📄 %s\n", path))
			sb.WriteString(fmt.Sprintf("  大小: %d 字节\n", info.Size()))
			sb.WriteString(fmt.Sprintf("  修改时间: %s\n", info.ModTime().Format("2006-01-02 15:04:05")))
			sb.WriteString(fmt.Sprintf("  权限: %s\n", info.Mode().Perm()))
			if info.IsDir() {
				sb.WriteString("  类型: 目录\n")
			} else {
				sb.WriteString("  类型: 文件\n")
			}
			return sb.String()
		},
	}
}
