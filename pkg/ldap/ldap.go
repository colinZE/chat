package ldap

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/go-ldap/ldap/v3"
)

type LDAPService struct {
	config *Config
	conn   *ldap.Conn
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

// Connection 建立LDAP连接
func (l *LDAPService) Connection(ctx context.Context) error {
	conn, err := ldap.Dial("tcp", fmt.Sprintf("%s:%d", l.config.Server, l.config.Port))
	if err != nil {
		return fmt.Errorf("failed to connect to LDAP server: %v", err)
	}

	if l.config.UseSSL {
		if err = conn.StartTLS(&tls.Config{InsecureSkipVerify: true}); err != nil {
			conn.Close()
			return fmt.Errorf("failed to start TLS: %v", err)
		}
	}

	l.conn = conn
	return nil
}

// Close 关闭LDAP连接
func (l *LDAPService) Close() error {
	if l.conn != nil {
		return l.conn.Close()
	}
	return nil
}

func (l *LDAPService) Authenticate(ctx context.Context, username, password string) (*UserInfo, error) {
	if !l.config.Enable {
		return nil, fmt.Errorf("LDAP is not enabled")
	}

	// 每次认证都建立新连接，认证完成后关闭
	if err := l.Connection(ctx); err != nil {
		return nil, err
	}
	defer l.Close() // 确保连接被关闭

	// 使用bind用户进行认证
	err := l.conn.Bind(l.config.BindUser, l.config.BindPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to bind with LDAP server: %v", err)
	}

	// 构建用户搜索过滤器，转义特殊字符
	searchFilter := fmt.Sprintf(l.config.UserFilter, ldap.EscapeFilter(username))

	// 搜索用户
	searchRequest := ldap.NewSearchRequest(
		l.config.BindDN,
		ldap.ScopeWholeSubtree, ldap.DerefAlways, 0, 0, false,
		searchFilter,
		l.config.Attributes,
		nil,
	)

	sr, err := l.conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to search user: %v", err)
	}

	if len(sr.Entries) != 1 {
		return nil, fmt.Errorf("user does not exist or too many entries returned: %s", username)
	}

	entry := sr.Entries[0]
	userDN := entry.DN

	// 提取用户信息
	userInfo := &UserInfo{
		DN: userDN,
	}

	// 从LDAP属性中提取用户信息
	for _, attr := range entry.Attributes {
		switch attr.Name {
		case "userPrincipalName":
			if len(attr.Values) > 0 {
				userInfo.UserPrincipalName = attr.Values[0]
			}
		case "sAMAccountName":
			if len(attr.Values) > 0 {
				userInfo.SAMAccountName = attr.Values[0]
			}
		case "displayName":
			if len(attr.Values) > 0 {
				userInfo.DisplayName = attr.Values[0]
			}
		case "mail":
			if len(attr.Values) > 0 {
				userInfo.Mail = attr.Values[0]
			}
		case "telephoneNumber":
			if len(attr.Values) > 0 {
				userInfo.TelephoneNumber = attr.Values[0]
			}
		case "name":
			if len(attr.Values) > 0 {
				userInfo.Name = attr.Values[0]
			}
		}
	}

	// 使用用户的DN和提供的密码进行认证
	err = l.conn.Bind(userDN, password)
	if err != nil {
		return nil, fmt.Errorf("invalid password for user: %s", username)
	}

	// 重新绑定回bind用户，以便后续查询
	err = l.conn.Bind(l.config.BindUser, l.config.BindPassword)
	if err != nil {
		return nil, fmt.Errorf("failed to rebind with LDAP server: %v", err)
	}

	return userInfo, nil
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
