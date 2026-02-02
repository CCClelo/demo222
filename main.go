package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/proxy"
)

// ============================================================================
// 配置
// ============================================================================

const (
	FirefoxUserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:128.0) Gecko/20100101 Firefox/128.0"
	FirefoxAcceptHTML     = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/png,image/svg+xml,*/*;q=0.8"
	FirefoxAcceptJSON     = "application/json, text/plain, */*"
	FirefoxAcceptLanguage = "zh-CN,zh;q=0.8,zh-TW;q=0.7,zh-HK;q=0.5,en-US;q=0.3,en;q=0.2"
	FirefoxAcceptEncoding = "gzip, deflate, br, zstd"
)

var ModelMapping = map[string]string{
	"gpt-5.2":              "openai/gpt-5.2",
	"claude-opus-4.5":      "anthropic/claude-opus-4.5",
	"claude-sonnet-4.5":    "anthropic/claude-sonnet-4.5",
	"gemini-3-pro-preview": "google/gemini-3-pro-preview",
}

// ============================================================================
// 日志工具
// ============================================================================

type Logger struct {
	requestCount uint64
	debugEnabled bool
}

var logger = &Logger{}

func (l *Logger) nextRequestID() string {
	id := atomic.AddUint64(&l.requestCount, 1)
	return fmt.Sprintf("REQ-%06d", id)
}

