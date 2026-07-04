package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	Toolkit["list_dir"] = Tool{
		Name:        "list_dir",
		Description: "查看电脑某个目录下的文件列表。参数: path (绝对路径)",
		Category:    "文件系统",
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
		Category:    "文件系统",
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
		Category:    "文件系统",
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

			// ===== human_ext 增量优化：人格修改行为自动写入轻量记忆 =====
			// 拦截 write_file 写入 workspace/系统提示.md，自动创建轻量记忆
			if filepath.Base(filename) == "系统提示.md" {
				if ms := GetMemoryStore(); ms != nil {
					moodTag := "calm"
					if globalApp != nil && globalApp.moodState != "" {
						moodTag = globalApp.moodState
					}
					entry := &MemoryEntry{
						Topic:      "自我人格变更记录",
						Content:    "青羽在" + time.Now().Format("2006-01-02 15:04") + "修改了系统提示，调整了自我认知与行为倾向。",
						Tags:       []string{"persona_edit", "mood:" + moodTag},
						Importance: 6,
					}
					ms.Save(entry)
				}
			}

			return fmt.Sprintf("文件已写入: %s (%d 字节)", safePath, len(content))
		},
	}

	Toolkit["append_file"] = Tool{
		Name:        "append_file",
		Description: "【文件追加】在 workspace 中的文件末尾追加内容。参数: filename (文件名), content (追加内容)",
		Category:    "文件系统",
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
		Category:    "文件系统",
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
		Category:    "文件系统",
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

	// ===== 【拓展工具集迭代】file_batch_scan — 批量扫描目录文件 =====
	Toolkit["file_batch_scan"] = Tool{
		Name:        "file_batch_scan",
		Description: "【本地整理】批量扫描目录，按扩展名/大小/时间过滤文件。参数: dir (目录路径), ext (扩展名过滤,如 .md,.txt), max_depth (最大深度,默认3), min_size (最小字节), max_size (最大字节), sort_by (排序: name/size/time,默认name)",
		Category:    "文件系统",
		Execute: func(args map[string]string) string {
			dir := args["dir"]
			if dir == "" {
				dir = "."
			}
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(RootDir, dir)
			}

			maxDepth := 3
			if n := args["max_depth"]; n != "" {
				fmt.Sscanf(n, "%d", &maxDepth)
			}
			extFilter := args["ext"]
			var exts []string
			if extFilter != "" {
				for _, e := range strings.Split(extFilter, ",") {
					e = strings.TrimSpace(e)
					if !strings.HasPrefix(e, ".") {
						e = "." + e
					}
					exts = append(exts, strings.ToLower(e))
				}
			}
			var minSize, maxSize int64
			if n := args["min_size"]; n != "" {
				fmt.Sscanf(n, "%d", &minSize)
			}
			if n := args["max_size"]; n != "" {
				fmt.Sscanf(n, "%d", &maxSize)
			}
			sortBy := args["sort_by"]
			if sortBy == "" {
				sortBy = "name"
			}

			type fileEntry struct {
				path string
				size int64
				mod  time.Time
			}
			var files []fileEntry
			baseDepth := strings.Count(dir, string(filepath.Separator))

			filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				currentDepth := strings.Count(path, string(filepath.Separator)) - baseDepth
				if currentDepth > maxDepth {
					return nil
				}
				// 扩展名过滤
				if len(exts) > 0 {
					ext := strings.ToLower(filepath.Ext(path))
					found := false
					for _, e := range exts {
						if ext == e {
							found = true
							break
						}
					}
					if !found {
						return nil
					}
				}
				// 大小过滤
				if minSize > 0 && info.Size() < minSize {
					return nil
				}
				if maxSize > 0 && info.Size() > maxSize {
					return nil
				}
				files = append(files, fileEntry{path, info.Size(), info.ModTime()})
				return nil
			})

			if len(files) == 0 {
				return "📂 未找到匹配的文件"
			}

			// 排序
			switch sortBy {
			case "size":
				for i := 0; i < len(files); i++ {
					for j := i + 1; j < len(files); j++ {
						if files[j].size < files[i].size {
							files[i], files[j] = files[j], files[i]
						}
					}
				}
			case "time":
				for i := 0; i < len(files); i++ {
					for j := i + 1; j < len(files); j++ {
						if files[j].mod.Before(files[i].mod) {
							files[i], files[j] = files[j], files[i]
						}
					}
				}
			default:
				for i := 0; i < len(files); i++ {
					for j := i + 1; j < len(files); j++ {
						if files[j].path < files[i].path {
							files[i], files[j] = files[j], files[i]
						}
					}
				}
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("📂 扫描结果: %s (%d 个文件)\n", dir, len(files)))
			sb.WriteString(fmt.Sprintf("  过滤: ext=%v, min=%d, max=%d, depth=%d\n", exts, minSize, maxSize, maxDepth))
			sb.WriteString("  文件列表:\n")
			displayCount := 50
			if len(files) < displayCount {
				displayCount = len(files)
			}
			for i := 0; i < displayCount; i++ {
				rel, _ := filepath.Rel(dir, files[i].path)
				sb.WriteString(fmt.Sprintf("  %s (%s, %s)\n",
					rel, formatSize(files[i].size), files[i].mod.Format("01-02 15:04")))
			}
			if len(files) > displayCount {
				sb.WriteString(fmt.Sprintf("  ... 还有 %d 个文件\n", len(files)-displayCount))
			}
			return sb.String()
		},
	}

	// ===== 【拓展工具集迭代】file_md_merge — Markdown文件合并 =====
	Toolkit["file_md_merge"] = Tool{
		Name:        "file_md_merge",
		Description: "【本地整理】合并多个Markdown文件为一个。参数: files (文件路径列表,逗号分隔), output (输出文件名,可选), add_toc (是否添加目录, true/false,默认true)",
		Category:    "文件系统",
		Execute: func(args map[string]string) string {
			filesStr := args["files"]
			if filesStr == "" {
				return "❌ 请提供要合并的文件列表 (逗号分隔)"
			}
			outputName := args["output"]
			addTOC := args["add_toc"] != "false"

			fileList := strings.Split(filesStr, ",")
			if len(fileList) == 0 {
				return "❌ 请至少提供一个文件"
			}

			var merged strings.Builder
			var tocEntries []string
			fileIndex := 0

			for _, f := range fileList {
				f = strings.TrimSpace(f)
				path := f
				if !filepath.IsAbs(path) {
					path = filepath.Join(RootDir, path)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					merged.WriteString(fmt.Sprintf("\n\n> ⚠️ 无法读取: %s (%v)\n", f, err))
					continue
				}
				content := string(data)

				// 提取标题用于目录
				title := filepath.Base(f)
				re := regexp.MustCompile(`(?m)^#\s+(.+)$`)
				if m := re.FindStringSubmatch(content); len(m) > 1 {
					title = m[1]
				}

				if fileIndex > 0 {
					merged.WriteString("\n\n---\n\n")
				}
				merged.WriteString(fmt.Sprintf("\n<!-- 来源: %s -->\n", f))
				merged.WriteString(content)
				tocEntries = append(tocEntries, fmt.Sprintf("%d. [%s](#%s)", fileIndex+1, title, f))
				fileIndex++
			}

			// 构建最终输出
			var finalOutput strings.Builder
			if addTOC && len(tocEntries) > 0 {
				finalOutput.WriteString("# 目录\n\n")
				for _, entry := range tocEntries {
					finalOutput.WriteString(entry + "\n")
				}
				finalOutput.WriteString("\n---\n\n")
			}
			finalOutput.WriteString(merged.String())

			// 保存
			if outputName == "" {
				outputName = fmt.Sprintf("merged_%s.md", time.Now().Format("20060102_150405"))
			}
			outputPath := outputName
			if !filepath.IsAbs(outputPath) {
				outputPath = filepath.Join(RootDir, WorkspaceDir, outputPath)
			}
			if err := os.WriteFile(outputPath, []byte(finalOutput.String()), 0644); err != nil {
				return fmt.Sprintf("❌ 保存合并文件失败: %v", err)
			}

			return fmt.Sprintf("✅ 已合并 %d 个文件到: %s (%.1f KB)", fileIndex, outputPath, float64(len(finalOutput.String()))/1024)
		},
	}

	// ===== 【拓展工具集迭代】file_media_meta — 媒体文件元信息提取 =====
	Toolkit["file_media_meta"] = Tool{
		Name:        "file_media_meta",
		Description: "【本地整理】提取媒体文件（图片/音频/视频）的基础元信息。参数: path (文件路径)",
		Category:    "文件系统",
		Execute: func(args map[string]string) string {
			path := args["path"]
			if path == "" {
				return "❌ 请提供文件路径"
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(RootDir, path)
			}

			info, err := os.Stat(path)
			if err != nil {
				return fmt.Sprintf("❌ 无法访问文件: %v", err)
			}
			if info.IsDir() {
				return "❌ 请指定文件而非目录"
			}

			ext := strings.ToLower(filepath.Ext(path))
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Sprintf("❌ 读取文件失败: %v", err)
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("📁 %s\n", path))
			sb.WriteString(fmt.Sprintf("  大小: %s\n", formatSize(info.Size())))
			sb.WriteString(fmt.Sprintf("  修改时间: %s\n", info.ModTime().Format("2006-01-02 15:04:05")))
			sb.WriteString(fmt.Sprintf("  扩展名: %s\n", ext))

			// 根据扩展名尝试提取元信息
			switch ext {
			case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp":
				sb.WriteString("  类型: 图片\n")
				if len(data) >= 24 {
					// PNG
					if data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
						w := int(data[16])<<24 | int(data[17])<<16 | int(data[18])<<8 | int(data[19])
						h := int(data[20])<<24 | int(data[21])<<16 | int(data[22])<<8 | int(data[23])
						sb.WriteString(fmt.Sprintf("  格式: PNG\n  尺寸: %d x %d\n", w, h))
					} else if data[0] == 0xFF && data[1] == 0xD8 {
						sb.WriteString("  格式: JPEG\n")
						for i := 2; i < len(data)-9; i++ {
							if data[i] == 0xFF && (data[i+1] == 0xC0 || data[i+1] == 0xC1 || data[i+1] == 0xC2) {
								h := int(data[i+5])<<8 | int(data[i+6])
								w := int(data[i+7])<<8 | int(data[i+8])
								sb.WriteString(fmt.Sprintf("  尺寸: %d x %d\n", w, h))
								break
							}
						}
					} else if data[0] == 'G' && data[1] == 'I' && data[2] == 'F' {
						w := int(data[6]) | int(data[7])<<8
						h := int(data[8]) | int(data[9])<<8
						sb.WriteString(fmt.Sprintf("  格式: GIF\n  尺寸: %d x %d\n", w, h))
					} else {
						sb.WriteString("  格式: 未知图片格式\n")
					}
				}
			case ".mp3", ".wav", ".flac", ".ogg", ".m4a":
				sb.WriteString("  类型: 音频\n")
				sb.WriteString(fmt.Sprintf("  数据大小: %s\n", formatSize(int64(len(data)))))
			case ".mp4", ".avi", ".mkv", ".mov", ".wmv", ".flv":
				sb.WriteString("  类型: 视频\n")
				sb.WriteString(fmt.Sprintf("  数据大小: %s\n", formatSize(int64(len(data)))))
			case ".pdf":
				sb.WriteString("  类型: PDF文档\n")
				if len(data) > 5 && string(data[:5]) == "%PDF-" {
					sb.WriteString(fmt.Sprintf("  版本: %s\n", string(data[5:8])))
				}
			default:
				sb.WriteString(fmt.Sprintf("  类型: 未知媒体类型 (%s)\n", ext))
				// 尝试检测文件头
				if len(data) > 4 {
					header := fmt.Sprintf("%x", data[:4])
					sb.WriteString(fmt.Sprintf("  文件头: 0x%s\n", header))
				}
			}

			return sb.String()
		},
	}
}
