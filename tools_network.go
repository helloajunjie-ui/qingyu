package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
}
