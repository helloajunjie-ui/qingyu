package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func init() {
	Toolkit["fetch_url"] = Tool{
		Name:        "fetch_url",
		Description: "【Web 触角】获取网页内容。参数: url (完整网址)",
		Category:    "网络",
		Execute: func(args map[string]string) string {
			url := args["url"]
			if url == "" {
				return "错误：未提供 URL"
			}

			return CachedNetworkCall("fetch", url, func() string {
				client := &http.Client{Timeout: time.Duration(GetSettings().Timeouts.NetworkFetch) * time.Second}
				req, err := http.NewRequest("GET", url, nil)
				if err != nil {
					return fmt.Sprintf("请求构建失败: %v", err)
				}
				req.Header.Set("User-Agent", "Qingyu/1.0 (Ambient Agent)")

				resp, err := client.Do(req)
				if err != nil {
					return fmt.Sprintf("无法访问该网址: %v", err)
				}
				defer resp.Body.Close()

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return fmt.Sprintf("读取响应失败: %v", err)
				}

				content := string(body)
				if len(content) > 3000 {
					content = content[:3000] + "\n\n... (内容过长，已截断)"
				}
				return fmt.Sprintf("网址 [%s] 的内容 (HTTP %d):\n%s", url, resp.StatusCode, content)
			})
		},
	}

	Toolkit["web_search"] = Tool{
		Name:        "web_search",
		Description: "【网络搜索】通过搜索引擎获取实时信息。参数: q (搜索关键词)",
		Category:    "网络",
		Execute: func(args map[string]string) string {
			query := args["q"]
			if query == "" {
				return "错误：未提供搜索关键词"
			}

			return CachedNetworkCall("search", query, func() string {
				searchURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1", url.QueryEscape(query))

				client := &http.Client{Timeout: 10 * time.Second}
				req, _ := http.NewRequest("GET", searchURL, nil)
				req.Header.Set("User-Agent", "Qingyu/1.0")

				resp, err := client.Do(req)
				if err != nil {
					return fmt.Sprintf("搜索失败: %v", err)
				}
				defer resp.Body.Close()

				body, _ := io.ReadAll(resp.Body)
				var result struct {
					AbstractText  string `json:"AbstractText"`
					AbstractURL   string `json:"AbstractURL"`
					Answer        string `json:"Answer"`
					RelatedTopics []struct {
						Text     string `json:"Text"`
						FirstURL string `json:"FirstURL"`
					} `json:"RelatedTopics"`
				}
				json.Unmarshal(body, &result)

				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("搜索结果: %s\n\n", query))
				if result.Answer != "" {
					sb.WriteString(fmt.Sprintf("📌 %s\n\n", result.Answer))
				}
				if result.AbstractText != "" {
					sb.WriteString(fmt.Sprintf("📖 %s\n", result.AbstractText))
					if result.AbstractURL != "" {
						sb.WriteString(fmt.Sprintf("   %s\n", result.AbstractURL))
					}
					sb.WriteString("\n")
				}
				count := 0
				for _, topic := range result.RelatedTopics {
					if topic.Text != "" && count < 5 {
						sb.WriteString(fmt.Sprintf("• %s\n", topic.Text))
						count++
					}
				}
				if sb.Len() < 20 {
					sb.WriteString("(未找到结构化结果，建议使用 fetch_url 直接访问网页)\n")
				}
				return sb.String()
			})
		},
	}

	Toolkit["get_ip"] = Tool{
		Name:        "get_ip",
		Description: "【IP 查询】获取当前的公网 IP 地址。无需参数",
		Category:    "网络",
		Execute: func(args map[string]string) string {
			return CachedNetworkCall("ip", "global", func() string {
				client := &http.Client{Timeout: 10 * time.Second}
				resp, err := client.Get("https://api.ipify.org?format=json")
				if err != nil {
					return fmt.Sprintf("IP 查询失败: %v", err)
				}
				defer resp.Body.Close()

				body, _ := io.ReadAll(resp.Body)
				var result struct {
					IP string `json:"ip"`
				}
				json.Unmarshal(body, &result)
				if result.IP != "" {
					return fmt.Sprintf("🌐 公网 IP: %s", result.IP)
				}
				return "IP 查询失败"
			})
		},
	}

	Toolkit["translate"] = Tool{
		Name:        "translate",
		Description: "【文本翻译】将文本翻译成指定语言。参数: text (要翻译的文本), to (目标语言代码，如 en, zh, ja, fr, de, es)",
		Category:    "网络",
		Execute: func(args map[string]string) string {
			text := args["text"]
			to := args["to"]
			if text == "" {
				return "错误：未提供翻译文本"
			}
			if to == "" {
				to = "en"
			}

			// 源1: lingva.ml
			url1 := fmt.Sprintf("https://lingva.ml/api/v1/auto/%s/%s", url.QueryEscape(to), url.QueryEscape(text))
			if result := tryTranslateLingva(url1); result != "" {
				return fmt.Sprintf("📝 翻译 (%s → %s):\n%s", "auto", to, result)
			}

			// 源2: Google Translate 非官方接口
			url2 := fmt.Sprintf("https://translate.googleapis.com/translate_a/single?client=gtx&sl=auto&tl=%s&dt=t&q=%s", url.QueryEscape(to), url.QueryEscape(text))
			client := &http.Client{Timeout: 10 * time.Second}
			req, _ := http.NewRequest("GET", url2, nil)
			req.Header.Set("User-Agent", "Mozilla/5.0")
			if resp, err := client.Do(req); err == nil {
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				var result []interface{}
				if json.Unmarshal(body, &result) == nil && len(result) > 0 {
					if inner, ok := result[0].([]interface{}); ok && len(inner) > 0 {
						if item, ok := inner[0].([]interface{}); ok && len(item) > 0 {
							if trans, ok := item[0].(string); ok && trans != "" {
								return fmt.Sprintf("📝 翻译 (%s → %s):\n%s", "auto", to, trans)
							}
						}
					}
				}
			}

			return "翻译服务暂不可用（所有翻译源均失败）"
		},
	}

	Toolkit["get_weather"] = Tool{
		Name:        "get_weather",
		Description: "【天气查询】获取某个城市的天气信息。参数: city (城市名，如 Beijing, Shanghai, Tokyo)",
		Category:    "网络",
		Execute: func(args map[string]string) string {
			city := args["city"]
			if city == "" {
				city = "Beijing"
			}

			return CachedNetworkCall("weather", city, func() string {
				weatherURL := fmt.Sprintf("https://wttr.in/%s?format=%%l:+%%c+%%t,+%%w,+%%h,+%%p&lang=zh", url.QueryEscape(city))

				client := &http.Client{Timeout: 10 * time.Second}
				req, _ := http.NewRequest("GET", weatherURL, nil)
				req.Header.Set("User-Agent", "Qingyu/1.0")

				resp, err := client.Do(req)
				if err != nil {
					return fmt.Sprintf("天气查询失败: %v", err)
				}
				defer resp.Body.Close()

				body, _ := io.ReadAll(resp.Body)
				weather := strings.TrimSpace(string(body))
				if weather == "" {
					return fmt.Sprintf("未获取到 %s 的天气信息", city)
				}
				return fmt.Sprintf("🌤 %s 天气: %s", city, weather)
			})
		},
	}

	// ===== 【拓展工具集迭代】web_multi_search — 多引擎并发搜索 =====
	Toolkit["web_multi_search"] = Tool{
		Name:        "web_multi_search",
		Description: "【网络检索】多引擎并发搜索，聚合多个搜索结果。参数: query (搜索关键词), engines (引擎列表,逗号分隔: web/news/scholar,默认web), max_results (每引擎最大结果,默认5)。缓存TTL: 300s",
		Category:    "网络",
		Execute: func(args map[string]string) string {
			query := args["query"]
			if query == "" {
				return "❌ 请提供搜索关键词"
			}
			engines := args["engines"]
			if engines == "" {
				engines = "web"
			}
			maxResults := 5
			if n := args["max_results"]; n != "" {
				fmt.Sscanf(n, "%d", &maxResults)
			}
			if maxResults < 1 || maxResults > 20 {
				maxResults = 5
			}

			cacheKey := fmt.Sprintf("multi_search_%s_%s_%d", query, engines, maxResults)
			return CachedNetworkCall("multi_search", cacheKey, func() string {
				engineList := strings.Split(engines, ",")
				type result struct {
					engine string
					text   string
				}
				ch := make(chan result, len(engineList))

				for _, eng := range engineList {
					eng = strings.TrimSpace(eng)
					go func(engine string) {
						var searchURL string
						switch engine {
						case "web":
							searchURL = fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
						case "news":
							searchURL = fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s&iar=news", url.QueryEscape(query))
						case "scholar":
							searchURL = fmt.Sprintf("https://scholar.google.com/scholar?q=%s&hl=zh-CN", url.QueryEscape(query))
						default:
							searchURL = fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
						}

						client := &http.Client{Timeout: 10 * time.Second}
						req, _ := http.NewRequest("GET", searchURL, nil)
						req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
						resp, err := client.Do(req)
						if err != nil {
							ch <- result{engine, fmt.Sprintf("[%s] 请求失败: %v", engine, err)}
							return
						}
						defer resp.Body.Close()
						body, _ := io.ReadAll(resp.Body)
						text := string(body)

						// 简单提取链接和标题
						re := regexp.MustCompile(`<a[^>]+href="(https?://[^"]+)"[^>]*>([^<]+)</a>`)
						matches := re.FindAllStringSubmatch(text, maxResults)
						var sb strings.Builder
						sb.WriteString(fmt.Sprintf("【%s】搜索结果:\n", engine))
						count := 0
						for _, m := range matches {
							if count >= maxResults {
								break
							}
							title := strings.TrimSpace(m[2])
							link := m[1]
							if title != "" && !strings.Contains(link, "duckduckgo.com") {
								sb.WriteString(fmt.Sprintf("  %d. %s\n    %s\n", count+1, title, link))
								count++
							}
						}
						if count == 0 {
							sb.WriteString("  无结果\n")
						}
						ch <- result{engine, sb.String()}
					}(eng)
				}

				var allResults strings.Builder
				allResults.WriteString(fmt.Sprintf("🔍 多引擎搜索「%s」结果:\n\n", query))
				for i := 0; i < len(engineList); i++ {
					r := <-ch
					allResults.WriteString(r.text)
					allResults.WriteString("\n")
				}

				resultStr := allResults.String()
				if len([]rune(resultStr)) > 3000 {
					resultStr = string([]rune(resultStr)[:3000]) + "\n\n... (结果过长，已截断)"
				}
				return resultStr
			})
		},
	}

	// ===== 【拓展工具集迭代】web_deep_extract — 深度页面内容提取 =====
	Toolkit["web_deep_extract"] = Tool{
		Name:        "web_deep_extract",
		Description: "【网络检索】深度提取指定URL的正文内容，自动去除导航/广告。参数: url (目标网址), max_chars (最大字符,默认5000)。缓存TTL: 600s",
		Category:    "网络",
		Execute: func(args map[string]string) string {
			targetURL := args["url"]
			if targetURL == "" {
				return "❌ 请提供目标URL"
			}
			maxChars := 5000
			if n := args["max_chars"]; n != "" {
				fmt.Sscanf(n, "%d", &maxChars)
			}
			if maxChars < 100 || maxChars > 50000 {
				maxChars = 5000
			}

			cacheKey := fmt.Sprintf("deep_extract_%s_%d", targetURL, maxChars)
			return CachedNetworkCall("deep_extract", cacheKey, func() string {
				client := &http.Client{Timeout: 15 * time.Second}
				req, _ := http.NewRequest("GET", targetURL, nil)
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
				resp, err := client.Do(req)
				if err != nil {
					return fmt.Sprintf("❌ 请求失败: %v", err)
				}
				defer resp.Body.Close()

				body, _ := io.ReadAll(resp.Body)
				html := string(body)

				// 提取 <title>
				title := ""
				re := regexp.MustCompile(`<title>([^<]+)</title>`)
				if m := re.FindStringSubmatch(html); len(m) > 1 {
					title = strings.TrimSpace(m[1])
				}

				// 提取 <meta name="description">
				desc := ""
				reDesc := regexp.MustCompile(`<meta[^>]+name="description"[^>]+content="([^"]+)"`)
				if m := reDesc.FindStringSubmatch(html); len(m) > 1 {
					desc = strings.TrimSpace(m[1])
				}

				// 移除 script/style 标签
				reScript := regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`)
				html = reScript.ReplaceAllString(html, "")
				reStyle := regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`)
				html = reStyle.ReplaceAllString(html, "")
				// 移除 HTML 标签
				reTag := regexp.MustCompile(`<[^>]+>`)
				text := reTag.ReplaceAllString(html, " ")
				// 合并空白
				reSpace := regexp.MustCompile(`\s+`)
				text = strings.TrimSpace(reSpace.ReplaceAllString(text, " "))

				var sb strings.Builder
				if title != "" {
					sb.WriteString(fmt.Sprintf("📄 %s\n\n", title))
				}
				if desc != "" {
					sb.WriteString(fmt.Sprintf("📝 %s\n\n", desc))
				}
				sb.WriteString(text)

				resultStr := sb.String()
				if len([]rune(resultStr)) > maxChars {
					resultStr = string([]rune(resultStr)[:maxChars]) + "\n\n... (内容过长，已截断)"
				}
				return resultStr
			})
		},
	}

	// ===== 【拓展工具集迭代】web_link_parse — 链接解析与预览 =====
	Toolkit["web_link_parse"] = Tool{
		Name:        "web_link_parse",
		Description: "【网络检索】解析URL链接，提取元信息（标题/描述/图标/链接数）。参数: url (目标网址)。缓存TTL: 600s",
		Category:    "网络",
		Execute: func(args map[string]string) string {
			targetURL := args["url"]
			if targetURL == "" {
				return "❌ 请提供目标URL"
			}

			cacheKey := fmt.Sprintf("link_parse_%s", targetURL)
			return CachedNetworkCall("link_parse", cacheKey, func() string {
				client := &http.Client{Timeout: 10 * time.Second}
				req, _ := http.NewRequest("GET", targetURL, nil)
				req.Header.Set("User-Agent", "Mozilla/5.0")
				resp, err := client.Do(req)
				if err != nil {
					return fmt.Sprintf("❌ 请求失败: %v", err)
				}
				defer resp.Body.Close()

				body, _ := io.ReadAll(resp.Body)
				html := string(body)

				// 提取 title
				title := ""
				re := regexp.MustCompile(`<title>([^<]+)</title>`)
				if m := re.FindStringSubmatch(html); len(m) > 1 {
					title = strings.TrimSpace(m[1])
				}

				// 提取 description
				desc := ""
				reDesc := regexp.MustCompile(`<meta[^>]+name="description"[^>]+content="([^"]+)"`)
				if m := reDesc.FindStringSubmatch(html); len(m) > 1 {
					desc = strings.TrimSpace(m[1])
				}

				// 提取 favicon
				icon := ""
				reIcon := regexp.MustCompile(`<link[^>]+rel="(?:shortcut )?icon"[^>]+href="([^"]+)"`)
				if m := reIcon.FindStringSubmatch(html); len(m) > 1 {
					icon = m[1]
					if !strings.HasPrefix(icon, "http") {
						u, _ := url.Parse(targetURL)
						icon = u.Scheme + "://" + u.Host + "/" + strings.TrimLeft(icon, "/")
					}
				}

				// 统计链接数
				reLink := regexp.MustCompile(`<a[^>]+href="(https?://[^"]+)"`)
				links := reLink.FindAllStringSubmatch(html, -1)
				uniqueLinks := make(map[string]bool)
				for _, l := range links {
					uniqueLinks[l[1]] = true
				}

				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("🔗 链接解析: %s\n", targetURL))
				if title != "" {
					sb.WriteString(fmt.Sprintf("  标题: %s\n", title))
				}
				if desc != "" {
					sb.WriteString(fmt.Sprintf("  描述: %s\n", desc))
				}
				if icon != "" {
					sb.WriteString(fmt.Sprintf("  图标: %s\n", icon))
				}
				sb.WriteString(fmt.Sprintf("  外链数: %d\n", len(uniqueLinks)))
				sb.WriteString(fmt.Sprintf("  内容长度: %d 字符\n", len(html)))
				return sb.String()
			})
		},
	}

	// ===== 【拓展工具集迭代】web_rss_read — RSS订阅源读取 =====
	Toolkit["web_rss_read"] = Tool{
		Name:        "web_rss_read",
		Description: "【网络检索】读取RSS/Atom订阅源，返回最新条目列表。参数: url (订阅源地址), max_items (最大条目数,默认10)。缓存TTL: 300s",
		Category:    "网络",
		Execute: func(args map[string]string) string {
			feedURL := args["url"]
			if feedURL == "" {
				return "❌ 请提供RSS订阅源URL"
			}
			maxItems := 10
			if n := args["max_items"]; n != "" {
				fmt.Sscanf(n, "%d", &maxItems)
			}
			if maxItems < 1 || maxItems > 50 {
				maxItems = 10
			}

			cacheKey := fmt.Sprintf("rss_read_%s_%d", feedURL, maxItems)
			return CachedNetworkCall("rss_read", cacheKey, func() string {
				client := &http.Client{Timeout: 10 * time.Second}
				req, _ := http.NewRequest("GET", feedURL, nil)
				req.Header.Set("User-Agent", "Mozilla/5.0")
				resp, err := client.Do(req)
				if err != nil {
					return fmt.Sprintf("❌ 读取RSS失败: %v", err)
				}
				defer resp.Body.Close()

				body, _ := io.ReadAll(resp.Body)
				xmlContent := string(body)

				// 简单解析 RSS 2.0 / Atom
				var items []string

				// 尝试 RSS 2.0
				reItem := regexp.MustCompile(`(?s)<item>.*?</item>`)
				rssItems := reItem.FindAllString(xmlContent, maxItems)
				for _, item := range rssItems {
					title := ""
					reTitle := regexp.MustCompile(`<title>(?:<!\[CDATA\[)?([^\]>]+)(?:\]\]>)?</title>`)
					if m := reTitle.FindStringSubmatch(item); len(m) > 1 {
						title = strings.TrimSpace(m[1])
					}
					link := ""
					reLink := regexp.MustCompile(`<link>(?:<!\[CDATA\[)?([^\]>]+)(?:\]\]>)?</link>`)
					if m := reLink.FindStringSubmatch(item); len(m) > 1 {
						link = strings.TrimSpace(m[1])
					}
					pubDate := ""
					reDate := regexp.MustCompile(`<pubDate>([^<]+)</pubDate>`)
					if m := reDate.FindStringSubmatch(item); len(m) > 1 {
						pubDate = strings.TrimSpace(m[1])
					}
					entry := title
					if pubDate != "" {
						entry = fmt.Sprintf("[%s] %s", pubDate, title)
					}
					if link != "" {
						entry += "\n    " + link
					}
					items = append(items, entry)
				}

				// 如果 RSS 2.0 没结果，尝试 Atom
				if len(items) == 0 {
					reEntry := regexp.MustCompile(`(?s)<entry>.*?</entry>`)
					atomEntries := reEntry.FindAllString(xmlContent, maxItems)
					for _, entry := range atomEntries {
						title := ""
						reTitle := regexp.MustCompile(`<title>(?:<!\[CDATA\[)?([^\]>]+)(?:\]\]>)?</title>`)
						if m := reTitle.FindStringSubmatch(entry); len(m) > 1 {
							title = strings.TrimSpace(m[1])
						}
						link := ""
						reLink := regexp.MustCompile(`<link[^>]+href="([^"]+)"`)
						if m := reLink.FindStringSubmatch(entry); len(m) > 1 {
							link = strings.TrimSpace(m[1])
						}
						updated := ""
						reUpdated := regexp.MustCompile(`<updated>([^<]+)</updated>`)
						if m := reUpdated.FindStringSubmatch(entry); len(m) > 1 {
							updated = strings.TrimSpace(m[1])
						}
						entryStr := title
						if updated != "" {
							entryStr = fmt.Sprintf("[%s] %s", updated, title)
						}
						if link != "" {
							entryStr += "\n    " + link
						}
						items = append(items, entryStr)
					}
				}

				if len(items) == 0 {
					return "📡 RSS源无可用条目"
				}

				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("📡 RSS订阅: %s\n\n", feedURL))
				for i, item := range items {
					sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
				}
				return sb.String()
			})
		},
	}

	// ===== 【拓展工具集迭代】web_image_analysis — 图片URL分析 =====
	Toolkit["web_image_analysis"] = Tool{
		Name:        "web_image_analysis",
		Description: "【网络检索】分析图片URL，获取图片元信息（尺寸/格式/大小）。参数: url (图片地址)。缓存TTL: 600s",
		Category:    "网络",
		Execute: func(args map[string]string) string {
			imageURL := args["url"]
			if imageURL == "" {
				return "❌ 请提供图片URL"
			}

			cacheKey := fmt.Sprintf("img_analysis_%s", imageURL)
			return CachedNetworkCall("img_analysis", cacheKey, func() string {
				client := &http.Client{Timeout: 10 * time.Second}
				req, _ := http.NewRequest("GET", imageURL, nil)
				req.Header.Set("User-Agent", "Mozilla/5.0")
				resp, err := client.Do(req)
				if err != nil {
					return fmt.Sprintf("❌ 获取图片失败: %v", err)
				}
				defer resp.Body.Close()

				contentType := resp.Header.Get("Content-Type")
				contentLength := resp.Header.Get("Content-Length")
				body, _ := io.ReadAll(resp.Body)

				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("🖼 图片分析: %s\n", imageURL))
				sb.WriteString(fmt.Sprintf("  格式: %s\n", contentType))
				if contentLength != "" {
					size := contentLength
					var cl int64
					if _, err := fmt.Sscanf(contentLength, "%d", &cl); err == nil && cl > 0 {
						if cl > 1024*1024 {
							size = fmt.Sprintf("%.1f MB", float64(cl)/(1024*1024))
						} else if cl > 1024 {
							size = fmt.Sprintf("%.1f KB", float64(cl)/1024)
						} else {
							size = fmt.Sprintf("%d B", cl)
						}
					}
					sb.WriteString(fmt.Sprintf("  大小: %s\n", size))
				}
				sb.WriteString(fmt.Sprintf("  实际数据: %d 字节\n", len(body)))

				// 尝试检测图片尺寸（仅对 PNG/JPEG/GIF）
				detectSize := func(data []byte) (int, int) {
					if len(data) < 24 {
						return 0, 0
					}
					// PNG
					if data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
						w := int(data[16])<<24 | int(data[17])<<16 | int(data[18])<<8 | int(data[19])
						h := int(data[20])<<24 | int(data[21])<<16 | int(data[22])<<8 | int(data[23])
						return w, h
					}
					// JPEG
					if data[0] == 0xFF && data[1] == 0xD8 {
						for i := 2; i < len(data)-9; i++ {
							if data[i] == 0xFF && data[i+1] == 0xC0 || data[i+1] == 0xC1 || data[i+1] == 0xC2 {
								h := int(data[i+5])<<8 | int(data[i+6])
								w := int(data[i+7])<<8 | int(data[i+8])
								return w, h
							}
						}
					}
					// GIF
					if data[0] == 'G' && data[1] == 'I' && data[2] == 'F' {
						w := int(data[6]) | int(data[7])<<8
						h := int(data[8]) | int(data[9])<<8
						return w, h
					}
					return 0, 0
				}

				w, h := detectSize(body)
				if w > 0 && h > 0 {
					sb.WriteString(fmt.Sprintf("  尺寸: %d x %d 像素\n", w, h))
				}

				return sb.String()
			})
		},
	}

	// ===== 【拓展工具集迭代】web_file_download_safe — 安全文件下载 =====
	Toolkit["web_file_download_safe"] = Tool{
		Name:        "web_file_download_safe",
		Description: "【网络检索】安全下载网络文件到本地沙盒目录，自动校验大小限制。参数: url (文件地址), save_name (保存文件名,可选)。缓存TTL: 0（不缓存）",
		Category:    "网络",
		Execute: func(args map[string]string) string {
			fileURL := args["url"]
			if fileURL == "" {
				return "❌ 请提供文件URL"
			}
			saveName := args["save_name"]
			if saveName == "" {
				// 从URL提取文件名
				parts := strings.Split(fileURL, "/")
				saveName = parts[len(parts)-1]
				if saveName == "" || strings.Contains(saveName, "?") {
					saveName = fmt.Sprintf("download_%x", sha256.Sum256([]byte(fileURL)))[:16]
				}
			}

			// 安全校验：只允许下载到 workspace/downloads/
			downloadDir := filepath.Join(RootDir, WorkspaceDir, "downloads")
			os.MkdirAll(downloadDir, 0755)
			savePath := filepath.Join(downloadDir, filepath.Base(saveName))

			// 大小限制检查
			maxSize := GetSettings().Behavior.WebDownloadMaxSize
			if maxSize <= 0 {
				maxSize = 50 * 1024 * 1024 // 默认 50MB
			}

			client := &http.Client{Timeout: 60 * time.Second}
			req, _ := http.NewRequest("GET", fileURL, nil)
			req.Header.Set("User-Agent", "Mozilla/5.0")
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Sprintf("❌ 下载失败: %v", err)
			}
			defer resp.Body.Close()

			// 检查 Content-Length
			contentLength := resp.Header.Get("Content-Length")
			var length int64
			if contentLength != "" {
				fmt.Sscanf(contentLength, "%d", &length)
				if length > int64(maxSize) {
					return fmt.Sprintf("❌ 文件过大 (%.1f MB)，超过限制 (%.1f MB)", float64(length)/(1024*1024), float64(maxSize)/(1024*1024))
				}
			}

			// 限制读取大小
			limitedReader := io.LimitReader(resp.Body, int64(maxSize)+1)
			data, err := io.ReadAll(limitedReader)
			if err != nil {
				return fmt.Sprintf("❌ 读取失败: %v", err)
			}
			if len(data) > maxSize {
				return fmt.Sprintf("❌ 文件超过大小限制 (%.1f MB)", float64(maxSize)/(1024*1024))
			}

			if err := os.WriteFile(savePath, data, 0644); err != nil {
				return fmt.Sprintf("❌ 保存失败: %v", err)
			}

			return fmt.Sprintf("✅ 文件已下载到: %s (%.1f KB)", savePath, float64(len(data))/1024)
		},
	}

	// ===== 【拓展工具集迭代】web_archive_save — 网页存档保存 =====
	Toolkit["web_archive_save"] = Tool{
		Name:        "web_archive_save",
		Description: "【网络检索】抓取网页内容并保存为本地存档文件。参数: url (目标网址), tag (分类标签,可选)。缓存TTL: 0（不缓存）",
		Category:    "网络",
		Execute: func(args map[string]string) string {
			targetURL := args["url"]
			if targetURL == "" {
				return "❌ 请提供目标URL"
			}
			tag := args["tag"]
			if tag == "" {
				tag = "general"
			}

			client := &http.Client{Timeout: 15 * time.Second}
			req, _ := http.NewRequest("GET", targetURL, nil)
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Sprintf("❌ 抓取失败: %v", err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			html := string(body)

			// 提取标题用于文件名
			title := "untitled"
			re := regexp.MustCompile(`<title>([^<]+)</title>`)
			if m := re.FindStringSubmatch(html); len(m) > 1 {
				title = strings.TrimSpace(m[1])
			}
			// 清理文件名非法字符
			title = strings.NewReplacer(
				"\\", "", "/", "", ":", "", "*", "", "?", "", "\"", "", "<", "", ">", "", "|", "",
			).Replace(title)
			if len([]rune(title)) > 50 {
				title = string([]rune(title)[:50])
			}

			// 保存到 workspace/archives/
			archiveDir := filepath.Join(RootDir, WorkspaceDir, "archives")
			os.MkdirAll(archiveDir, 0755)
			timestamp := time.Now().Format("20060102_150405")
			filename := fmt.Sprintf("%s_%s.html", timestamp, title)
			savePath := filepath.Join(archiveDir, filename)

			// 添加存档元信息头
			archiveContent := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>%s</title></head>
<body>
<!-- 青羽网页存档 -->
<!-- 存档时间: %s -->
<!-- 来源: %s -->
<!-- 标签: %s -->
<article>
%s
</article>
</body>
</html>`, title, time.Now().Format("2006-01-02 15:04:05"), targetURL, tag, html)

			if err := os.WriteFile(savePath, []byte(archiveContent), 0644); err != nil {
				return fmt.Sprintf("❌ 存档保存失败: %v", err)
			}

			return fmt.Sprintf("✅ 网页已存档: %s\n  来源: %s\n  大小: %.1f KB\n  标签: %s",
				savePath, targetURL, float64(len(archiveContent))/1024, tag)
		},
	}
}
