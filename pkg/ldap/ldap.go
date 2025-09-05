package ldap

import "context"

type LDAPService struct {
	config *Config
}

type Config struct {
	Enable       bool     `mapstructure:"enable"`
	Server       string   `mapstructure:"server"`
	Port         int      `mapstructure:"port"`
	UseSSL       bool     `mapstructure:"useSSL"`
	BindUser     string   `mapstructure:"bindUser"`
	BindPassword string   `mapstructure:"bindPassword"`
	BindDN       string   `mapstructure:"bindDN"`
	Attributes   []string `mapstructure:"attributes"`
	UserFilter   string   `mapstructure:"userFilter"`
}

func NewLDAPService(config *Config) *LDAPService {
	return &LDAPService{config: config}
}

func (l *LDAPService) Enable() bool {
	return l.config.Enable
}

func (l *LDAPService) Authenticate(ctx context.Context, username, password string) (*UserInfo, error) {
	// TODO: 实现LDAP认证逻辑
	return &UserInfo{}, nil
}

type UserInfo struct {
	DN                string // CN=魏泽民,OU=应用运维团队,OU=系统运维部,OU=科技中心,OU=总部,OU=宜信公司,OU=HABROOT,DC=creditease,DC=corp
	UserPrincipalName string // zeminwei2@creditease.cn
	SAMAccountName    string // zeminwei2
	DisplayName       string // 魏泽民
	Mail              string // zeminwei2@creditease.cn
	TelephoneNumber   string // 电话号码
	Name              string // 魏泽民
}
