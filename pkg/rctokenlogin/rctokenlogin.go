package rctokenlogin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
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
	// 构建请求体
	reqBody := RCTokenLoginReq{
		Source: source,
		Token:  token,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// 创建POST请求
	req, err := http.NewRequestWithContext(ctx, "POST", s.config.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RC API returned status: %d", resp.StatusCode)
	}

	// 解析响应
	var apiResp RCTokenLoginResp
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// 检查业务状态码 - 瑞承API成功时返回"A10001"
	if apiResp.Code != "A10001" {
		return nil, fmt.Errorf("RC API error: code=%s, msg=%s, message=%s", apiResp.Code, apiResp.Msg, apiResp.Message)
	}

	// 返回用户信息
	return &UserInfo{
		ID:    apiResp.Data.ID,
		Name:  apiResp.Data.Name,
		Email: apiResp.Data.Email,
	}, nil
}
