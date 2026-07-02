package main

import (
	"archive/zip"
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func init() {
	Toolkit["get_time"] = Tool{
		Name:        "get_time",
		Description: "【时间查询】获取当前的日期和时间信息。无需参数",
		Category:    "实用",
		Execute: func(args map[string]string) string {
			now := time.Now()
			return fmt.Sprintf("当前时间: %s\n日期: %s\n时区: %s\nUnix时间戳: %d",
				now.Format("15:04:05"),
				now.Format("2006-01-02"),
				now.Format("MST"),
				now.Unix())
		},
	}

	Toolkit["calc"] = Tool{
		Name:        "calc",
		Description: "【数学计算】计算数学表达式。参数: expr (数学表达式，如 1+2*3, sqrt(16), 2^10)",
		Category:    "实用",
		Execute: func(args map[string]string) string {
			expr := args["expr"]
			if expr == "" {
				return "错误：未提供表达式"
			}

			re := regexp.MustCompile(`^[0-9+\-*/().,%^sqrt abs sin cos tan log ln pi e\s]+$`)
			if !re.MatchString(expr) {
				return "错误：表达式包含非法字符，只允许数学运算"
			}

			script := fmt.Sprintf("const expr = %s; try { console.log(eval(expr)); } catch(e) { console.log('Error: ' + e.message); }", expr)
			cmd := exec.Command("node", "-e", script)
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			if err := cmd.Run(); err == nil {
				result := strings.TrimSpace(stdout.String())
				if !strings.HasPrefix(result, "Error:") {
					return fmt.Sprintf("%s = %s", expr, result)
				}
			}

			cmd2 := exec.Command("python", "-c", fmt.Sprintf("import math; print(eval(%s))", expr))
			cmd2.Stdout = &stdout
			cmd2.Stderr = &stderr
			stdout.Reset()
			stderr.Reset()

			if err := cmd2.Run(); err == nil {
				result := strings.TrimSpace(stdout.String())
				return fmt.Sprintf("%s = %s", expr, result)
			}

			return fmt.Sprintf("无法计算表达式: %s (需要安装 node.js 或 python)", expr)
		},
	}

	Toolkit["uuid"] = Tool{
		Name:        "uuid",
		Description: "【UUID 生成】生成一个随机的 UUID v4。无需参数",
		Category:    "实用",
		Execute: func(args map[string]string) string {
			uuid := make([]byte, 16)
			rand.Read(uuid)
			uuid[6] = (uuid[6] & 0x0f) | 0x40
			uuid[8] = (uuid[8] & 0x3f) | 0x80
			return fmt.Sprintf("%s-%s-%s-%s-%s",
				hex.EncodeToString(uuid[0:4]),
				hex.EncodeToString(uuid[4:6]),
				hex.EncodeToString(uuid[6:8]),
				hex.EncodeToString(uuid[8:10]),
				hex.EncodeToString(uuid[10:16]))
		},
	}

	Toolkit["hash"] = Tool{
		Name:        "hash",
		Description: "【哈希计算】计算文本或文件的哈希值。参数: text (要哈希的文本), file (可选，文件路径，与 text 二选一), algorithm (可选，md5/sha256，默认 sha256)",
		Category:    "安全",
		Execute: func(args map[string]string) string {
			algo := args["algorithm"]
			if algo == "" {
				algo = "sha256"
			}
			var data []byte
			if file := args["file"]; file != "" {
				path := file
				if !filepath.IsAbs(path) {
					path = filepath.Join(RootDir, path)
				}
				var err error
				data, err = os.ReadFile(path)
				if err != nil {
					return fmt.Sprintf("读取文件失败: %v", err)
				}
			} else if text := args["text"]; text != "" {
				data = []byte(text)
			} else {
				return "错误：请提供 text 或 file 参数"
			}
			switch algo {
			case "md5":
				h := md5.Sum(data)
				return fmt.Sprintf("MD5: %x", h)
			case "sha256":
				h := sha256.Sum256(data)
				return fmt.Sprintf("SHA256: %x", h)
			default:
				return fmt.Sprintf("不支持的算法: %s (支持: md5, sha256)", algo)
			}
		},
	}

	Toolkit["base64"] = Tool{
		Name:        "base64",
		Description: "【Base64 编解码】对文本进行 Base64 编码或解码。参数: text (要处理的文本), mode (encode/decode，默认 encode)",
		Category:    "安全",
		Execute: func(args map[string]string) string {
			text := args["text"]
			mode := args["mode"]
			if text == "" {
				return "错误：未提供文本"
			}
			if mode == "" {
				mode = "encode"
			}
			switch mode {
			case "encode":
				return fmt.Sprintf("Base64 编码: %s", base64.StdEncoding.EncodeToString([]byte(text)))
			case "decode":
				data, err := base64.StdEncoding.DecodeString(text)
				if err != nil {
					return fmt.Sprintf("Base64 解码失败: %v", err)
				}
				return fmt.Sprintf("Base64 解码: %s", string(data))
			default:
				return "错误：mode 参数应为 encode 或 decode"
			}
		},
	}

	Toolkit["json_tool"] = Tool{
		Name:        "json_tool",
		Description: "【JSON 工具】格式化、压缩或验证 JSON 字符串。参数: text (JSON 字符串), mode (format/compress/validate，默认 format)",
		Category:    "编码",
		Execute: func(args map[string]string) string {
			text := args["text"]
			mode := args["mode"]
			if text == "" {
				return "错误：未提供 JSON 文本"
			}
			if mode == "" {
				mode = "format"
			}
			var parsed interface{}
			if err := json.Unmarshal([]byte(text), &parsed); err != nil {
				return fmt.Sprintf("JSON 解析失败: %v", err)
			}
			switch mode {
			case "format", "pretty":
				formatted, _ := json.MarshalIndent(parsed, "", "  ")
				return fmt.Sprintf("格式化 JSON:\n%s", string(formatted))
			case "compress", "minify":
				compressed, _ := json.Marshal(parsed)
				return fmt.Sprintf("压缩 JSON:\n%s", string(compressed))
			case "validate":
				return "✅ JSON 格式有效"
			default:
				return "错误：mode 参数应为 format/compress/validate"
			}
		},
	}

	Toolkit["csv_tool"] = Tool{
		Name:        "csv_tool",
		Description: "【CSV 工具】解析 CSV 文本为表格格式。参数: text (CSV 文本内容), has_header (可选，true/false，默认 true)",
		Category:    "编码",
		Execute: func(args map[string]string) string {
			text := args["text"]
			if text == "" {
				return "错误：未提供 CSV 文本"
			}
			hasHeader := args["has_header"] != "false"
			reader := csv.NewReader(strings.NewReader(text))
			records, err := reader.ReadAll()
			if err != nil {
				return fmt.Sprintf("CSV 解析失败: %v", err)
			}
			if len(records) == 0 {
				return "CSV 内容为空"
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("📊 CSV 表格 (%d 行 x %d 列)\n", len(records), len(records[0])))
			sb.WriteString("───\n")
			start := 0
			if hasHeader && len(records) > 0 {
				sb.WriteString(fmt.Sprintf("【表头】%s\n", strings.Join(records[0], " | ")))
				sb.WriteString("───\n")
				start = 1
			}
			for i := start; i < len(records); i++ {
				sb.WriteString(fmt.Sprintf("第 %d 行: %s\n", i-start+1, strings.Join(records[i], " | ")))
				if i-start+1 >= 20 {
					sb.WriteString(fmt.Sprintf("... 还有 %d 行\n", len(records)-i-1))
					break
				}
			}
			return sb.String()
		},
	}

	Toolkit["gen_password"] = Tool{
		Name:        "gen_password",
		Description: "【密码生成】生成安全的随机密码。参数: length (可选，长度，默认 16), use_special (可选，是否包含特殊字符，默认 true)",
		Category:    "安全",
		Execute: func(args map[string]string) string {
			length := 16
			if l := args["length"]; l != "" {
				if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 128 {
					length = n
				}
			}
			useSpecial := args["use_special"] != "false"
			lower := "abcdefghijklmnopqrstuvwxyz"
			upper := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
			digits := "0123456789"
			special := "!@#$%^&*()-_=+[]{}<>?"
			charset := lower + upper + digits
			if useSpecial {
				charset += special
			}
			var password strings.Builder
			for i := 0; i < length; i++ {
				n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
				password.WriteByte(charset[n.Int64()])
			}
			result := password.String()
			return fmt.Sprintf("🔑 生成密码 (%d 位):\n%s\n强度: %s", length, result,
				map[bool]string{true: "强", false: "中"}[length >= 12 && useSpecial])
		},
	}

	Toolkit["color_tool"] = Tool{
		Name:        "color_tool",
		Description: "【颜色工具】颜色格式转换 (HEX/RGB/HSL)。参数: value (颜色值，如 #ff0000 或 rgb(255,0,0)), format (目标格式: hex/rgb/hsl)",
		Category:    "编码",
		Execute: func(args map[string]string) string {
			value := args["value"]
			format := args["format"]
			if value == "" {
				return "错误：未提供颜色值"
			}
			if format == "" {
				format = "rgb"
			}
			hexToRGB := func(hexStr string) (r, g, b int, ok bool) {
				hexStr = strings.TrimPrefix(hexStr, "#")
				if len(hexStr) == 3 {
					hexStr = string([]byte{hexStr[0], hexStr[0], hexStr[1], hexStr[1], hexStr[2], hexStr[2]})
				}
				if len(hexStr) != 6 {
					return 0, 0, 0, false
				}
				data, err := hex.DecodeString(hexStr)
				if err != nil {
					return 0, 0, 0, false
				}
				return int(data[0]), int(data[1]), int(data[2]), true
			}
			rgbToHSL := func(r, g, b int) (h, s, l float64) {
				rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
				max, min := mathMaxFloat(mathMaxFloat(rf, gf), bf), mathMinFloat(mathMinFloat(rf, gf), bf)
				l = (max + min) / 2
				if max == min {
					return 0, 0, l
				}
				d := max - min
				if l > 0.5 {
					s = d / (2 - max - min)
				} else {
					s = d / (max + min)
				}
				switch max {
				case rf:
					h = (gf - bf) / d
					if gf < bf {
						h += 6
					}
				case gf:
					h = (bf-rf)/d + 2
				case bf:
					h = (rf-gf)/d + 4
				}
				h /= 6
				return h * 360, s * 100, l * 100
			}
			var r, g, b int
			var ok bool
			if strings.HasPrefix(value, "#") {
				r, g, b, ok = hexToRGB(value)
			} else if strings.HasPrefix(value, "rgb") {
				re := regexp.MustCompile(`(\d+)`)
				matches := re.FindAllString(value, 3)
				if len(matches) == 3 {
					r, _ = strconv.Atoi(matches[0])
					g, _ = strconv.Atoi(matches[1])
					b, _ = strconv.Atoi(matches[2])
					ok = true
				}
			}
			if !ok {
				return "错误：无法解析颜色值，请使用 #HEX 或 rgb(R,G,B) 格式"
			}
			h, s, l := rgbToHSL(r, g, b)
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("🎨 颜色转换结果:\n"))
			sb.WriteString(fmt.Sprintf("  HEX: #%02x%02x%02x\n", r, g, b))
			sb.WriteString(fmt.Sprintf("  RGB: rgb(%d, %d, %d)\n", r, g, b))
			sb.WriteString(fmt.Sprintf("  HSL: hsl(%.0f, %.1f%%, %.1f%%)\n", h, s, l))
			return sb.String()
		},
	}

	Toolkit["zip_tool"] = Tool{
		Name:        "zip_tool",
		Description: "【压缩工具】创建或解压 ZIP 归档。参数: action (create/extract/list), source (源文件/目录路径), target (目标 zip 路径，仅 create/extract 需要)",
		Category:    "归档",
		Execute: func(args map[string]string) string {
			action := args["action"]
			source := args["source"]
			target := args["target"]
			if action == "" || source == "" {
				return "错误：请提供 action 和 source 参数"
			}
			if !filepath.IsAbs(source) {
				source = filepath.Join(RootDir, source)
			}
			switch action {
			case "list":
				reader, err := zip.OpenReader(source)
				if err != nil {
					return fmt.Sprintf("打开 zip 失败: %v", err)
				}
				defer reader.Close()
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("📦 ZIP 归档: %s\n", source))
				totalSize := int64(0)
				for _, f := range reader.File {
					sb.WriteString(fmt.Sprintf("  %s (%d 字节)\n", f.Name, f.UncompressedSize64))
					totalSize += int64(f.UncompressedSize64)
				}
				sb.WriteString(fmt.Sprintf("共 %d 个文件，原始大小: %d 字节", len(reader.File), totalSize))
				return sb.String()
			case "extract":
				reader, err := zip.OpenReader(source)
				if err != nil {
					return fmt.Sprintf("打开 zip 失败: %v", err)
				}
				defer reader.Close()
				if target == "" {
					target = filepath.Dir(source)
				}
				if !filepath.IsAbs(target) {
					target = filepath.Join(RootDir, target)
				}
				os.MkdirAll(target, 0755)
				count := 0
				for _, f := range reader.File {
					path := filepath.Join(target, f.Name)
					if f.FileInfo().IsDir() {
						os.MkdirAll(path, 0755)
						continue
					}
					os.MkdirAll(filepath.Dir(path), 0755)
					rc, err := f.Open()
					if err != nil {
						continue
					}
					outFile, _ := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
					io.Copy(outFile, rc)
					rc.Close()
					outFile.Close()
					count++
				}
				return fmt.Sprintf("解压完成: %d 个文件 → %s", count, target)
			case "create":
				if target == "" {
					return "错误：请提供 target 参数（目标 zip 路径）"
				}
				if !filepath.IsAbs(target) {
					target = filepath.Join(RootDir, target)
				}
				outFile, err := os.Create(target)
				if err != nil {
					return fmt.Sprintf("创建 zip 失败: %v", err)
				}
				defer outFile.Close()
				zw := zip.NewWriter(outFile)
				defer zw.Close()
				count := 0
				filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
					if err != nil || info.IsDir() {
						return nil
					}
					relPath, _ := filepath.Rel(source, path)
					w, err := zw.Create(relPath)
					if err != nil {
						return nil
					}
					data, _ := os.ReadFile(path)
					w.Write(data)
					count++
					return nil
				})
				return fmt.Sprintf("压缩完成: %d 个文件 → %s", count, target)
			default:
				return "错误：action 参数应为 create/extract/list"
			}
		},
	}
}