func logDebug(format string, args ...interface{}) {
	if logger.debugEnabled {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func logInfo(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

func logWarn(format string, args ...interface{}) {
	log.Printf("[WARN] "+format, args...)
}

func logError(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

// ============================================================================
// 数据结构
// ============================================================================

// Account 注册账户
type Account struct {
	Email        string
	Password     string
	SessionToken string
	Client       *http.Client
	ProxyIndex   int
	CreatedAt    time.Time
	LastUsedAt   time.Time
	mu           sync.Mutex
}

type GuestSession struct {
	Client     *http.Client
	ProxyIndex int
	CreatedAt  time.Time
	LastUsedAt time.Time
	mu         sync.Mutex
}

type ChatRequest struct {
	ID                     string  `json:"id"`
	Message                Message `json:"message"`
	SelectedChatModel      string  `json:"selectedChatModel"`
	SelectedVisibilityType string  `json:"selectedVisibilityType"`
}

type Message struct {
	Role  string        `json:"role"`
	Parts []MessagePart `json:"parts"`
	ID    string        `json:"id"`
}

type MessagePart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type OpenAIRequest struct {
	Model    string          `json:"model"`
	Messages []OpenAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type OpenAIMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type AnthropicRequest struct {
	Model    string           `json:"model"`
	Messages []AnthropicMessage `json:"messages"`
	Stream   bool             `json:"stream,omitempty"`
	MaxTokens int             `json:"max_tokens,omitempty"`
}

type AnthropicMessage struct {
	Role    string                  `json:"role"`
	Content []AnthropicContentBlock `json:"content"`
}

type AnthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// AnthropicMessageCompat 兼容两种消息格式
type AnthropicMessageCompat struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type SSEEvent struct {
	Type  string `json:"type"`
	Delta string `json:"delta,omitempty"`
}

// ============================================================================
// 代理管理
// ============================================================================

type ProxyManager struct {
	proxies      []string // 代理地址列表
	containers   []string // 对应的容器名
	currentIndex int
	mu           sync.Mutex
}

func NewProxyManager(proxiesStr, containersStr string) *ProxyManager {
	proxies := strings.Split(proxiesStr, ",")
	containers := strings.Split(containersStr, ",")

	// 过滤空值
	var validProxies, validContainers []string
	for i, p := range proxies {
		p = strings.TrimSpace(p)
		if p != "" {
			validProxies = append(validProxies, p)
			if i < len(containers) {
				validContainers = append(validContainers, strings.TrimSpace(containers[i]))
			}
		}
	}

	return &ProxyManager{
		proxies:      validProxies,
		containers:   validContainers,
		currentIndex: 0,
	}
}

func (pm *ProxyManager) GetCurrentProxy() (string, int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if len(pm.proxies) == 0 {
		return "", -1
	}
	return pm.proxies[pm.currentIndex], pm.currentIndex
}

func (pm *ProxyManager) OnRateLimit() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if len(pm.proxies) == 0 {
		return
	}

	oldIndex := pm.currentIndex
	oldContainer := pm.containers[oldIndex]

	// 切换到下一个代理
	pm.currentIndex = (pm.currentIndex + 1) % len(pm.proxies)
	logWarn("429 限流 | 切换代理: [%d]%s -> [%d]%s",
		oldIndex, oldContainer,
		pm.currentIndex, pm.containers[pm.currentIndex])

	// 异步重启被限流的 WARP 容器刷新 IP
	go pm.restartWarp(oldContainer)
}

func (pm *ProxyManager) restartWarp(container string) {
	if container == "" {
		return
	}
	logInfo("WARP 重启开始 | 容器: %s", container)
	cmd := exec.Command("docker", "restart", container)
	if err := cmd.Run(); err != nil {
		logError("WARP 重启失败 | 容器: %s, 错误: %v", container, err)
		return
	}
	// 等待 WARP 就绪
	time.Sleep(15 * time.Second)
	logInfo("WARP 重启完成 | 容器: %s", container)
}

// ============================================================================
// 网关
// ============================================================================

type Gateway struct {
	baseURL   string
	proxyMgr  *ProxyManager
	accounts  map[int]*Account      // 按代理索引存储注册账户
	sessions  map[int]*GuestSession // 按代理索引存储游客会话（备用）
	sessionMu sync.RWMutex
	useAuth   bool // 是否使用注册账户
}

func NewGateway(baseURL string, proxyMgr *ProxyManager, useAuth bool) *Gateway {
	return &Gateway{
		baseURL:  baseURL,
		proxyMgr: proxyMgr,
		accounts: make(map[int]*Account),
		sessions: make(map[int]*GuestSession),
		useAuth:  useAuth,
	}
}

func (g *Gateway) createHTTPClient(proxyURL string) (*http.Client, error) {
	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{}

	if proxyURL != "" {
		proxyParsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("解析代理失败: %w", err)
		}

		switch proxyParsed.Scheme {
		case "http", "https":
			transport.Proxy = http.ProxyURL(proxyParsed)
		case "socks5", "socks5h":
			auth := &proxy.Auth{}
			if proxyParsed.User != nil {
				auth.User = proxyParsed.User.Username()
				auth.Password, _ = proxyParsed.User.Password()
			}
			dialer, err := proxy.SOCKS5("tcp", proxyParsed.Host, auth, proxy.Direct)
			if err != nil {
				return nil, fmt.Errorf("创建 SOCKS5 失败: %w", err)
			}
			transport.Dial = dialer.Dial
		}
	}

	return &http.Client{
		Jar:       jar,
		Timeout:   60 * time.Second,
		Transport: transport,
	}, nil
}

// ============================================================================
// 账户注册
// ============================================================================

var firstNames = []string{"james", "john", "robert", "michael", "david", "william", "richard", "joseph", "thomas", "charles", "mary", "patricia", "jennifer", "linda", "elizabeth", "barbara", "susan", "jessica", "sarah", "karen"}
var lastNames = []string{"smith", "johnson", "williams", "brown", "jones", "garcia", "miller", "davis", "rodriguez", "martinez", "hernandez", "lopez", "gonzalez", "wilson", "anderson", "thomas", "taylor", "moore", "jackson", "martin"}

func generateEmail() string {
	first := firstNames[randInt(len(firstNames))]
	last := lastNames[randInt(len(lastNames))]
	num := randInt(9999)
	formats := []string{
		fmt.Sprintf("%s.%s%d@gmail.com", first, last, num),
		fmt.Sprintf("%s_%s%d@gmail.com", first, last, num),
		fmt.Sprintf("%s%s%d@gmail.com", first, last, num),
	}
	return formats[randInt(len(formats))]
}

func generatePassword() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	length := 16 + randInt(5)
	password := make([]byte, length)
	for i := range password {
		password[i] = chars[randInt(len(chars))]
	}
	return string(password)
}

func randInt(max int) int {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
	return int(n.Int64())
}

// register 注册新账户
func (g *Gateway) register(client *http.Client) (string, string, error) {
	email := generateEmail()
	password := generatePassword()

	// 构建 multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("1_email", email)
	writer.WriteField("1_password", password)
	writer.WriteField("0", `[{"status":"idle"},"$K1"]`)
	writer.Close()

	req, _ := http.NewRequest("POST", g.baseURL+"/register", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	setFirefoxHeaders(req, FirefoxAcceptHTML)

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("注册请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return "", "", fmt.Errorf("注册触发限流")
	}

	// 检查是否注册成功（通过 cookie）
	for _, cookie := range resp.Cookies() {
		if strings.Contains(cookie.Name, "session-token") {
			logInfo("注册成功 | 邮箱: %s", email)
			return email, password, nil
		}
	}

	// 检查 jar 中的 cookie
	u, _ := url.Parse(g.baseURL)
	for _, cookie := range client.Jar.Cookies(u) {
		if strings.Contains(cookie.Name, "session-token") {
			logInfo("注册成功 | 邮箱: %s", email)
			return email, password, nil
		}
	}

	return "", "", fmt.Errorf("注册失败，未获取到 session token")
}

// getOrCreateAccount 获取或创建注册账户
func (g *Gateway) getOrCreateAccount() (*Account, error) {
	proxyURL, proxyIndex := g.proxyMgr.GetCurrentProxy()

	g.sessionMu.Lock()
	defer g.sessionMu.Unlock()

	// 查找当前代理的账户
	if account, exists := g.accounts[proxyIndex]; exists {
		account.mu.Lock()
		account.LastUsedAt = time.Now()
		account.mu.Unlock()
		return account, nil
	}

	// 创建新账户
	containerName := ""
	if proxyIndex < len(g.proxyMgr.containers) {
		containerName = g.proxyMgr.containers[proxyIndex]
	}
	logInfo("账户注册中 | 代理: [%d]%s", proxyIndex, containerName)

	client, err := g.createHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}

	email, password, err := g.register(client)
	if err != nil {
		logError("账户注册失败 | 代理: [%d], 错误: %v", proxyIndex, err)
		if strings.Contains(err.Error(), "限流") {
			g.proxyMgr.OnRateLimit()
		}
		return nil, err
	}

	account := &Account{
		Email:      email,
		Password:   password,
		Client:     client,
		ProxyIndex: proxyIndex,
		CreatedAt:  time.Now(),
		LastUsedAt: time.Now(),
	}

	g.accounts[proxyIndex] = account
	logInfo("账户创建成功 | 代理: [%d]%s, 邮箱: %s", proxyIndex, containerName, email)
	return account, nil
}

func (g *Gateway) clearAccount(proxyIndex int) {
	g.sessionMu.Lock()
	defer g.sessionMu.Unlock()
	delete(g.accounts, proxyIndex)
}

func (g *Gateway) getOrCreateSession() (*GuestSession, error) {
	proxyURL, proxyIndex := g.proxyMgr.GetCurrentProxy()

	g.sessionMu.Lock()
	defer g.sessionMu.Unlock()

	// 查找当前代理的会话
	if session, exists := g.sessions[proxyIndex]; exists {
		session.mu.Lock()
		session.LastUsedAt = time.Now()
		session.mu.Unlock()
		return session, nil
	}

	// 创建新会话
	containerName := ""
	if proxyIndex >= 0 && proxyIndex < len(g.proxyMgr.containers) {
		containerName = g.proxyMgr.containers[proxyIndex]
	}
	logInfo("会话创建中 | 代理: [%d]%s", proxyIndex, containerName)
	client, err := g.createHTTPClient(proxyURL)
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequest("GET", g.baseURL+"/", nil)
	setFirefoxHeaders(req, FirefoxAcceptHTML)

	resp, err := client.Do(req)
	if err != nil {
		logError("会话创建失败 | 代理: [%d], 错误: %v", proxyIndex, err)
		return nil, fmt.Errorf("获取会话失败: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		g.proxyMgr.OnRateLimit()
		return nil, fmt.Errorf("触发限流")
	}

	session := &GuestSession{
		Client:     client,
		ProxyIndex: proxyIndex,
		CreatedAt:  time.Now(),
		LastUsedAt: time.Now(),
	}

	g.sessions[proxyIndex] = session
	logInfo("会话创建成功 | 代理: [%d]%s", proxyIndex, containerName)
	return session, nil
}

func (g *Gateway) clearSession(proxyIndex int) {
	g.sessionMu.Lock()
	defer g.sessionMu.Unlock()
	delete(g.sessions, proxyIndex)
}

func (g *Gateway) HandleChatCompletion(w http.ResponseWriter, r *http.Request) {
	reqID := logger.nextRequestID()
	startTime := time.Now()

	var openAIReq OpenAIRequest
	if err := json.NewDecoder(r.Body).Decode(&openAIReq); err != nil {
		logError("%s | 请求解析失败: %v", reqID, err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if len(openAIReq.Messages) == 0 {
		logError("%s | 消息为空", reqID)
		http.Error(w, "No messages found", http.StatusBadRequest)
		return
	}

	// 检测无意义请求
	if g.isMeaninglessRequest(openAIReq.Messages) {
		logInfo("%s | 无意义请求，直接返回 BAKA!", reqID)
		g.respondBAKA(w, openAIReq.Stream)
		return
	}

	// 统计消息
	msgCount := len(openAIReq.Messages)
	streamMode := "非流式"
	if openAIReq.Stream {
		streamMode = "流式"
	}
	logInfo("%s | 请求开始 | 模型: %s, 消息数: %d, 模式: %s", reqID, openAIReq.Model, msgCount, streamMode)

	// 详细记录每条消息的角色和长度
	for i, msg := range openAIReq.Messages {
		contentLen := 0
		if str, ok := msg.Content.(string); ok {
			contentLen = len(str)
		}
		logDebug("%s | 消息[%d] role=%s, len=%d", reqID, i, msg.Role, contentLen)
	}

	// 构建最终消息：拼接所有历史
	finalMessage := g.buildConversationMessage(openAIReq.Messages)
	logDebug("%s | 消息长度: %d 字符", reqID, len(finalMessage))

	chatModel := g.convertModel(openAIReq.Model)
	chatReq := ChatRequest{
		ID: uuid.New().String(),
		Message: Message{
			Role:  "user",
			Parts: []MessagePart{{Type: "text", Text: finalMessage}},
			ID:    uuid.New().String(),
		},
		SelectedChatModel:      chatModel,
		SelectedVisibilityType: "private",
	}

	// 根据模式选择账户或游客会话
	if g.useAuth {
		g.handleWithAccount(w, r, openAIReq, chatReq, reqID, startTime)
	} else {
		g.handleWithSession(w, r, openAIReq, chatReq, reqID, startTime)
	}
}

// handleWithAccount 使用注册账户处理请求
func (g *Gateway) handleWithAccount(w http.ResponseWriter, r *http.Request, openAIReq OpenAIRequest, chatReq ChatRequest, reqID string, startTime time.Time) {
	var account *Account
	var err error
	for retry := 0; retry < 3; retry++ {
		account, err = g.getOrCreateAccount()
		if err == nil {
			break
		}
		logWarn("%s | 获取账户失败 (重试 %d/3): %v", reqID, retry+1, err)
		time.Sleep(time.Second)
	}

	if account == nil {
		logError("%s | 服务不可用，所有重试失败", reqID)
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	containerName := ""
	if account.ProxyIndex >= 0 && account.ProxyIndex < len(g.proxyMgr.containers) {
		containerName = g.proxyMgr.containers[account.ProxyIndex]
	}
	logInfo("%s | 转发请求 | 代理: [%d]%s, 账户: %s, 目标模型: %s",
		reqID, account.ProxyIndex, containerName, account.Email, chatReq.SelectedChatModel)

	if openAIReq.Stream {
		g.handleStreamWithClient(w, account.Client, account.ProxyIndex, chatReq, reqID, startTime)
	} else {
		g.handleNonStreamWithClient(w, account.Client, account.ProxyIndex, chatReq, reqID, startTime)
	}
}

// handleWithSession 使用游客会话处理请求
func (g *Gateway) handleWithSession(w http.ResponseWriter, r *http.Request, openAIReq OpenAIRequest, chatReq ChatRequest, reqID string, startTime time.Time) {
	var session *GuestSession
	var err error
	for retry := 0; retry < 3; retry++ {
		session, err = g.getOrCreateSession()
		if err == nil {
			break
		}
		logWarn("%s | 获取会话失败 (重试 %d/3): %v", reqID, retry+1, err)
		time.Sleep(time.Second)
	}

	if session == nil {
		logError("%s | 服务不可用，所有重试失败", reqID)
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	containerName := ""
	if session.ProxyIndex >= 0 && session.ProxyIndex < len(g.proxyMgr.containers) {
		containerName = g.proxyMgr.containers[session.ProxyIndex]
	}
	logInfo("%s | 转发请求 | 代理: [%d]%s, 目标模型: %s",
		reqID, session.ProxyIndex, containerName, chatReq.SelectedChatModel)

	if openAIReq.Stream {
		g.handleStreamWithClient(w, session.Client, session.ProxyIndex, chatReq, reqID, startTime)
	} else {
		g.handleNonStreamWithClient(w, session.Client, session.ProxyIndex, chatReq, reqID, startTime)
	}
}

func (g *Gateway) handleStreamWithClient(w http.ResponseWriter, client *http.Client, proxyIndex int, chatReq ChatRequest, reqID string, startTime time.Time) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		logError("%s | 流式响应不支持", reqID)
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	reqBody, _ := json.Marshal(chatReq)
	// 记录请求字段（不含内容）
	logDebug("%s | 上游请求 | id=%s, model=%s, msgId=%s, textLen=%d",
		reqID, chatReq.ID, chatReq.SelectedChatModel, chatReq.Message.ID, len(chatReq.Message.Parts[0].Text))

	req, _ := http.NewRequest("POST", g.baseURL+"/api/chat", bytes.NewReader(reqBody))
	setFirefoxHeaders(req, FirefoxAcceptJSON)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", g.baseURL)
	req.Header.Set("Referer", g.baseURL+"/")

	resp, err := client.Do(req)
	if err != nil {
		logError("%s | 上游请求失败: %v", reqID, err)
		return
	}
	defer resp.Body.Close()

	logDebug("%s | 上游响应状态: %d", reqID, resp.StatusCode)

	// 非200时记录错误码
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := bufio.NewReader(resp.Body).Peek(512)
		logError("%s | 上游错误 [%d]: %s", reqID, resp.StatusCode, string(bodyBytes))
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		logWarn("%s | 上游返回 429 限流", reqID)
		g.proxyMgr.OnRateLimit()
		g.clearSession(proxyIndex)
		g.clearAccount(proxyIndex)
		http.Error(w, "Rate limited", http.StatusTooManyRequests)
		return
	}

	if resp.StatusCode != http.StatusOK {
		logError("%s | 上游响应异常: %d，切换代理", reqID, resp.StatusCode)
		g.proxyMgr.OnRateLimit()
		g.clearSession(proxyIndex)
		g.clearAccount(proxyIndex)
		http.Error(w, "Upstream error", resp.StatusCode)
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	chatID := uuid.New().String()
	var tokenCount int

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event SSEEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		if event.Type == "text-delta" && event.Delta != "" {
			tokenCount++
			chunk := map[string]interface{}{
				"id":      chatID,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   chatReq.SelectedChatModel,
				"choices": []map[string]interface{}{
					{"index": 0, "delta": map[string]string{"content": event.Delta}},
				},
			}
			chunkData, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", chunkData)
			flusher.Flush()
		}
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	duration := time.Since(startTime)
	logInfo("%s | 请求完成 | 耗时: %v, 输出块数: %d", reqID, duration.Round(time.Millisecond), tokenCount)
}

func (g *Gateway) handleNonStreamWithClient(w http.ResponseWriter, client *http.Client, proxyIndex int, chatReq ChatRequest, reqID string, startTime time.Time) {
	reqBody, _ := json.Marshal(chatReq)
	// 记录请求字段（不含内容）
	logDebug("%s | 上游请求 | id=%s, model=%s, msgId=%s, textLen=%d",
		reqID, chatReq.ID, chatReq.SelectedChatModel, chatReq.Message.ID, len(chatReq.Message.Parts[0].Text))

	req, _ := http.NewRequest("POST", g.baseURL+"/api/chat", bytes.NewReader(reqBody))
	setFirefoxHeaders(req, FirefoxAcceptJSON)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", g.baseURL)
	req.Header.Set("Referer", g.baseURL+"/")

	resp, err := client.Do(req)
	if err != nil {
		logError("%s | 上游请求失败: %v", reqID, err)
		http.Error(w, "Request failed", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	logDebug("%s | 上游响应状态: %d", reqID, resp.StatusCode)

	// 非200时记录错误码
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := bufio.NewReader(resp.Body).Peek(512)
		logError("%s | 上游错误 [%d]: %s", reqID, resp.StatusCode, string(bodyBytes))
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		logWarn("%s | 上游返回 429 限流", reqID)
		g.proxyMgr.OnRateLimit()
		g.clearSession(proxyIndex)
		g.clearAccount(proxyIndex)
		http.Error(w, "Rate limited", http.StatusTooManyRequests)
		return
	}

	if resp.StatusCode != http.StatusOK {
		logError("%s | 上游响应异常: %d，切换代理", reqID, resp.StatusCode)
		g.proxyMgr.OnRateLimit()
		g.clearSession(proxyIndex)
		g.clearAccount(proxyIndex)
		http.Error(w, "Upstream error", resp.StatusCode)
		return
	}

	var fullContent strings.Builder
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event SSEEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		if event.Type == "text-delta" && event.Delta != "" {
			fullContent.WriteString(event.Delta)
		}
	}

	openAIResp := map[string]interface{}{
		"id":      uuid.New().String(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   chatReq.SelectedChatModel,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"message":       map[string]string{"role": "assistant", "content": fullContent.String()},
				"finish_reason": "stop",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(openAIResp)

	duration := time.Since(startTime)
	logInfo("%s | 请求完成 | 耗时: %v, 响应长度: %d", reqID, duration.Round(time.Millisecond), fullContent.Len())
}

func (g *Gateway) convertModel(model string) string {
	if mapped, exists := ModelMapping[model]; exists {
		return mapped
	}
	return model
}

// isMeaninglessRequest 检测无意义请求
// 条件：无历史消息（只有1条user消息），长度<10，且包含测试关键词
func (g *Gateway) isMeaninglessRequest(messages []OpenAIMessage) bool {
	// 统计非 system 消息数量
	var userMsgCount int
	var firstUserMsg string
	for _, msg := range messages {
		if msg.Role == "user" || msg.Role == "assistant" {
			userMsgCount++
			if msg.Role == "user" && firstUserMsg == "" {
				firstUserMsg = g.extractContent(msg.Content)
			}
		}
	}

	// 有历史消息（多轮对话），直接放行
	if userMsgCount > 1 {
		return false
	}

	// 消息长度 >= 10，直接放行
	if len(firstUserMsg) >= 10 {
		return false
	}

	// 检查是否包含测试关键词
	lowerMsg := strings.ToLower(firstUserMsg)
	testKeywords := []string{"hi", "hello", "test", "测试", "你好", "hey", "ping"}
	for _, keyword := range testKeywords {
		if strings.Contains(lowerMsg, keyword) {
			return true
		}
	}

	return false
}

// respondBAKA 返回 BAKA! 响应
func (g *Gateway) respondBAKA(w http.ResponseWriter, stream bool) {
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		chunk := map[string]interface{}{
			"id":      uuid.New().String(),
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   "baka",
			"choices": []map[string]interface{}{
				{"index": 0, "delta": map[string]string{"content": "BAKA!"}},
			},
		}
		chunkData, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", chunkData)
		fmt.Fprintf(w, "data: [DONE]\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      uuid.New().String(),
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "baka",
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"message":       map[string]string{"role": "assistant", "content": "BAKA!"},
					"finish_reason": "stop",
				},
			},
		})
	}
}

// buildConversationMessage 将所有消息拼接成一条完整的对话
func (g *Gateway) buildConversationMessage(messages []OpenAIMessage) string {
	var parts []string

	// 内置系统提示词（禁用工具）
	builtinSystemPrompt := "请不要调用任何工具"

	// 提取用户透传的系统提示词
	var userSystemPrompt string
	for _, msg := range messages {
		if msg.Role == "system" {
			userSystemPrompt = g.extractContent(msg.Content)
			break
		}
	}

	// 构建系统指令块
	if userSystemPrompt != "" {
		parts = append(parts, fmt.Sprintf("[System]\n%s\n\n%s", userSystemPrompt, builtinSystemPrompt))
	} else {
		parts = append(parts, fmt.Sprintf("[System]\n%s", builtinSystemPrompt))
	}

	// 拼接对话历史
	for _, msg := range messages {
		content := g.extractContent(msg.Content)
		switch msg.Role {
		case "system":
			continue
		case "user":
			parts = append(parts, fmt.Sprintf("[User]\n%s", content))
		case "assistant":
			parts = append(parts, fmt.Sprintf("[Assistant]\n%s", content))
		}
	}

	return strings.Join(parts, "\n\n")
}

// extractContent 提取消息内容（支持字符串和数组格式）
func (g *Gateway) extractContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		var texts []string
		for _, item := range v {
			if block, ok := item.(map[string]interface{}); ok {
				if typ, ok := block["type"].(string); ok && typ == "text" {
					if text, ok := block["text"].(string); ok {
						texts = append(texts, text)
					}
				}
			}
		}
		return strings.Join(texts, "")
	}
	return ""
}

func (g *Gateway) HandleModels(w http.ResponseWriter, r *http.Request) {
	models := make([]map[string]interface{}, 0, len(ModelMapping))
	for id := range ModelMapping {
		models = append(models, map[string]interface{}{
			"id":       id,
			"object":   "model",
			"created":  1700000000,
			"owned_by": "chat-sdk",
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"object": "list", "data": models})
}

func (g *Gateway) HandleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	reqID := logger.nextRequestID()
	startTime := time.Now()

	// 使用兼容格式解析
	var anthropicReqCompat struct {
		Model    string                   `json:"model"`
		Messages []AnthropicMessageCompat `json:"messages"`
		Stream   bool                     `json:"stream,omitempty"`
		MaxTokens int                     `json:"max_tokens,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&anthropicReqCompat); err != nil {
		logError("%s | Anthropic 请求解析失败: %v", reqID, err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if len(anthropicReqCompat.Messages) == 0 {
		logError("%s | Anthropic 消息为空", reqID)
		http.Error(w, "No messages found", http.StatusBadRequest)
		return
	}

	streamMode := "非流式"
	if anthropicReqCompat.Stream {
		streamMode = "流式"
	}
	logInfo("%s | Anthropic 请求开始 | 模型: %s, 消息数: %d, 模式: %s", reqID, anthropicReqCompat.Model, len(anthropicReqCompat.Messages), streamMode)

	// 转换为 OpenAI 格式处理
	openAIReq := g.anthropicCompatToOpenAI(anthropicReqCompat)

	// 检测无意义请求
	if g.isMeaninglessRequest(openAIReq.Messages) {
		logInfo("%s | 无意义请求，直接返回 BAKA!", reqID)
		g.respondAnthropicBAKA(w, anthropicReqCompat.Stream)
		return
	}

	finalMessage := g.buildConversationMessage(openAIReq.Messages)
	chatModel := g.convertModel(openAIReq.Model)

	chatReq := ChatRequest{
		ID: uuid.New().String(),
		Message: Message{
			Role:  "user",
			Parts: []MessagePart{{Type: "text", Text: finalMessage}},
			ID:    uuid.New().String(),
		},
		SelectedChatModel:      chatModel,
		SelectedVisibilityType: "private",
	}

	if g.useAuth {
		g.handleWithAccountAnthropic(w, r, openAIReq, chatReq, reqID, startTime)
	} else {
		g.handleWithSessionAnthropic(w, r, openAIReq, chatReq, reqID, startTime)
	}
}

func (g *Gateway) handleWithAccountAnthropic(w http.ResponseWriter, r *http.Request, openAIReq OpenAIRequest, chatReq ChatRequest, reqID string, startTime time.Time) {
	var account *Account
	var err error
	for retry := 0; retry < 3; retry++ {
		account, err = g.getOrCreateAccount()
		if err == nil {
			break
		}
		logWarn("%s | 获取账户失败 (重试 %d/3): %v", reqID, retry+1, err)
		time.Sleep(time.Second)
	}

	if account == nil {
		logError("%s | 服务不可用，所有重试失败", reqID)
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	containerName := ""
	if account.ProxyIndex >= 0 && account.ProxyIndex < len(g.proxyMgr.containers) {
		containerName = g.proxyMgr.containers[account.ProxyIndex]
	}
	logInfo("%s | 转发请求 | 代理: [%d]%s, 账户: %s, 目标模型: %s",
		reqID, account.ProxyIndex, containerName, account.Email, chatReq.SelectedChatModel)

	if openAIReq.Stream {
		g.handleStreamAnthropic(w, account.Client, account.ProxyIndex, chatReq, reqID, startTime)
	} else {
		g.handleNonStreamAnthropic(w, account.Client, account.ProxyIndex, chatReq, reqID, startTime)
	}
}

func (g *Gateway) handleWithSessionAnthropic(w http.ResponseWriter, r *http.Request, openAIReq OpenAIRequest, chatReq ChatRequest, reqID string, startTime time.Time) {
	var session *GuestSession
	var err error
	for retry := 0; retry < 3; retry++ {
		session, err = g.getOrCreateSession()
		if err == nil {
			break
		}
		logWarn("%s | 获取会话失败 (重试 %d/3): %v", reqID, retry+1, err)
		time.Sleep(time.Second)
	}

	if session == nil {
		logError("%s | 服务不可用，所有重试失败", reqID)
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	containerName := ""
	if session.ProxyIndex >= 0 && session.ProxyIndex < len(g.proxyMgr.containers) {
		containerName = g.proxyMgr.containers[session.ProxyIndex]
	}
	logInfo("%s | 转发请求 | 代理: [%d]%s, 目标模型: %s",
		reqID, session.ProxyIndex, containerName, chatReq.SelectedChatModel)

	if openAIReq.Stream {
		g.handleStreamAnthropic(w, session.Client, session.ProxyIndex, chatReq, reqID, startTime)
	} else {
		g.handleNonStreamAnthropic(w, session.Client, session.ProxyIndex, chatReq, reqID, startTime)
	}
}

func (g *Gateway) handleStreamAnthropic(w http.ResponseWriter, client *http.Client, proxyIndex int, chatReq ChatRequest, reqID string, startTime time.Time) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		logError("%s | 流式响应不支持", reqID)
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	reqBody, _ := json.Marshal(chatReq)
	req, _ := http.NewRequest("POST", g.baseURL+"/api/chat", bytes.NewReader(reqBody))
	setFirefoxHeaders(req, FirefoxAcceptJSON)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", g.baseURL)
	req.Header.Set("Referer", g.baseURL+"/")

	resp, err := client.Do(req)
	if err != nil {
		logError("%s | 上游请求失败: %v", reqID, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		logWarn("%s | 上游返回 429 限流", reqID)
		g.proxyMgr.OnRateLimit()
		g.clearSession(proxyIndex)
		g.clearAccount(proxyIndex)
		http.Error(w, "Rate limited", http.StatusTooManyRequests)
		return
	}

	if resp.StatusCode != http.StatusOK {
		logError("%s | 上游响应异常: %d", reqID, resp.StatusCode)
		g.proxyMgr.OnRateLimit()
		g.clearSession(proxyIndex)
		g.clearAccount(proxyIndex)
		http.Error(w, "Upstream error", resp.StatusCode)
		return
	}

	scanner := bufio.NewScanner(resp.Body)
	var tokenCount int

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event SSEEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		if event.Type == "text-delta" && event.Delta != "" {
			tokenCount++
			// Anthropic 格式
			chunk := map[string]interface{}{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]string{"type": "text_delta", "text": event.Delta},
			}
			chunkData, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "event: content_block_delta\n")
			fmt.Fprintf(w, "data: %s\n\n", chunkData)
			flusher.Flush()
		}
	}

	fmt.Fprintf(w, "event: message_stop\n")
	fmt.Fprintf(w, "data: {\"type\":\"message_stop\"}\n\n")
	flusher.Flush()

	duration := time.Since(startTime)
	logInfo("%s | Anthropic 请求完成 | 耗时: %v, 输出块数: %d", reqID, duration.Round(time.Millisecond), tokenCount)
}

func (g *Gateway) handleNonStreamAnthropic(w http.ResponseWriter, client *http.Client, proxyIndex int, chatReq ChatRequest, reqID string, startTime time.Time) {
	reqBody, _ := json.Marshal(chatReq)
	req, _ := http.NewRequest("POST", g.baseURL+"/api/chat", bytes.NewReader(reqBody))
	setFirefoxHeaders(req, FirefoxAcceptJSON)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", g.baseURL)
	req.Header.Set("Referer", g.baseURL+"/")

	resp, err := client.Do(req)
	if err != nil {
		logError("%s | 上游请求失败: %v", reqID, err)
		http.Error(w, "Request failed", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		logWarn("%s | 上游返回 429 限流", reqID)
		g.proxyMgr.OnRateLimit()
		g.clearSession(proxyIndex)
		g.clearAccount(proxyIndex)
		http.Error(w, "Rate limited", http.StatusTooManyRequests)
		return
	}

	if resp.StatusCode != http.StatusOK {
		logError("%s | 上游响应异常: %d", reqID, resp.StatusCode)
		g.proxyMgr.OnRateLimit()
		g.clearSession(proxyIndex)
		g.clearAccount(proxyIndex)
		http.Error(w, "Upstream error", resp.StatusCode)
		return
	}

	var fullContent strings.Builder
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event SSEEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		if event.Type == "text-delta" && event.Delta != "" {
			fullContent.WriteString(event.Delta)
		}
	}

	anthropicResp := map[string]interface{}{
		"id":      "msg_" + uuid.New().String(),
		"type":    "message",
		"role":    "assistant",
		"content": []map[string]interface{}{
			{"type": "text", "text": fullContent.String()},
		},
		"model":             chatReq.SelectedChatModel,
		"stop_reason":       "end_turn",
		"stop_sequence":     nil,
		"usage": map[string]interface{}{
			"input_tokens":  0,
			"output_tokens": len(fullContent.String()),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(anthropicResp)

	duration := time.Since(startTime)
	logInfo("%s | Anthropic 请求完成 | 耗时: %v, 响应长度: %d", reqID, duration.Round(time.Millisecond), fullContent.Len())
}

func (g *Gateway) anthropicCompatToOpenAI(req struct {
	Model    string                   `json:"model"`
	Messages []AnthropicMessageCompat `json:"messages"`
	Stream   bool                     `json:"stream,omitempty"`
	MaxTokens int                     `json:"max_tokens,omitempty"`
}) OpenAIRequest {
	var messages []OpenAIMessage
	for _, msg := range req.Messages {
		role := msg.Role
		content := ""

		// 处理两种 content 格式
		switch v := msg.Content.(type) {
		case string:
			content = v
		case []interface{}:
			if len(v) > 0 {
				if block, ok := v[0].(map[string]interface{}); ok {
					if typ, ok := block["type"].(string); ok && typ == "text" {
						if text, ok := block["text"].(string); ok {
							content = text
						}
					}
				}
			}
		}

		messages = append(messages, OpenAIMessage{Role: role, Content: content})
	}
	return OpenAIRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   req.Stream,
	}
}

func (g *Gateway) respondAnthropicBAKA(w http.ResponseWriter, stream bool) {
	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		chunk := map[string]interface{}{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]string{"type": "text_delta", "text": "BAKA!"},
		}
		chunkData, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "event: content_block_delta\n")
		fmt.Fprintf(w, "data: %s\n\n", chunkData)
		fmt.Fprintf(w, "event: message_stop\n")
		fmt.Fprintf(w, "data: {\"type\":\"message_stop\"}\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	} else {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "msg_" + uuid.New().String(),
			"type":    "message",
			"role":    "assistant",
			"content": []map[string]interface{}{{"type": "text", "text": "BAKA!"}},
			"model":   "claude-3-opus-20240229",
			"stop_reason": "end_turn",
			"usage": map[string]interface{}{
				"input_tokens":  0,
				"output_tokens": 5,
			},
		})
	}
}

func setFirefoxHeaders(req *http.Request, acceptType string) {
	req.Header.Set("User-Agent", FirefoxUserAgent)
	req.Header.Set("Accept", acceptType)
	req.Header.Set("Accept-Language", FirefoxAcceptLanguage)
	req.Header.Set("Accept-Encoding", FirefoxAcceptEncoding)
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
}

// ============================================================================
// 主程序
// ============================================================================

func main() {
	baseURL := getEnv("BASE_URL", "https://demo.chat-sdk.dev")
	warpProxies := getEnv("WARP_PROXIES", "")
	warpContainers := getEnv("WARP_CONTAINERS", "")
	port := getEnv("PORT", "8080")
	debugMode := getEnv("DEBUG", "false")
	useAuth := getEnv("USE_AUTH", "false")

	logger.debugEnabled = debugMode == "true" || debugMode == "1"
	useAuthEnabled := useAuth == "true" || useAuth == "1"

	logInfo("========================================")
	logInfo("Chat SDK 2API 网关启动")
	logInfo("========================================")
	logInfo("监听端口: %s", port)
	logInfo("上游地址: %s", baseURL)
	logInfo("WARP 代理: %s", warpProxies)
	logInfo("WARP 容器: %s", warpContainers)
	logInfo("DEBUG 模式: %v", logger.debugEnabled)
	logInfo("账户模式: %v", useAuthEnabled)
	logInfo("========================================")

	proxyMgr := NewProxyManager(warpProxies, warpContainers)
	gateway := NewGateway(baseURL, proxyMgr, useAuthEnabled)

	// OpenAI 兼容接口
	http.HandleFunc("/v1/chat/completions", gateway.HandleChatCompletion)
	http.HandleFunc("/v1/models", gateway.HandleModels)

	// Anthropic 兼容接口
	http.HandleFunc("/v1/messages", gateway.HandleAnthropicMessages)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	logInfo("服务就绪，等待请求...")
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
