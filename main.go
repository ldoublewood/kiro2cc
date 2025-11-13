package main

import (
	"bytes"
	"encoding/json"
	jsonStr "encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/bestk/kiro2cc/parser"
)

// TokenData 表示token文件的结构
type TokenData struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
}

// RefreshRequest 刷新token的请求结构
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// RefreshResponse 刷新token的响应结构
type RefreshResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    string `json:"expiresAt,omitempty"`
}

// AnthropicTool 表示 Anthropic API 的工具结构
type AnthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// InputSchema 表示工具输入模式的结构
type InputSchema struct {
	Json map[string]any `json:"json"`
}

// ToolSpecification 表示工具规范的结构
type ToolSpecification struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// CodeWhispererTool 表示 CodeWhisperer API 的工具结构
type CodeWhispererTool struct {
	ToolSpecification ToolSpecification `json:"toolSpecification"`
}

// HistoryUserMessage 表示历史记录中的用户消息
type HistoryUserMessage struct {
	UserInputMessage struct {
		Content string `json:"content"`
		ModelId string `json:"modelId"`
		Origin  string `json:"origin"`
	} `json:"userInputMessage"`
}

// HistoryAssistantMessage 表示历史记录中的助手消息
type HistoryAssistantMessage struct {
	AssistantResponseMessage struct {
		Content  string `json:"content"`
		ToolUses []any  `json:"toolUses"`
	} `json:"assistantResponseMessage"`
}

// AnthropicErrorResponse 表示 Anthropic API 的错误响应结构
type AnthropicErrorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// AnthropicRequest 表示 Anthropic API 的请求结构
type AnthropicRequest struct {
	Model       string                    `json:"model"`
	MaxTokens   int                       `json:"max_tokens"`
	Messages    []AnthropicRequestMessage `json:"messages"`
	System      []AnthropicSystemMessage  `json:"system,omitempty"`
	Tools       []AnthropicTool           `json:"tools,omitempty"`
	Stream      bool                      `json:"stream"`
	Temperature *float64                  `json:"temperature,omitempty"`
	Metadata    map[string]any            `json:"metadata,omitempty"`
}

