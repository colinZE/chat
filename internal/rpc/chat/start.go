package chat

import (
	"context"
	"strings"
	"time"

	"github.com/openimsdk/chat/pkg/common/constant"
	"github.com/openimsdk/chat/pkg/common/mctx"
	"github.com/openimsdk/chat/pkg/common/rtc"
	"github.com/openimsdk/chat/pkg/protocol/admin"
	"github.com/openimsdk/chat/pkg/protocol/chat"
	"github.com/openimsdk/tools/db/mongoutil"
	"github.com/openimsdk/tools/discovery"
	"github.com/openimsdk/tools/errs"
	"github.com/openimsdk/tools/log"
	"github.com/openimsdk/tools/mw"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/openimsdk/chat/pkg/common/config"
	"github.com/openimsdk/chat/pkg/common/db/database"
	"github.com/openimsdk/chat/pkg/email"
	"github.com/openimsdk/chat/pkg/ldap"
	"github.com/openimsdk/chat/pkg/rctokenlogin"
	chatClient "github.com/openimsdk/chat/pkg/rpclient/chat"
	"github.com/openimsdk/chat/pkg/sms"
)

type Config struct {
	RpcConfig     config.Chat
	RedisConfig   config.Redis
	MongodbConfig config.Mongo
	Discovery     config.Discovery
	Share         config.Share
	LDAP          config.LDAP
	RCToken       config.RCToken
}

func Start(ctx context.Context, config *Config, client discovery.SvcDiscoveryRegistry, server *grpc.Server) error {
	if len(config.Share.ChatAdmin) == 0 {
		return errs.New("share chat admin not configured")
	}
	mgocli, err := mongoutil.NewMongoDB(ctx, config.MongodbConfig.Build())
	if err != nil {
		return err
	}
	var srv chatSvr
	config.RpcConfig.VerifyCode.Phone.Use = strings.ToLower(config.RpcConfig.VerifyCode.Phone.Use)
	config.RpcConfig.VerifyCode.Mail.Use = strings.ToLower(config.RpcConfig.VerifyCode.Mail.Use)
	srv.conf = config.RpcConfig.VerifyCode
	switch config.RpcConfig.VerifyCode.Phone.Use {
	case "ali":
		ali := config.RpcConfig.VerifyCode.Phone.Ali
		srv.SMS, err = sms.NewAli(ali.Endpoint, ali.AccessKeyID, ali.AccessKeySecret, ali.SignName, ali.VerificationCodeTemplateCode)
		if err != nil {
			return err
		}
	}
	if mail := config.RpcConfig.VerifyCode.Mail; mail.Use == constant.VerifyMail {
		srv.Mail = email.NewMail(mail.SMTPAddr, mail.SMTPPort, mail.SenderMail, mail.SenderAuthorizationCode, mail.Title)
	}
	srv.Database, err = database.NewChatDatabase(mgocli)
	if err != nil {
		return err
	}
	conn, err := client.GetConn(ctx, config.Discovery.RpcService.Admin, grpc.WithTransportCredentials(insecure.NewCredentials()), mw.GrpcClient())
	if err != nil {
		return err
	}
	srv.Admin = chatClient.NewAdminClient(admin.NewAdminClient(conn))
	srv.Code = verifyCode{
		UintTime:   time.Duration(config.RpcConfig.VerifyCode.UintTime) * time.Second,
		MaxCount:   config.RpcConfig.VerifyCode.MaxCount,
		ValidCount: config.RpcConfig.VerifyCode.ValidCount,
		SuperCode:  config.RpcConfig.VerifyCode.SuperCode,
		ValidTime:  time.Duration(config.RpcConfig.VerifyCode.ValidTime) * time.Second,
		Len:        config.RpcConfig.VerifyCode.Len,
	}
	srv.Livekit = rtc.NewLiveKit(config.RpcConfig.LiveKit.Key, config.RpcConfig.LiveKit.Secret, config.RpcConfig.LiveKit.URL)
	srv.AllowRegister = config.RpcConfig.AllowRegister

	// 初始化LDAP服务
	if config.LDAP.Enable {
		ldapConfig := &ldap.Config{
			Enable:       config.LDAP.Enable,
			Server:       config.LDAP.Server,
			Port:         config.LDAP.Port,
			UseSSL:       config.LDAP.UseSSL,
			BindUser:     config.LDAP.BindUser,
			BindPassword: config.LDAP.BindPassword,
			BindDN:       config.LDAP.BindDN,
			Attributes:   config.LDAP.Attributes,
			UserFilter:   config.LDAP.UserFilter,
		}
		srv.LDAP = ldap.NewLDAPService(ldapConfig)
	}

	// 初始化瑞承Token服务
	log.ZInfo(ctx, "Loading RCToken config", "enable", config.RCToken.Enable, "url", config.RCToken.URL)

	// 如果RCToken开启但URL为空，记录错误但不阻止服务启动
	if config.RCToken.Enable && config.RCToken.URL == "" {
		log.ZError(ctx, "RCToken is enabled but URL is empty", errs.ErrArgs.WrapMsg("RCToken URL not configured"))
	}

	rcConfig := &rctokenlogin.Config{
		Enable: config.RCToken.Enable,
		URL:    config.RCToken.URL,
	}

	srv.RCToken = rctokenlogin.NewService(rcConfig)
	log.ZInfo(ctx, "RCToken service initialized", "enable", rcConfig.Enable, "url", rcConfig.URL)

	chat.RegisterChatServer(server, &srv)
	return nil
}

type chatSvr struct {
	chat.UnimplementedChatServer
	conf            config.VerifyCode
	Database        database.ChatDatabaseInterface
	Admin           *chatClient.AdminClient
	SMS             sms.SMS
	Mail            email.Mail
	Code            verifyCode
	Livekit         *rtc.LiveKit
	ChatAdminUserID string
	AllowRegister   bool
	LDAP            *ldap.LDAPService
	RCToken         *rctokenlogin.Service
}

func (o *chatSvr) WithAdminUser(ctx context.Context) context.Context {
	return mctx.WithAdminUser(ctx, o.ChatAdminUserID)
}

type verifyCode struct {
	UintTime   time.Duration // sec
	MaxCount   int
	ValidCount int
	SuperCode  string
	ValidTime  time.Duration
	Len        int
}
