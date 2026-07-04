package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ============================================
// 上下文摘要压缩引擎
// 用轻量模型对对话历史/思考记录做摘要，丢弃原始文本
// ============================================

// SummarizeRequest 摘要请求
type SummarizeRequest struct {
	Content     string `json:"content"`
	MaxWords    int    `json:"max_words,omitempty"`    // 目标摘要长度（词数）
	FocusPoints string `json:"focus_points,omitempty"` // 关注点提示
}

// SummarizeResult 摘要结果
type SummarizeResult struct {
	Summary   string `json:"summary"`
	KeyPoints string `json:"key_points,omitempty"` // 关键要点
	Emotion   string `json:"emotion,omitempty"`    // 情绪基调
}

// summarizeWithLightModel 使用轻量模型做摘要
// 如果未配置轻量模型，回退到主模型
func (a *App) summarizeWithLightModel(content string, maxWords int) string {
	if content == "" {
		return ""
	}

	// 如果内容很短，直接返回
	if len([]rune(content)) < 100 {
		return content
	}

	lightModel := GetSettings().Models.LightModel
	lightBaseURL := GetSettings().Models.LightBaseURL
	if lightBaseURL == "" {
		lightBaseURL = a.apiBaseURL
	}
	if lightModel == "" {
		lightModel = a.modelName // 回退到主模型
	}

	prompt := fmt.Sprintf(`请用简洁的中文总结以下内容的核心要点，控制在 %d 字以内。
只输出总结本身，不要额外说明。

内容：
%s`, maxWords, content)

	payload := map[string]interface{}{
		"model": lightModel,
		"messages": []map[string]string{
			{"role": "system", "content": "你是一个高效的摘要引擎。用最精炼的语言提取核心信息，不添加任何额外说明。"},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
		"max_tokens":  maxWords * 2,
	}

	result := a.callLLM(lightBaseURL, lightModel, payload)
	if result == "" {
		// 回退到简单截断
		runes := []rune(content)
		if len(runes) > maxWords*2 {
			return string(runes[:maxWords*2]) + "..."
		}
		return content
	}

	return strings.TrimSpace(result)
}

// SummarizeThinkingLog 对 thinking 日志做摘要压缩
// 在 autonomicLoop 每 5 轮调用
func (a *App) SummarizeThinkingLog(logContent string) string {
	if logContent == "" {
		return ""
	}

	return a.summarizeWithLightModel(logContent, 200)
}

// SummarizeMemoryContent 对长记忆内容做摘要（归档时使用）
func (a *App) SummarizeMemoryContent(content string) string {
	if content == "" {
		return ""
	}

	return a.summarizeWithLightModel(content, 150)
}

// ============================================
// 分层模型调度器
// ============================================

// ModelTier 模型层级
type ModelTier int

const (
	ModelTierLight ModelTier = iota // 轻量模型：简单工具调用、心跳自检、时间判断、日记记录
	ModelTierMain                   // 主模型：复杂人格修改、深度思考、对话
)

// GetModelForTier 根据任务层级返回对应的模型配置
func (a *App) GetModelForTier(tier ModelTier) (baseURL, modelName string) {
	s := GetSettings()

	switch tier {
	case ModelTierLight:
		baseURL = s.Models.LightBaseURL
		if baseURL == "" {
			baseURL = a.apiBaseURL
		}
		modelName = s.Models.LightModel
		if modelName == "" {
			modelName = a.modelName
		}
	default:
		baseURL = a.apiBaseURL
		modelName = a.modelName
	}
	return
}

// callLLM 底层 LLM 调用（不注入系统提示，纯 API 调用）
func (a *App) callLLM(baseURL, model string, payload map[string]interface{}) string {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return ""
	}

	// 规范化 base URL
	baseURL = normalizeBaseURL(baseURL)
	apiURL := baseURL + "/chat/completions"

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	if len(result.Choices) == 0 {
		return ""
	}

	return result.Choices[0].Message.Content
}

// callLLMWithBody 直接发送原始请求体（用于需要自定义参数的场景）
func (a *App) callLLMWithBody(baseURL, model string, body []byte) string {
	baseURL = normalizeBaseURL(baseURL)
	apiURL := baseURL + "/chat/completions"

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(body))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	respBody, _ := io.ReadAll(resp.Body)
	return string(respBody)
}
