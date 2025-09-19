package rctokenlogin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/openimsdk/tools/log"
)

type Service struct {
	config *Config
	client *http.Client
}

type Config struct {
	Enable bool   `mapstructure:"enable"`
	URL    string `mapstructure:"url"`
}

type UserInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// RCTokenLoginReq 瑞承token验证请求
type RCTokenLoginReq struct {
	Source string `json:"source"`
	Token  string `json:"token"`
}

type RCTokenLoginResp struct {
	Code    string `json:"code"`
	Msg     string `json:"msg"`
	Message string `json:"message"`
	Data    struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"data"`
}

func NewService(config *Config) *Service {
	return &Service{
		config: config,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (s *Service) Enable() bool {
	return s.config.Enable
}

func (s *Service) Authenticate(ctx context.Context, source, token string) (*UserInfo, error) {
	return s.authenticateWithRetry(ctx, source, token, 3)
}

func (s *Service) authenticateWithRetry(ctx context.Context, source, token string, maxRetries int) (*UserInfo, error) {
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.ZInfo(ctx, "瑞承Token验证尝试", "尝试次数", attempt, "最大重试", maxRetries, "来源", source, "Token长度", len(token), "API地址", s.config.URL)

		userInfo, err := s.authenticateOnce(ctx, source, token)
		if err == nil {
			if attempt > 1 {
				log.ZInfo(ctx, "瑞承Token验证重试成功", "尝试次数", attempt, "用户ID", userInfo.ID, "用户名称", userInfo.Name, "邮箱", userInfo.Email)
			}
			return userInfo, nil
		}

		lastErr = err
		log.ZWarn(ctx, "瑞承Token验证尝试失败", err, "尝试次数", attempt, "最大重试", maxRetries)

		// 检查是否为业务错误，业务错误不重试
		if s.isBusinessError(err) {
			log.ZInfo(ctx, "检测到业务错误，不进行重试", "错误", err.Error())
			return nil, err
		}

		// 如果不是最后一次尝试，等待一段时间后重试
		if attempt < maxRetries {
			// 指数退避：1秒、2秒、4秒
			waitTime := time.Duration(attempt) * time.Second
			log.ZInfo(ctx, "等待重试", "等待时间", waitTime)
			time.Sleep(waitTime)
		}
	}

	log.ZError(ctx, "瑞承Token验证最终失败", lastErr, "最大重试次数", maxRetries)
	return nil, lastErr
}

func (s *Service) authenticateOnce(ctx context.Context, source, token string) (*UserInfo, error) {
	// 构建请求体
	reqBody := RCTokenLoginReq{
		Source: source,
		Token:  token,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		log.ZError(ctx, "构建请求体失败", err)
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// 创建POST请求
	req, err := http.NewRequestWithContext(ctx, "POST", s.config.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.ZError(ctx, "创建请求失败", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	log.ZInfo(ctx, "发送请求到瑞承API", "API地址", s.config.URL)
	resp, err := s.client.Do(req)
	if err != nil {
		log.ZError(ctx, "发送请求到瑞承API失败", err, "API地址", s.config.URL)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	log.ZInfo(ctx, "收到瑞承API响应", "状态码", resp.StatusCode)

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		log.ZError(ctx, "瑞承API返回非200状态码", nil, "状态码", resp.StatusCode)
		return nil, fmt.Errorf("RC API returned status: %d", resp.StatusCode)
	}

	// 解析响应
	var apiResp RCTokenLoginResp
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		log.ZError(ctx, "解析瑞承API响应失败", err)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	log.ZInfo(ctx, "瑞承API响应解析成功", "业务状态码", apiResp.Code, "消息", apiResp.Msg, "详细消息", apiResp.Message)

	// 检查业务状态码 - 瑞承API成功时返回"A10001"
	if apiResp.Code != "A10001" {
		log.ZError(ctx, "瑞承API返回错误状态码", nil, "业务状态码", apiResp.Code, "消息", apiResp.Msg, "详细消息", apiResp.Message)
		return nil, fmt.Errorf("RC API error: code=%s, msg=%s, message=%s", apiResp.Code, apiResp.Msg, apiResp.Message)
	}

	log.ZInfo(ctx, "瑞承Token验证成功", "用户ID", apiResp.Data.ID, "用户名称", apiResp.Data.Name, "邮箱", apiResp.Data.Email)

	// 返回用户信息
	return &UserInfo{
		ID:    apiResp.Data.ID,
		Name:  apiResp.Data.Name,
		Email: apiResp.Data.Email,
	}, nil
}

// isBusinessError 判断是否为业务错误（不需要重试的错误）
func (s *Service) isBusinessError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// 业务错误：Token无效、用户不存在等
	if strings.Contains(errStr, "RC API error") {
		return true
	}

	// 网络错误：连接超时、DNS解析失败等（需要重试）
	if strings.Contains(errStr, "failed to send request") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") {
		return false
	}

	// 默认情况下，其他错误也认为是业务错误，不重试
	return true
}