// AnthropicStreamResponse 表示 Anthropic 流式响应的结构
type AnthropicStreamResponse struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentDelta struct {
		Text string `json:"text"`
		Type string `json:"type"`
	} `json:"delta,omitempty"`
	Content []struct {
		Text string `json:"text"`
		Type string `json:"type"`
	} `json:"content,omitempty"`
	StopReason   string `json:"stop_reason,omitempty"`
	StopSequence string `json:"stop_sequence,omitempty"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
}

// AnthropicRequestMessage 表示 Anthropic API 的消息结构
type AnthropicRequestMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // 可以是 string 或 []ContentBlock
}

type AnthropicSystemMessage struct {
	Type string `json:"type"`
	Text string `json:"text"` // 可以是 string 或 []ContentBlock
}

// ContentBlock 表示消息内容块的结构
type ContentBlock struct {
	Type      string  `json:"type"`
	Text      *string `json:"text,omitempty"`
	ToolUseId *string `json:"tool_use_id,omitempty"`
	Content   *string `json:"content,omitempty"`
	Name      *string `json:"name,omitempty"`
	Input     *any    `json:"input,omitempty"`
}

// getMessageContent 从消息中提取文本内容
func getMessageContent(content any) string {
	switch v := content.(type) {
	case string:
		if len(strings.TrimSpace(v)) == 0 {
			return "Please provide a response."
		}
		return v
	case []interface{}:
		var texts []string
		for _, block := range v {
			if m, ok := block.(map[string]interface{}); ok {
				var cb ContentBlock
				if data, err := jsonStr.Marshal(m); err == nil {
					if err := jsonStr.Unmarshal(data, &cb); err == nil {
						switch cb.Type {
						case "tool_result":
							if cb.Content != nil {
								texts = append(texts, *cb.Content)
							}
						case "text":
							if cb.Text != nil {
								texts = append(texts, *cb.Text)
							}
						case "tool_use":
							// Skip tool_use blocks for content extraction
							continue
						}
					}
				}
			}
		}
		if len(texts) == 0 {
			s, err := jsonStr.Marshal(content)
			if err != nil {
				return "Please provide a response."
			}
			log.Printf("Unhandled content format: %s", string(s))
			return "Please provide a response."
		}
		return strings.Join(texts, "\n")
	default:
		s, err := jsonStr.Marshal(content)
		if err != nil {
			return "Please provide a response."
		}
		log.Printf("Unhandled content type: %s", string(s))
		return "Please provide a response."
	}
}

// CodeWhispererRequest 表示 CodeWhisperer API 的请求结构
type CodeWhispererRequest struct {
	ConversationState struct {
		ChatTriggerType string `json:"chatTriggerType"`
		ConversationId  string `json:"conversationId"`
		CurrentMessage  struct {
			UserInputMessage struct {
				Content                 string `json:"content"`
				ModelId                 string `json:"modelId"`
				Origin                  string `json:"origin"`
				UserInputMessageContext struct {
					ToolResults []struct {
						Content []struct {
							Text string `json:"text"`
						} `json:"content"`
						Status    string `json:"status"`
						ToolUseId string `json:"toolUseId"`
					} `json:"toolResults,omitempty"`
					Tools []CodeWhispererTool `json:"tools,omitempty"`
				} `json:"userInputMessageContext"`
			} `json:"userInputMessage"`
		} `json:"currentMessage"`
		History []any `json:"history"`
	} `json:"conversationState"`
	ProfileArn string `json:"profileArn"`
}

// CodeWhispererEvent 表示 CodeWhisperer 的事件响应
type CodeWhispererEvent struct {
	ContentType string `json:"content-type"`
	MessageType string `json:"message-type"`
	Content     string `json:"content"`
	EventType   string `json:"event-type"`
}

var ModelMap = map[string]string{
	"claude-3-5-sonnet-20241022": "CLAUDE_3_5_SONNET_20241022_V2_0",
	"claude-3-5-sonnet-20240620": "CLAUDE_3_5_SONNET_20240620_V1_0",
	"claude-3-5-haiku-20241022":  "CLAUDE_3_5_HAIKU_20241022_V1_0",
	"claude-3-opus-20240229":     "CLAUDE_3_OPUS_20240229_V1_0",
	"claude-3-sonnet-20240229":   "CLAUDE_3_SONNET_20240229_V1_0",
	"claude-3-haiku-20240307":    "CLAUDE_3_HAIKU_20240307_V1_0",
	"claude-sonnet-4-20250514":   "CLAUDE_SONNET_4_20250514_V1_0",
}

// generateUUID generates a simple UUID v4
func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", 
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// buildCodeWhispererRequest 构建 CodeWhisperer 请求
func buildCodeWhispererRequest(anthropicReq AnthropicRequest) CodeWhispererRequest {
	// 使用环境变量或默认ProfileArn
	profileArn := os.Getenv("KIRO_PROFILE_ARN")
	if profileArn == "" {
		profileArn = "arn:aws:codewhisperer:us-east-1:699475941385:profile/EHGA3GRVQMUK"
	}
	
	cwReq := CodeWhispererRequest{
		ProfileArn: profileArn,
	}
	cwReq.ConversationState.ChatTriggerType = "MANUAL"
	cwReq.ConversationState.ConversationId = generateUUID()
	
	// 确保获取最后一条用户消息
	lastMessage := anthropicReq.Messages[len(anthropicReq.Messages)-1]
	content := getMessageContent(lastMessage.Content)
	
	// 确保内容不为空
	if strings.TrimSpace(content) == "" {
		content = "Please provide a response."
	}
	
	cwReq.ConversationState.CurrentMessage.UserInputMessage.Content = content
	cwReq.ConversationState.CurrentMessage.UserInputMessage.ModelId = ModelMap[anthropicReq.Model]
	cwReq.ConversationState.CurrentMessage.UserInputMessage.Origin = "AI_EDITOR"
	// 处理 tools 信息
	if len(anthropicReq.Tools) > 0 {
		var tools []CodeWhispererTool
		for _, tool := range anthropicReq.Tools {
			cwTool := CodeWhispererTool{}
			cwTool.ToolSpecification.Name = tool.Name
			cwTool.ToolSpecification.Description = tool.Description
			cwTool.ToolSpecification.InputSchema = InputSchema{
				Json: tool.InputSchema,
			}
			tools = append(tools, cwTool)
		}
		cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools = tools
	}

	// 构建历史消息
	// 先处理 system 消息或者常规历史消息
	if len(anthropicReq.System) > 0 || len(anthropicReq.Messages) > 1 {
		var history []any

		// 首先添加每个 system 消息作为独立的历史记录项

		assistantDefaultMsg := HistoryAssistantMessage{}
		assistantDefaultMsg.AssistantResponseMessage.Content = getMessageContent("I will follow these instructions")
		assistantDefaultMsg.AssistantResponseMessage.ToolUses = make([]any, 0)

		if len(anthropicReq.System) > 0 {
			for _, sysMsg := range anthropicReq.System {
				userMsg := HistoryUserMessage{}
				userMsg.UserInputMessage.Content = sysMsg.Text
				userMsg.UserInputMessage.ModelId = ModelMap[anthropicReq.Model]
				userMsg.UserInputMessage.Origin = "AI_EDITOR"
				history = append(history, userMsg)
				history = append(history, assistantDefaultMsg)
			}
		}

		// 然后处理常规消息历史
		for i := 0; i < len(anthropicReq.Messages)-1; i++ {
			if anthropicReq.Messages[i].Role == "user" {
				userMsg := HistoryUserMessage{}
				userMsg.UserInputMessage.Content = getMessageContent(anthropicReq.Messages[i].Content)
				userMsg.UserInputMessage.ModelId = ModelMap[anthropicReq.Model]
				userMsg.UserInputMessage.Origin = "AI_EDITOR"
				history = append(history, userMsg)

				// 检查下一条消息是否是助手回复
				if i+1 < len(anthropicReq.Messages)-1 && anthropicReq.Messages[i+1].Role == "assistant" {
					assistantMsg := HistoryAssistantMessage{}
					assistantMsg.AssistantResponseMessage.Content = getMessageContent(anthropicReq.Messages[i+1].Content)
					assistantMsg.AssistantResponseMessage.ToolUses = make([]any, 0)
					history = append(history, assistantMsg)
					i++ // 跳过已处理的助手消息
				}
			}
		}

		cwReq.ConversationState.History = history
	}

	return cwReq
}

var tokenFilePath string

func main() {
	// 定义命令行参数
	flag.StringVar(&tokenFilePath, "f", "", "指定token文件路径")
	
	// 自定义用法信息
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: %s [选项] <命令> [参数]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "选项:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n命令:\n")
		fmt.Fprintf(os.Stderr, "  read    - 读取并显示token\n")
		fmt.Fprintf(os.Stderr, "  refresh - 刷新token\n")
		fmt.Fprintf(os.Stderr, "  export  - 导出环境变量\n")
		fmt.Fprintf(os.Stderr, "  claude  - 跳过 claude 地区限制\n")
		fmt.Fprintf(os.Stderr, "  server [port] - 启动Anthropic API代理服务器 (默认端口: 8080)\n")
		fmt.Fprintf(os.Stderr, "\n示例:\n")
		fmt.Fprintf(os.Stderr, "  %s read\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -f /path/to/token.json refresh\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s server 9000\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nauthor: https://github.com/bestK/kiro2cc\n")
	}

	// 解析命令行参数
	flag.Parse()

	// 获取剩余的非flag参数
	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	command := args[0]

	switch command {
	case "read":
		readToken()
	case "refresh":
		refreshToken()
	case "export":
		exportEnvVars()
	case "claude":
		setClaude()
	case "server":
		port := "8080" // 默认端口
		if len(args) > 1 {
			port = args[1]
		}
		startServer(port)
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", command)
		flag.Usage()
		os.Exit(1)
	}
}

// getTokenFilePath 获取跨平台的token文件路径
func getTokenFilePath() string {
	// 如果通过 -f 参数指定了token文件路径，则使用指定的路径
	if tokenFilePath != "" {
		return tokenFilePath
	}

	// 否则使用默认路径
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("获取用户目录失败: %v\n", err)
		os.Exit(1)
	}

	return filepath.Join(homeDir, ".aws", "sso", "cache", "kiro-auth-token.json")
}

// readToken 读取并显示token信息
func readToken() {
	tokenPath := getTokenFilePath()

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		fmt.Printf("读取token文件失败: %v\n", err)
		os.Exit(1)
	}

	var token TokenData
	if err := jsonStr.Unmarshal(data, &token); err != nil {
		fmt.Printf("解析token文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Token信息:")
	fmt.Printf("Access Token: %s\n", token.AccessToken)
	fmt.Printf("Refresh Token: %s\n", token.RefreshToken)
	if token.ExpiresAt != "" {
		fmt.Printf("过期时间: %s\n", token.ExpiresAt)
	}
}

// refreshToken 刷新token
func refreshToken() {
	tokenPath := getTokenFilePath()

	// 读取当前token
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		fmt.Printf("读取token文件失败: %v\n", err)
		os.Exit(1)
	}

	var currentToken TokenData
	if err := jsonStr.Unmarshal(data, &currentToken); err != nil {
		fmt.Printf("解析token文件失败: %v\n", err)
		os.Exit(1)
	}

	// 准备刷新请求
	refreshReq := RefreshRequest{
		RefreshToken: currentToken.RefreshToken,
	}

	reqBody, err := jsonStr.Marshal(refreshReq)
	if err != nil {
		fmt.Printf("序列化请求失败: %v\n", err)
		os.Exit(1)
	}

	// 发送刷新请求
	resp, err := http.Post(
		"https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken",
		"application/json",
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		fmt.Printf("刷新token请求失败: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("刷新token失败，状态码: %d, 响应: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	// 解析响应
	var refreshResp RefreshResponse
	if err := jsonStr.NewDecoder(resp.Body).Decode(&refreshResp); err != nil {
		fmt.Printf("解析刷新响应失败: %v\n", err)
		os.Exit(1)
	}

	// 更新token文件
	newToken := TokenData(refreshResp)

	newData, err := jsonStr.MarshalIndent(newToken, "", "  ")
	if err != nil {
		fmt.Printf("序列化新token失败: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(tokenPath, newData, 0600); err != nil {
		fmt.Printf("写入token文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Token刷新成功!")
	fmt.Printf("新的Access Token: %s\n", newToken.AccessToken)
}

// refreshTokenSilently 静默刷新token，用于服务器内部调用
func refreshTokenSilently() error {
	tokenPath := getTokenFilePath()

	// 读取当前token
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return fmt.Errorf("读取token文件失败: %v", err)
	}

	var currentToken TokenData
	if err := jsonStr.Unmarshal(data, &currentToken); err != nil {
		return fmt.Errorf("解析token文件失败: %v", err)
	}

	// 准备刷新请求
	refreshReq := RefreshRequest{
		RefreshToken: currentToken.RefreshToken,
	}

	reqBody, err := jsonStr.Marshal(refreshReq)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %v", err)
	}

	// 发送刷新请求
	resp, err := http.Post(
		"https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken",
		"application/json",
		bytes.NewBuffer(reqBody),
	)
	if err != nil {
		return fmt.Errorf("刷新token请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("刷新token失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var refreshResp RefreshResponse
	if err := jsonStr.NewDecoder(resp.Body).Decode(&refreshResp); err != nil {
		return fmt.Errorf("解析刷新响应失败: %v", err)
	}

	// 更新token文件
	newToken := TokenData(refreshResp)
	newData, err := jsonStr.MarshalIndent(newToken, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化新token失败: %v", err)
	}

	if err := os.WriteFile(tokenPath, newData, 0600); err != nil {
		return fmt.Errorf("写入token文件失败: %v", err)
	}

	fmt.Printf("Token已静默刷新\n")
	return nil
}

// exportEnvVars 导出环境变量
func exportEnvVars() {
	tokenPath := getTokenFilePath()

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		fmt.Printf("读取 token失败,请先安装 Kiro 并登录！: %v\n", err)
		os.Exit(1)
	}

	var token TokenData
	if err := jsonStr.Unmarshal(data, &token); err != nil {
		fmt.Printf("解析token文件失败: %v\n", err)
		os.Exit(1)
	}

	// 根据操作系统输出不同格式的环境变量设置命令
	if runtime.GOOS == "windows" {
		fmt.Println("CMD")
		fmt.Printf("set ANTHROPIC_BASE_URL=http://localhost:8080\n")
		fmt.Printf("set ANTHROPIC_API_KEY=%s\n\n", token.AccessToken)
		fmt.Println("Powershell")
		fmt.Println(`$env:ANTHROPIC_BASE_URL="http://localhost:8080"`)
		fmt.Printf(`$env:ANTHROPIC_API_KEY="%s"`, token.AccessToken)
	} else {
		fmt.Printf("export ANTHROPIC_BASE_URL=http://localhost:8080\n")
		fmt.Printf("export ANTHROPIC_API_KEY=\"%s\"\n", token.AccessToken)
	}
}

func setClaude() {
	// C:\Users\WIN10\.claude.json
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("获取用户目录失败: %v\n", err)
		os.Exit(1)
	}

	claudeJsonPath := filepath.Join(homeDir, ".claude.json")
	ok, _ := FileExists(claudeJsonPath)
	if !ok {
		fmt.Println("未找到Claude配置文件，请确认是否已安装 Claude Code")
		fmt.Println("npm install -g @anthropic-ai/claude-code")
		os.Exit(1)
	}

	data, err := os.ReadFile(claudeJsonPath)
	if err != nil {
		fmt.Printf("读取 Claude 文件失败: %v\n", err)
		os.Exit(1)
	}

	var jsonData map[string]interface{}

	err = jsonStr.Unmarshal(data, &jsonData)

	if err != nil {
		fmt.Printf("解析 JSON 文件失败: %v\n", err)
		os.Exit(1)
	}

	jsonData["hasCompletedOnboarding"] = true
	jsonData["kiro2cc"] = true

	newJson, err := json.MarshalIndent(jsonData, "", "  ")

	if err != nil {
		fmt.Printf("生成 JSON 文件失败: %v\n", err)
		os.Exit(1)
	}

	err = os.WriteFile(claudeJsonPath, newJson, 0644)

	if err != nil {
		fmt.Printf("写入 JSON 文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Claude 配置文件已更新")

}

// getToken 获取当前token
func getToken() (TokenData, error) {
	tokenPath := getTokenFilePath()

	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return TokenData{}, fmt.Errorf("读取token文件失败: %v", err)
	}

	var token TokenData
	if err := jsonStr.Unmarshal(data, &token); err != nil {
		return TokenData{}, fmt.Errorf("解析token文件失败: %v", err)
	}

	return token, nil
}

// logMiddleware 记录所有HTTP请求的中间件
func logMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		// fmt.Printf("\n=== 收到请求 ===\n")
		// fmt.Printf("时间: %s\n", startTime.Format("2006-01-02 15:04:05"))
		// fmt.Printf("请求方法: %s\n", r.Method)
		// fmt.Printf("请求路径: %s\n", r.URL.Path)
		// fmt.Printf("客户端IP: %s\n", r.RemoteAddr)
		// fmt.Printf("请求头:\n")
		// for name, values := range r.Header {
		// 	fmt.Printf("  %s: %s\n", name, strings.Join(values, ", "))
		// }

		// 调用下一个处理器
		next(w, r)

		// 计算处理时间
		duration := time.Since(startTime)
		fmt.Printf("处理时间: %v\n", duration)
		fmt.Printf("=== 请求结束 ===\n\n")
	}
}

// startServer 启动HTTP代理服务器
func startServer(port string) {
	// 创建路由器
	mux := http.NewServeMux()

	// 注册所有端点
	mux.HandleFunc("/v1/messages", logMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// 只处理POST请求
		if r.Method != http.MethodPost {
			fmt.Printf("错误: 不支持的请求方法\n")
			http.Error(w, "只支持POST请求", http.StatusMethodNotAllowed)
			return
		}

		// 获取当前token
		token, err := getToken()
		if err != nil {
			fmt.Printf("错误: 获取token失败: %v\n", err)
			sendJSONError(w, http.StatusInternalServerError, "authentication_error", fmt.Sprintf("获取token失败: %v", err))
			return
		}
		
		// 验证token不为空
		if strings.TrimSpace(token.AccessToken) == "" {
			fmt.Printf("错误: AccessToken为空\n")
			sendJSONError(w, http.StatusUnauthorized, "authentication_error", "AccessToken为空，请先登录或刷新token")
			return
		}

		// 限制请求体大小 (10MB)
		r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
		
		// 读取请求体
		body, err := io.ReadAll(r.Body)
		if err != nil {
			fmt.Printf("错误: 读取请求体失败: %v\n", err)
			sendJSONError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("读取请求体失败: %v", err))
			return
		}
		defer r.Body.Close()
		
		// 验证请求体不为空
		if len(body) == 0 {
			sendJSONError(w, http.StatusBadRequest, "invalid_request_error", "请求体不能为空")
			return
		}

		fmt.Printf("\n=========================Anthropic 请求体:\n%s\n=======================================\n", string(body))
		
		// 验证JSON格式
		var testJson map[string]interface{}
		if err := jsonStr.Unmarshal(body, &testJson); err != nil {
			fmt.Printf("错误: 请求体不是有效的JSON: %v\n", err)
			sendJSONError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("请求体不是有效的JSON: %v", err))
			return
		}

		// 解析 Anthropic 请求
		var anthropicReq AnthropicRequest
		if err := jsonStr.Unmarshal(body, &anthropicReq); err != nil {
			fmt.Printf("错误: 解析请求体失败: %v\n", err)
			http.Error(w, fmt.Sprintf("解析请求体失败: %v", err), http.StatusBadRequest)
			return
		}

		// 基础校验，给出明确的错误提示
		if anthropicReq.Model == "" {
			sendJSONError(w, http.StatusBadRequest, "invalid_request_error", "Missing required field: model")
			return
		}
		if len(anthropicReq.Messages) == 0 {
			sendJSONError(w, http.StatusBadRequest, "invalid_request_error", "Missing required field: messages")
			return
		}
		if anthropicReq.MaxTokens <= 0 {
			sendJSONError(w, http.StatusBadRequest, "invalid_request_error", "max_tokens must be a positive integer")
			return
		}
		if _, ok := ModelMap[anthropicReq.Model]; !ok {
			// 提示可用的模型名称
			available := make([]string, 0, len(ModelMap))
			for k := range ModelMap {
				available = append(available, k)
			}
			sendJSONError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("Unknown or unsupported model: %s. Available models: %s", anthropicReq.Model, strings.Join(available, ", ")))
			return
		}
		
		// 验证消息格式
		for i, msg := range anthropicReq.Messages {
			if msg.Role != "user" && msg.Role != "assistant" {
				sendJSONError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("Invalid role '%s' in message %d. Must be 'user' or 'assistant'", msg.Role, i))
				return
			}
			if msg.Content == nil || (fmt.Sprintf("%v", msg.Content) == "" && fmt.Sprintf("%v", msg.Content) != "0") {
				sendJSONError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("Message %d has empty content", i))
				return
			}
		}

		// 如果是流式请求
		if anthropicReq.Stream {
			handleStreamRequest(w, anthropicReq, token.AccessToken)
			return
		}

		// 非流式请求处理
		handleNonStreamRequest(w, anthropicReq, token.AccessToken)
	}))

	// 添加健康检查端点
	mux.HandleFunc("/health", logMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	// 添加404处理
	mux.HandleFunc("/", logMiddleware(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("警告: 访问未知端点\n")
		http.Error(w, "404 未找到", http.StatusNotFound)
	}))

	// 启动服务器
	fmt.Printf("启动Anthropic API代理服务器，监听端口: %s\n", port)
	fmt.Printf("可用端点:\n")
	fmt.Printf("  POST /v1/messages - Anthropic API代理\n")
	fmt.Printf("  GET  /health      - 健康检查\n")
	fmt.Printf("按Ctrl+C停止服务器\n")

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		fmt.Printf("启动服务器失败: %v\n", err)
		os.Exit(1)
	}
}

// handleStreamRequest 处理流式请求
func handleStreamRequest(w http.ResponseWriter, anthropicReq AnthropicRequest, accessToken string) {
	// 设置SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	messageId := fmt.Sprintf("msg_%s", time.Now().Format("20060102150405"))

	// 构建 CodeWhisperer 请求
	cwReq := buildCodeWhispererRequest(anthropicReq)

	// 序列化请求体
	cwReqBody, err := jsonStr.Marshal(cwReq)
	if err != nil {
		sendErrorEvent(w, flusher, "序列化请求失败", err)
		return
	}

	fmt.Printf("\n=========================CodeWhisperer 流式请求体:\n%s\n=======================================\n", string(cwReqBody))

	// 创建流式请求
	proxyReq, err := http.NewRequest(
		http.MethodPost,
		"https://codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse",
		bytes.NewBuffer(cwReqBody),
	)
	if err != nil {
		sendErrorEvent(w, flusher, "创建代理请求失败", err)
		return
	}

	// 设置请求头
	proxyReq.Header.Set("Authorization", "Bearer "+accessToken)
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Accept", "text/event-stream")
	proxyReq.Header.Set("User-Agent", "kiro2cc/1.0")
	proxyReq.Header.Set("X-Amz-Target", "CodeWhispererStreaming_20220101.GenerateAssistantResponse")

	// 发送请求
	client := &http.Client{
		Timeout: 60 * time.Second, // 流式请求需要更长超时
	}

	resp, err := client.Do(proxyReq)
	if err != nil {
		sendErrorEvent(w, flusher, "CodeWhisperer request error", fmt.Errorf("request error: %s", err.Error()))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("CodeWhisperer 响应错误，状态码: %d, 响应: %s\n", resp.StatusCode, string(body))

		// 根据不同的状态码发送相应的错误事件
		switch resp.StatusCode {
		case 400:
			sendErrorEvent(w, flusher, "请求参数错误", fmt.Errorf("Bad Request: %s", string(body)))
		case 401:
			sendErrorEvent(w, flusher, "认证失败", fmt.Errorf("Unauthorized: 请检查token"))
		case 403:
			refreshToken()
			sendErrorEvent(w, flusher, "权限不足", fmt.Errorf("Forbidden: Token已刷新，请重试"))
		case 429:
			sendErrorEvent(w, flusher, "请求频率过高", fmt.Errorf("Rate Limited: 请稍后重试"))
		case 500:
			sendErrorEvent(w, flusher, "服务器内部错误", fmt.Errorf("Internal Server Error: CodeWhisperer服务异常"))
		case 502, 503, 504:
			sendErrorEvent(w, flusher, "服务不可用", fmt.Errorf("Service Unavailable: CodeWhisperer服务暂时不可用"))
		default:
			sendErrorEvent(w, flusher, "未知错误", fmt.Errorf("状态码: %d, 响应: %s", resp.StatusCode, string(body)))
		}
		return
	}

	// 先读取整个响应体
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		sendErrorEvent(w, flusher, "error", fmt.Errorf("CodeWhisperer Error 读取响应失败"))
		return
	}

	// os.WriteFile(messageId+"response.raw", respBody, 0644)

	// 使用新的CodeWhisperer解析器
	events := parser.ParseEvents(respBody)

	if len(events) > 0 {

		// 发送开始事件
		messageStart := map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":            messageId,
				"type":          "message",
				"role":          "assistant",
				"content":       []any{},
				"model":         anthropicReq.Model,
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage": map[string]any{
					"input_tokens":  len(getMessageContent(anthropicReq.Messages[0].Content)),
					"output_tokens": 1,
				},
			},
		}
		sendSSEEvent(w, flusher, "message_start", messageStart)
		sendSSEEvent(w, flusher, "ping", map[string]string{
			"type": "ping",
		})

		contentBlockStart := map[string]any{
			"content_block": map[string]any{
				"text": "",
				"type": "text"},
			"index": 0, "type": "content_block_start",
		}

		sendSSEEvent(w, flusher, "content_block_start", contentBlockStart)
		// 处理解析出的事件

		outputTokens := 0
		for _, e := range events {
			sendSSEEvent(w, flusher, e.Event, e.Data)

			if e.Event == "content_block_delta" {
				outputTokens = len(getMessageContent(e.Data))
			}

			// 随机延时
			time.Sleep(time.Duration(rand.Intn(300)) * time.Millisecond)
		}

		contentBlockStop := map[string]any{
			"index": 0,
			"type":  "content_block_stop",
		}
		sendSSEEvent(w, flusher, "content_block_stop", contentBlockStop)

		contentBlockStopReason := map[string]any{
			"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": map[string]any{
				"output_tokens": outputTokens,
			},
		}
		sendSSEEvent(w, flusher, "message_delta", contentBlockStopReason)

		messageStop := map[string]any{
			"type": "message_stop",
		}
		sendSSEEvent(w, flusher, "message_stop", messageStop)
	}

}

// handleNonStreamRequest 处理非流式请求
func handleNonStreamRequest(w http.ResponseWriter, anthropicReq AnthropicRequest, accessToken string) {
	// 构建 CodeWhisperer 请求
	cwReq := buildCodeWhispererRequest(anthropicReq)

	// 序列化请求体
	cwReqBody, err := jsonStr.Marshal(cwReq)
	if err != nil {
		fmt.Printf("错误: 序列化请求失败: %v\n", err)
		sendJSONError(w, http.StatusInternalServerError, "api_error", fmt.Sprintf("序列化请求失败: %v", err))
		return
	}

	fmt.Printf("\n=========================CodeWhisperer 请求体:\n%s\n=======================================\n", string(cwReqBody))

	// 创建请求
	proxyReq, err := http.NewRequest(
		http.MethodPost,
		"https://codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse",
		bytes.NewBuffer(cwReqBody),
	)
	if err != nil {
		fmt.Printf("错误: 创建代理请求失败: %v\n", err)
		sendJSONError(w, http.StatusInternalServerError, "api_error", fmt.Sprintf("创建代理请求失败: %v", err))
		return
	}

	// 设置请求头
	proxyReq.Header.Set("Authorization", "Bearer "+accessToken)
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("User-Agent", "kiro2cc/1.0")
	proxyReq.Header.Set("X-Amz-Target", "CodeWhispererStreaming_20220101.GenerateAssistantResponse")

	// 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(proxyReq)
	if err != nil {
		fmt.Printf("错误: 发送请求失败: %v\n", err)
		sendJSONError(w, http.StatusInternalServerError, "api_error", fmt.Sprintf("发送请求失败: %v", err))
		return
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("CodeWhisperer 响应错误，状态码: %d, 响应: %s\n", resp.StatusCode, string(body))

		// 根据不同的状态码返回相应的错误
		switch resp.StatusCode {
		case 400:
			sendJSONError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("请求参数错误: %s", string(body)))
		case 401:
			sendJSONError(w, http.StatusUnauthorized, "authentication_error", "认证失败，请检查token")
		case 403:
			// 尝试刷新token
			fmt.Printf("Token可能已过期，尝试刷新...\n")
			if refreshErr := refreshTokenSilently(); refreshErr == nil {
				sendJSONError(w, http.StatusForbidden, "permission_error", "Token已刷新，请重试请求")
			} else {
				sendJSONError(w, http.StatusForbidden, "permission_error", "权限不足且Token刷新失败，请重新登录")
			}
		case 429:
			sendJSONError(w, http.StatusTooManyRequests, "rate_limit_error", "请求频率过高，请稍后重试")
		case 500:
			sendJSONError(w, http.StatusInternalServerError, "api_error", "CodeWhisperer服务器内部错误")
		case 502, 503, 504:
			sendJSONError(w, http.StatusServiceUnavailable, "overloaded_error", "CodeWhisperer服务暂时不可用，请稍后重试")
		default:
			sendJSONError(w, resp.StatusCode, "api_error", fmt.Sprintf("CodeWhisperer返回错误，状态码: %d, 响应: %s", resp.StatusCode, string(body)))
		}
		return
	}

	// 读取响应
	cwRespBody, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("错误: 读取响应失败: %v\n", err)
		sendJSONError(w, http.StatusInternalServerError, "api_error", fmt.Sprintf("读取响应失败: %v", err))
		return
	}

	fmt.Printf("CodeWhisperer 响应体:\n%s\n", string(cwRespBody))

	events := parser.ParseEvents(cwRespBody)

	context := ""
	toolName := ""
	toolUseId := ""

	contexts := []map[string]any{}

	partialJsonStr := ""
	for _, event := range events {
		if event.Data != nil {
			if dataMap, ok := event.Data.(map[string]any); ok {
				switch dataMap["type"] {
				case "content_block_start":
					context = ""
				case "content_block_delta":
					if delta, ok := dataMap["delta"]; ok {

						if deltaMap, ok := delta.(map[string]any); ok {
							switch deltaMap["type"] {
							case "text_delta":
								if text, ok := deltaMap["text"]; ok {
									context += text.(string)
								}
							case "input_json_delta":
								toolUseId = deltaMap["id"].(string)
								toolName = deltaMap["name"].(string)
								if partial_json, ok := deltaMap["partial_json"]; ok {
									if strPtr, ok := partial_json.(*string); ok && strPtr != nil {
										partialJsonStr = partialJsonStr + *strPtr
									} else if str, ok := partial_json.(string); ok {
										partialJsonStr = partialJsonStr + str
									} else {
										log.Println("partial_json is not string or *string")
									}
								} else {
									log.Println("partial_json not found")
								}

							}
						}
					}

				case "content_block_stop":
					if index, ok := dataMap["index"]; ok {
						switch index {
						case 1:
							toolInput := map[string]interface{}{}
							if err := jsonStr.Unmarshal([]byte(partialJsonStr), &toolInput); err != nil {
								log.Printf("json unmarshal error:%s", err.Error())
							}

							contexts = append(contexts, map[string]interface{}{
								"type":  "tool_use",
								"id":    toolUseId,
								"name":  toolName,
								"input": toolInput,
							})
						case 0:
							contexts = append(contexts, map[string]interface{}{
								"text": context,
								"type": "text",
							})
						}
					}
				}

			}
		}
	}

	// 回退：如果已累积到文本但未收到 content_block_stop(index=0)，也要返回文本
	if len(contexts) == 0 && strings.TrimSpace(context) != "" {
		contexts = append(contexts, map[string]any{
			"type": "text",
			"text": context,
		})
	}

	// 检查是否是错误响应
	if strings.Contains(string(cwRespBody), "Improperly formed request.") {
		fmt.Printf("错误: CodeWhisperer返回格式错误: %s\n", string(cwRespBody))
		sendJSONError(w, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("请求格式错误: %s", string(cwRespBody)))
		return
	}

	// 构建 Anthropic 响应
	anthropicResp := map[string]any{
		"content":       contexts,
		"model":         anthropicReq.Model,
		"role":          "assistant",
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"type":          "message",
		"usage": map[string]any{
			"input_tokens":  len(cwReq.ConversationState.CurrentMessage.UserInputMessage.Content),
			"output_tokens": len(context),
		},
	}

	// 发送响应
	w.Header().Set("Content-Type", "application/json")
	jsonStr.NewEncoder(w).Encode(anthropicResp)
}

// sendSSEEvent 发送 SSE 事件
func sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, eventType string, data any) {

	json, err := jsonStr.Marshal(data)
	if err != nil {
		return
	}

	fmt.Printf("event: %s\n", eventType)
	fmt.Printf("data: %v\n\n", string(json))

	fmt.Fprintf(w, "event: %s\n", eventType)
	fmt.Fprintf(w, "data: %s\n\n", string(json))
	flusher.Flush()

}

// sendErrorEvent 发送错误事件
func sendErrorEvent(w http.ResponseWriter, flusher http.Flusher, message string, err error) {
	errorResp := map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    "api_error",
			"message": fmt.Sprintf("%s: %v", message, err),
		},
	}

	sendSSEEvent(w, flusher, "error", errorResp)
}

// sendJSONError 发送JSON格式的错误响应
func sendJSONError(w http.ResponseWriter, statusCode int, errorType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResp := AnthropicErrorResponse{
		Type: "error",
	}
	errorResp.Error.Type = errorType
	errorResp.Error.Message = message

	jsonStr.NewEncoder(w).Encode(errorResp)
}

func FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil // 文件或文件夹存在
	}
	if os.IsNotExist(err) {
		return false, nil // 文件或文件夹不存在
	}
	return false, err // 其他错误
}
