package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func init() {
	Toolkit["self_protect"] = Tool{
		Name:        "self_protect",
		Description: "【自我保护】备份、恢复、健康检查、自愈。参数: action (backup/list_backups/restore/health/self_heal/auto_archive), name (备份名称，可选，默认自动生成时间戳)",
		Execute: func(args map[string]string) string {
			action := args["action"]
			if action == "" {
				action = "health"
			}

			backupDir := filepath.Join(RootDir, "backups")
			snapshotDir := filepath.Join(RootDir, "snapshots")

			os.MkdirAll(backupDir, 0700)
			os.MkdirAll(snapshotDir, 0700)

			criticalPaths := []string{
				filepath.Join(RootDir, "dna"),
				filepath.Join(RootDir, "memories"),
				filepath.Join(RootDir, "workspace"),
			}

			criticalFiles := []string{
				filepath.Join(RootDir, "dna", "config.json"),
				filepath.Join(RootDir, "dna", "vault.dat"),
				filepath.Join(RootDir, "memories", "creator.json"),
				filepath.Join(RootDir, "workspace", "角色定义.md"),
				filepath.Join(RootDir, "workspace", "书柜清单.md"),
				filepath.Join(RootDir, "workspace", "造物主档案.md"),
				filepath.Join(RootDir, "workspace", "schedule.json"),
				filepath.Join(RootDir, "workspace", "notes.json"),
				filepath.Join(RootDir, "workspace", "contacts.json"),
				filepath.Join(RootDir, "workspace", "recurring.json"),
				filepath.Join(RootDir, "workspace", "reminders.json"),
				filepath.Join(RootDir, "workspace", "todo.json"),
			}

			now := time.Now()
			timestamp := now.Format("2006-01-02_150405")

			switch action {
			case "backup":
				name := args["name"]
				if name == "" {
					name = "backup_" + timestamp
				}
				backupPath := filepath.Join(backupDir, name+".zip")

				buf := new(bytes.Buffer)
				zw := zip.NewWriter(buf)

				for _, basePath := range criticalPaths {
					filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
						if err != nil || info.IsDir() {
							return nil
						}
						relPath, _ := filepath.Rel(RootDir, path)
						f, _ := zw.Create(relPath)
						data, _ := os.ReadFile(path)
						f.Write(data)
						return nil
					})
				}
				zw.Close()

				if err := os.WriteFile(backupPath, buf.Bytes(), 0600); err != nil {
					return fmt.Sprintf("错误：备份失败: %v", err)
				}

				info, _ := os.Stat(backupPath)
				sizeStr := formatBytes(fmt.Sprintf("%d", info.Size()))
				return fmt.Sprintf("🛡️ 备份完成: %s (%s)\n  路径: %s", name, sizeStr, backupPath)

			case "list_backups":
				files, err := os.ReadDir(backupDir)
				if err != nil || len(files) == 0 {
					return "📂 没有找到备份文件"
				}
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("📂 备份列表 (%d 个)\n", len(files)))
				sb.WriteString("───\n")
				for _, f := range files {
					if !f.IsDir() && strings.HasSuffix(f.Name(), ".zip") {
						info, _ := f.Info()
						sizeStr := formatBytes(fmt.Sprintf("%d", info.Size()))
						sb.WriteString(fmt.Sprintf("  📦 %s (%s, %s)\n",
							strings.TrimSuffix(f.Name(), ".zip"),
							sizeStr,
							info.ModTime().Format("01-02 15:04")))
					}
				}
				return sb.String()

			case "restore":
				name := args["name"]
				if name == "" {
					return "错误：restore 需要 name 参数（备份名称，使用 list_backups 查看）"
				}
				backupPath := filepath.Join(backupDir, name+".zip")
				if _, err := os.Stat(backupPath); os.IsNotExist(err) {
					return fmt.Sprintf("错误：未找到备份 '%s'", name)
				}

				snapshotPath := filepath.Join(snapshotDir, "pre_restore_"+timestamp+".zip")
				buf := new(bytes.Buffer)
				zw := zip.NewWriter(buf)
				for _, basePath := range criticalPaths {
					filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
						if err != nil || info.IsDir() {
							return nil
						}
						relPath, _ := filepath.Rel(RootDir, path)
						f, _ := zw.Create(relPath)
						data, _ := os.ReadFile(path)
						f.Write(data)
						return nil
					})
				}
				zw.Close()
				os.WriteFile(snapshotPath, buf.Bytes(), 0600)

				zr, err := zip.OpenReader(backupPath)
				if err != nil {
					return fmt.Sprintf("错误：无法读取备份: %v", err)
				}
				defer zr.Close()

				restored := 0
				for _, f := range zr.File {
					targetPath := filepath.Join(RootDir, f.Name)
					if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(RootDir)) {
						continue
					}
					os.MkdirAll(filepath.Dir(targetPath), 0700)
					rc, _ := f.Open()
					data, _ := io.ReadAll(rc)
					rc.Close()
					os.WriteFile(targetPath, data, 0600)
					restored++
				}
				return fmt.Sprintf("♻️ 已从备份 '%s' 恢复 (%d 个文件)\n  恢复前快照已保存至 snapshots/", name, restored)

			case "health":
				var sb strings.Builder
				sb.WriteString("🩺 健康检查报告\n")
				sb.WriteString("───\n")
				issues := 0

				sb.WriteString("📁 目录完整性:\n")
				for _, dir := range []string{"dna", "memories", "workspace", "backups"} {
					path := filepath.Join(RootDir, dir)
					if info, err := os.Stat(path); err == nil && info.IsDir() {
						sb.WriteString(fmt.Sprintf("  ✅ %s/\n", dir))
					} else {
						sb.WriteString(fmt.Sprintf("  ❌ %s/ — 缺失\n", dir))
						issues++
					}
				}

				sb.WriteString("📄 关键文件:\n")
				for _, f := range criticalFiles {
					if info, err := os.Stat(f); err == nil {
						sizeStr := formatBytes(fmt.Sprintf("%d", info.Size()))
						sb.WriteString(fmt.Sprintf("  ✅ %s (%s)\n", filepath.Base(f), sizeStr))
					} else {
						sb.WriteString(fmt.Sprintf("  ⚠️ %s — 缺失\n", filepath.Base(f)))
						issues++
					}
				}

				sb.WriteString("🔍 JSON 完整性:\n")
				jsonFiles := []string{
					filepath.Join(RootDir, "dna", "config.json"),
					filepath.Join(RootDir, "memories", "creator.json"),
					filepath.Join(RootDir, "workspace", "schedule.json"),
					filepath.Join(RootDir, "workspace", "notes.json"),
					filepath.Join(RootDir, "workspace", "contacts.json"),
					filepath.Join(RootDir, "workspace", "recurring.json"),
					filepath.Join(RootDir, "workspace", "reminders.json"),
					filepath.Join(RootDir, "workspace", "todo.json"),
				}
				for _, f := range jsonFiles {
					if data, err := os.ReadFile(f); err == nil {
						if json.Valid(data) {
							sb.WriteString(fmt.Sprintf("  ✅ %s\n", filepath.Base(f)))
						} else {
							sb.WriteString(fmt.Sprintf("  ❌ %s — JSON 格式损坏\n", filepath.Base(f)))
							issues++
						}
					}
				}

				backupCount := 0
				if files, err := os.ReadDir(backupDir); err == nil {
					for _, f := range files {
						if strings.HasSuffix(f.Name(), ".zip") {
							backupCount++
						}
					}
				}
				sb.WriteString(fmt.Sprintf("📦 备份: %d 个\n", backupCount))

				sb.WriteString("───\n")
				if issues == 0 {
					sb.WriteString("✅ 状态: 健康\n")
				} else {
					sb.WriteString(fmt.Sprintf("⚠️ 发现 %d 个问题，建议执行 self_heal\n", issues))
				}
				return sb.String()

			case "self_heal":
				var sb strings.Builder
				sb.WriteString("🩹 自愈修复报告\n")
				sb.WriteString("───\n")
				fixed := 0

				for _, dir := range []string{"dna", "memories", "workspace", "backups", "snapshots"} {
					path := filepath.Join(RootDir, dir)
					if err := os.MkdirAll(path, 0700); err == nil {
						if info, err := os.Stat(path); err == nil && info.IsDir() {
							sb.WriteString(fmt.Sprintf("  ✅ %s/ 已就绪\n", dir))
						}
					}
				}

				jsonFiles := []struct {
					path     string
					defaults string
				}{
					{filepath.Join(RootDir, "workspace", "schedule.json"), "[]"},
					{filepath.Join(RootDir, "workspace", "notes.json"), "[]"},
					{filepath.Join(RootDir, "workspace", "contacts.json"), "[]"},
					{filepath.Join(RootDir, "workspace", "recurring.json"), "[]"},
					{filepath.Join(RootDir, "workspace", "reminders.json"), "[]"},
					{filepath.Join(RootDir, "workspace", "todo.json"), "[]"},
				}
				for _, jf := range jsonFiles {
					if data, err := os.ReadFile(jf.path); err != nil || !json.Valid(data) {
						os.WriteFile(jf.path, []byte(jf.defaults), 0644)
						sb.WriteString(fmt.Sprintf("  🔧 %s — 已重置\n", filepath.Base(jf.path)))
						fixed++
					}
				}

				creatorPath := filepath.Join(RootDir, "memories", "creator.json")
				if data, err := os.ReadFile(creatorPath); err != nil || !json.Valid(data) {
					defaultCreator := `{"name":"造物主"}`
					os.WriteFile(creatorPath, []byte(defaultCreator), 0600)
					sb.WriteString("  🔧 creator.json — 已重置\n")
					fixed++
				}

				configPath := filepath.Join(RootDir, "dna", "config.json")
				if data, err := os.ReadFile(configPath); err != nil || !json.Valid(data) {
					defaultConfig := `{"api_key":"","api_base_url":"","model_name":""}`
					os.WriteFile(configPath, []byte(defaultConfig), 0600)
					sb.WriteString("  🔧 config.json — 已重置\n")
					fixed++
				}

				if fixed == 0 {
					sb.WriteString("  ✨ 一切正常，无需修复\n")
				} else {
					sb.WriteString(fmt.Sprintf("\n  🔧 共修复 %d 个问题\n", fixed))
				}
				return sb.String()

			case "auto_archive":
				archiveName := "snapshot_" + timestamp
				archivePath := filepath.Join(snapshotDir, archiveName+".zip")

				buf := new(bytes.Buffer)
				zw := zip.NewWriter(buf)
				for _, basePath := range criticalPaths {
					filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
						if err != nil || info.IsDir() {
							return nil
						}
						relPath, _ := filepath.Rel(RootDir, path)
						f, _ := zw.Create(relPath)
						data, _ := os.ReadFile(path)
						f.Write(data)
						return nil
					})
				}
				zw.Close()
				os.WriteFile(archivePath, buf.Bytes(), 0600)

				files, _ := os.ReadDir(snapshotDir)
				type snapInfo struct {
					name    string
					modTime time.Time
				}
				var snaps []snapInfo
				for _, f := range files {
					if !f.IsDir() && strings.HasSuffix(f.Name(), ".zip") {
						info, _ := f.Info()
						snaps = append(snaps, snapInfo{f.Name(), info.ModTime()})
					}
				}
				if len(snaps) > 10 {
					for i := 0; i < len(snaps); i++ {
						for j := i + 1; j < len(snaps); j++ {
							if snaps[j].modTime.Before(snaps[i].modTime) {
								snaps[i], snaps[j] = snaps[j], snaps[i]
							}
						}
					}
					removed := 0
					for i := 0; i < len(snaps)-10; i++ {
						os.Remove(filepath.Join(snapshotDir, snaps[i].name))
						removed++
					}
					return fmt.Sprintf("📦 自动存档完成: %s\n  已清理 %d 个旧快照，保留最近 10 个", archiveName, removed)
				}

				info, _ := os.Stat(archivePath)
				sizeStr := formatBytes(fmt.Sprintf("%d", info.Size()))
				return fmt.Sprintf("📦 自动存档完成: %s (%s)", archiveName, sizeStr)

			default:
				return "错误：action 参数应为 backup/list_backups/restore/health/self_heal/auto_archive"
			}
		},
	}
}
