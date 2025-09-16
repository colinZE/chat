package cache

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type CacheManager struct {
	redis *redis.Client
}

type TokenCache struct {
	Token    string    `json:"token"`
	UserInfo UserInfo  `json:"userInfo"`
	ExpireAt time.Time `json:"expireAt"`
}

type UserInfo struct {
	ID     string `json:"id"`     // 瑞承用户ID
	Name   string `json:"name"`   // 用户名称
	Email  string `json:"email"`  // 用户邮箱
	UserID string `json:"userID"` // IM系统中的用户ID
}

func NewCacheManager(redis *redis.Client) *CacheManager {
	return &CacheManager{
		redis: redis,
	}
}

// hashToken 对token进行MD5哈希，避免在Redis key中存储明文token
func (cm *CacheManager) hashToken(token string) string {
	hash := md5.Sum([]byte(token))
	return hex.EncodeToString(hash[:])
}

// GetTokenInfo 从缓存中获取token信息
func (cm *CacheManager) GetTokenInfo(ctx context.Context, token string) (*UserInfo, error) {
	key := fmt.Sprintf("rc_token:%s", cm.hashToken(token))
	result, err := cm.redis.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var tokenCache TokenCache
	if err := json.Unmarshal([]byte(result), &tokenCache); err != nil {
		return nil, err
	}

	// 检查是否过期
	if time.Now().After(tokenCache.ExpireAt) {
		cm.redis.Del(ctx, key) // 删除过期缓存
		return nil, fmt.Errorf("token expired")
	}

	return &tokenCache.UserInfo, nil
}

// SetTokenInfo 设置token信息到缓存
func (cm *CacheManager) SetTokenInfo(ctx context.Context, token string, userInfo *UserInfo, ttl time.Duration) error {
	key := fmt.Sprintf("rc_token:%s", cm.hashToken(token))
	tokenCache := TokenCache{
		Token:    token,
		UserInfo: *userInfo,
		ExpireAt: time.Now().Add(ttl),
	}

	data, err := json.Marshal(tokenCache)
	if err != nil {
		return err
	}

	return cm.redis.Set(ctx, key, data, ttl).Err()
}
