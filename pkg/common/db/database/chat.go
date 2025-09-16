// Copyright © 2023 OpenIM open source community. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package database

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/openimsdk/tools/db/mongoutil"
	"github.com/openimsdk/tools/db/pagination"
	"github.com/openimsdk/tools/db/tx"

	"github.com/openimsdk/chat/pkg/common/constant"
	"github.com/openimsdk/chat/pkg/common/db/dbutil"
	admindb "github.com/openimsdk/chat/pkg/common/db/model/admin"
	"github.com/openimsdk/chat/pkg/common/db/model/chat"
	"github.com/openimsdk/chat/pkg/common/db/table/admin"
	chatdb "github.com/openimsdk/chat/pkg/common/db/table/chat"
	"github.com/openimsdk/chat/pkg/ldap"
	"github.com/openimsdk/chat/pkg/rctokenlogin"
)

type ChatDatabaseInterface interface {
	GetUser(ctx context.Context, userID string) (account *chatdb.Account, err error)
	UpdateUseInfo(ctx context.Context, userID string, attribute map[string]any, updateCred, delCred []*chatdb.Credential) (err error)
	FindAttribute(ctx context.Context, userIDs []string) ([]*chatdb.Attribute, error)
	FindAttributeByAccount(ctx context.Context, accounts []string) ([]*chatdb.Attribute, error)
	TakeAttributeByPhone(ctx context.Context, areaCode string, phoneNumber string) (*chatdb.Attribute, error)
	TakeAttributeByEmail(ctx context.Context, Email string) (*chatdb.Attribute, error)
	TakeAttributeByAccount(ctx context.Context, account string) (*chatdb.Attribute, error)
	TakeAttributeByUserID(ctx context.Context, userID string) (*chatdb.Attribute, error)
	TakeAccount(ctx context.Context, userID string) (*chatdb.Account, error)
	TakeCredentialByAccount(ctx context.Context, account string) (*chatdb.Credential, error)
	TakeCredentialsByUserID(ctx context.Context, userID string) ([]*chatdb.Credential, error)
	TakeLastVerifyCode(ctx context.Context, account string) (*chatdb.VerifyCode, error)
	Search(ctx context.Context, normalUser int32, keyword string, gender int32, pagination pagination.Pagination) (int64, []*chatdb.Attribute, error)
	SearchUser(ctx context.Context, keyword string, userIDs []string, genders []int32, pagination pagination.Pagination) (int64, []*chatdb.Attribute, error)
	CountVerifyCodeRange(ctx context.Context, account string, start time.Time, end time.Time) (int64, error)
	AddVerifyCode(ctx context.Context, verifyCode *chatdb.VerifyCode, fn func() error) error
	UpdateVerifyCodeIncrCount(ctx context.Context, id string) error
	DelVerifyCode(ctx context.Context, id string) error
	RegisterUser(ctx context.Context, register *chatdb.Register, account *chatdb.Account, attribute *chatdb.Attribute, credentials []*chatdb.Credential) error
	LoginRecord(ctx context.Context, record *chatdb.UserLoginRecord, verifyCodeID *string) error
	UpdatePassword(ctx context.Context, userID string, password string) error
	UpdatePasswordAndDeleteVerifyCode(ctx context.Context, userID string, password string, codeID string) error
	NewUserCountTotal(ctx context.Context, before *time.Time) (int64, error)
	UserLoginCountTotal(ctx context.Context, before *time.Time) (int64, error)
	UserLoginCountRangeEverydayTotal(ctx context.Context, start *time.Time, end *time.Time) (map[string]int64, int64, error)
	DelUserAccount(ctx context.Context, userIDs []string) error
	SyncLDAPUser(ctx context.Context, userInfo *ldap.UserInfo, account string) (string, error)
	SyncRCTokenUser(ctx context.Context, userInfo *rctokenlogin.UserInfo, account string) (string, error)
}

func NewChatDatabase(cli *mongoutil.Client) (ChatDatabaseInterface, error) {
	register, err := chat.NewRegister(cli.GetDB())
	if err != nil {
		return nil, err
	}
	account, err := chat.NewAccount(cli.GetDB())
	if err != nil {
		return nil, err
	}
	attribute, err := chat.NewAttribute(cli.GetDB())
	if err != nil {
		return nil, err
	}
	credential, err := chat.NewCredential(cli.GetDB())
	if err != nil {
		return nil, err
	}
	userLoginRecord, err := chat.NewUserLoginRecord(cli.GetDB())
	if err != nil {
		return nil, err
	}
	verifyCode, err := chat.NewVerifyCode(cli.GetDB())
	if err != nil {
		return nil, err
	}
	forbiddenAccount, err := admindb.NewForbiddenAccount(cli.GetDB())
	if err != nil {
		return nil, err
	}
	return &ChatDatabase{
		tx:               cli.GetTx(),
		register:         register,
		account:          account,
		attribute:        attribute,
		credential:       credential,
		userLoginRecord:  userLoginRecord,
		verifyCode:       verifyCode,
		forbiddenAccount: forbiddenAccount,
	}, nil
}

type ChatDatabase struct {
	tx               tx.Tx
	register         chatdb.RegisterInterface
	account          chatdb.AccountInterface
	attribute        chatdb.AttributeInterface
	credential       chatdb.CredentialInterface
	userLoginRecord  chatdb.UserLoginRecordInterface
	verifyCode       chatdb.VerifyCodeInterface
	forbiddenAccount admin.ForbiddenAccountInterface
}

func (o *ChatDatabase) GetUser(ctx context.Context, userID string) (account *chatdb.Account, err error) {
	return o.account.Take(ctx, userID)
}

func (o *ChatDatabase) UpdateUseInfo(ctx context.Context, userID string, attribute map[string]any, updateCred, delCred []*chatdb.Credential) (err error) {
	return o.tx.Transaction(ctx, func(ctx context.Context) error {
		if err = o.attribute.Update(ctx, userID, attribute); err != nil {
			return err
		}
		for _, credential := range updateCred {
			if err = o.credential.CreateOrUpdateAccount(ctx, credential); err != nil {
				return err
			}
		}
		if err = o.credential.DeleteByUserIDType(ctx, delCred...); err != nil {
			return err
		}
		return nil
	})
}

func (o *ChatDatabase) FindAttribute(ctx context.Context, userIDs []string) ([]*chatdb.Attribute, error) {
	return o.attribute.Find(ctx, userIDs)
}

func (o *ChatDatabase) FindAttributeByAccount(ctx context.Context, accounts []string) ([]*chatdb.Attribute, error) {
	return o.attribute.FindAccount(ctx, accounts)
}

func (o *ChatDatabase) TakeAttributeByPhone(ctx context.Context, areaCode string, phoneNumber string) (*chatdb.Attribute, error) {
	return o.attribute.TakePhone(ctx, areaCode, phoneNumber)
}

func (o *ChatDatabase) TakeAttributeByEmail(ctx context.Context, email string) (*chatdb.Attribute, error) {
	return o.attribute.TakeEmail(ctx, email)
}

func (o *ChatDatabase) TakeAttributeByAccount(ctx context.Context, account string) (*chatdb.Attribute, error) {
	return o.attribute.TakeAccount(ctx, account)
}

func (o *ChatDatabase) TakeAttributeByUserID(ctx context.Context, userID string) (*chatdb.Attribute, error) {
	return o.attribute.Take(ctx, userID)
}

func (o *ChatDatabase) TakeLastVerifyCode(ctx context.Context, account string) (*chatdb.VerifyCode, error) {
	return o.verifyCode.TakeLast(ctx, account)
}

func (o *ChatDatabase) TakeAccount(ctx context.Context, userID string) (*chatdb.Account, error) {
	return o.account.Take(ctx, userID)
}

func (o *ChatDatabase) TakeCredentialByAccount(ctx context.Context, account string) (*chatdb.Credential, error) {
	return o.credential.TakeAccount(ctx, account)
}

func (o *ChatDatabase) TakeCredentialsByUserID(ctx context.Context, userID string) ([]*chatdb.Credential, error) {
	return o.credential.Find(ctx, userID)
}

func (o *ChatDatabase) Search(ctx context.Context, normalUser int32, keyword string, genders int32, pagination pagination.Pagination) (total int64, attributes []*chatdb.Attribute, err error) {
	var forbiddenIDs []string
	if int(normalUser) == constant.NormalUser {
		forbiddenIDs, err = o.forbiddenAccount.FindAllIDs(ctx)
		if err != nil {
			return 0, nil, err
		}
	}
	total, totalUser, err := o.attribute.SearchNormalUser(ctx, keyword, forbiddenIDs, genders, pagination)
	if err != nil {
		return 0, nil, err
	}
	return total, totalUser, nil
}

func (o *ChatDatabase) SearchUser(ctx context.Context, keyword string, userIDs []string, genders []int32, pagination pagination.Pagination) (int64, []*chatdb.Attribute, error) {
	return o.attribute.SearchUser(ctx, keyword, userIDs, genders, pagination)
}

func (o *ChatDatabase) CountVerifyCodeRange(ctx context.Context, account string, start time.Time, end time.Time) (int64, error) {
	return o.verifyCode.RangeNum(ctx, account, start, end)
}

func (o *ChatDatabase) AddVerifyCode(ctx context.Context, verifyCode *chatdb.VerifyCode, fn func() error) error {
	return o.tx.Transaction(ctx, func(ctx context.Context) error {
		if err := o.verifyCode.Add(ctx, []*chatdb.VerifyCode{verifyCode}); err != nil {
			return err
		}
		if fn != nil {
			return fn()
		}
		return nil
	})
}

func (o *ChatDatabase) UpdateVerifyCodeIncrCount(ctx context.Context, id string) error {
	return o.verifyCode.Incr(ctx, id)
}

func (o *ChatDatabase) DelVerifyCode(ctx context.Context, id string) error {
	return o.verifyCode.Delete(ctx, id)
}

func (o *ChatDatabase) RegisterUser(ctx context.Context, register *chatdb.Register, account *chatdb.Account, attribute *chatdb.Attribute, credentials []*chatdb.Credential) error {
	return o.tx.Transaction(ctx, func(ctx context.Context) error {
		if err := o.register.Create(ctx, register); err != nil {
			return err
		}
		if err := o.account.Create(ctx, account); err != nil {
			return err
		}
		if err := o.attribute.Create(ctx, attribute); err != nil {
			return err
		}
		if err := o.credential.Create(ctx, credentials...); err != nil {
			return err
		}
		return nil
	})
}

func (o *ChatDatabase) LoginRecord(ctx context.Context, record *chatdb.UserLoginRecord, verifyCodeID *string) error {
	return o.tx.Transaction(ctx, func(ctx context.Context) error {
		if err := o.userLoginRecord.Create(ctx, record); err != nil {
			return err
		}
		if verifyCodeID != nil {
			if err := o.verifyCode.Delete(ctx, *verifyCodeID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (o *ChatDatabase) UpdatePassword(ctx context.Context, userID string, password string) error {
	return o.account.UpdatePassword(ctx, userID, password)
}

func (o *ChatDatabase) UpdatePasswordAndDeleteVerifyCode(ctx context.Context, userID string, password string, codeID string) error {
	return o.tx.Transaction(ctx, func(ctx context.Context) error {
		if err := o.account.UpdatePassword(ctx, userID, password); err != nil {
			return err
		}
		if codeID == "" {
			return nil
		}
		if err := o.verifyCode.Delete(ctx, codeID); err != nil {
			return err
		}
		return nil
	})
}

func (o *ChatDatabase) NewUserCountTotal(ctx context.Context, before *time.Time) (int64, error) {
	return o.register.CountTotal(ctx, before)
}

func (o *ChatDatabase) UserLoginCountTotal(ctx context.Context, before *time.Time) (int64, error) {
	return o.userLoginRecord.CountTotal(ctx, before)
}

func (o *ChatDatabase) UserLoginCountRangeEverydayTotal(ctx context.Context, start *time.Time, end *time.Time) (map[string]int64, int64, error) {
	return o.userLoginRecord.CountRangeEverydayTotal(ctx, start, end)
}

func (o *ChatDatabase) DelUserAccount(ctx context.Context, userIDs []string) error {
	return o.tx.Transaction(ctx, func(ctx context.Context) error {
		if err := o.register.Delete(ctx, userIDs); err != nil {
			return err
		}
		if err := o.account.Delete(ctx, userIDs); err != nil {
			return err
		}
		if err := o.attribute.Delete(ctx, userIDs); err != nil {
			return err
		}
		return nil
	})
}

// SyncLDAPUser 同步LDAP用户信息到本地数据库
func (o *ChatDatabase) SyncLDAPUser(ctx context.Context, userInfo *ldap.UserInfo, account string) (string, error) {
	// 尝试通过邮箱查找用户记录
	credential, err := o.credential.TakeAccount(ctx, userInfo.Mail)
	if err != nil {
		if dbutil.IsDBNotFound(err) {
			// 用户不存在，创建新用户
			return o.createLDAPUser(ctx, userInfo, account)
		}
		return "", err
	}

	// 用户已存在，智能更新用户信息（不覆盖用户自定义字段）
	userID := credential.UserID
	err = o.updateLDAPUserSmart(ctx, userID, userInfo)
	if err != nil {
		return "", err
	}

	return userID, nil
}

// createLDAPUser 创建新的LDAP用户
func (o *ChatDatabase) createLDAPUser(ctx context.Context, userInfo *ldap.UserInfo, account string) (string, error) {
	userID := o.genUserID()
	now := time.Now()

	// 创建用户凭证记录 - 使用邮箱作为登录凭证
	credential := &chatdb.Credential{
		UserID:      userID,
		Account:     userInfo.Mail, // 使用LDAP的邮箱作为登录账号
		Type:        constant.CredentialEmail,
		AllowChange: false, // LDAP用户不允许修改账号
	}

	// 创建用户账号记录（不存储密码）
	accountRecord := &chatdb.Account{
		UserID:         userID,
		Password:       "", // LDAP用户不存储密码
		OperatorUserID: "", // LDAP用户创建时没有操作者
		ChangeTime:     now,
		CreateTime:     now,
	}

	// 创建用户属性记录
	attribute := &chatdb.Attribute{
		UserID:         userID,
		Account:        userInfo.SAMAccountName,
		PhoneNumber:    userInfo.TelephoneNumber,
		AreaCode:       "", // 从电话号码中提取
		Email:          userInfo.Mail,
		Nickname:       userInfo.DisplayName,
		FaceURL:        "",
		Gender:         0,           // 默认值
		BirthTime:      time.Time{}, // 默认值
		ChangeTime:     now,
		CreateTime:     now,
		AllowVibration: constant.DefaultAllowVibration,
		AllowBeep:      constant.DefaultAllowBeep,
		AllowAddFriend: constant.DefaultAllowAddFriend,
		RegisterType:   constant.LDAPRegister,
	}

	// 创建注册记录
	register := &chatdb.Register{
		UserID:      userID,
		DeviceID:    "", // 登录时获取
		IP:          "", // 登录时获取
		Platform:    "LDAP",
		AccountType: "LDAP",
		Mode:        constant.UserMode,
		CreateTime:  now,
	}

	// 保存到数据库
	err := o.RegisterUser(ctx, register, accountRecord, attribute, []*chatdb.Credential{credential})
	if err != nil {
		return "", err
	}

	return userID, nil
}

// updateLDAPUserSmart 智能更新LDAP用户信息，不覆盖用户自定义字段
func (o *ChatDatabase) updateLDAPUserSmart(ctx context.Context, userID string, userInfo *ldap.UserInfo) error {
	// 获取当前用户信息
	currentAttr, err := o.attribute.Take(ctx, userID)
	if err != nil {
		return err
	}

	// 只更新LDAP提供的字段，保留用户自定义的字段
	updateData := map[string]any{
		"change_time": time.Now(),
	}

	// 只更新LDAP有值的字段，如果用户已经自定义了昵称，则不覆盖
	if userInfo.Mail != "" && userInfo.Mail != currentAttr.Email {
		updateData["email"] = userInfo.Mail
	}

	if userInfo.TelephoneNumber != "" && userInfo.TelephoneNumber != currentAttr.PhoneNumber {
		updateData["phone_number"] = userInfo.TelephoneNumber
	}

	// 昵称策略：如果用户没有自定义昵称（还是LDAP的DisplayName），则更新
	// 如果用户已经自定义了昵称，则不覆盖
	if userInfo.DisplayName != "" && currentAttr.Nickname == currentAttr.Account {
		// 如果当前昵称还是账号名（说明用户没有自定义），则更新为LDAP的DisplayName
		updateData["nickname"] = userInfo.DisplayName
	}

	// 如果没有需要更新的字段，直接返回
	if len(updateData) <= 1 { // 只有change_time
		return nil
	}

	// 使用UpdateUseInfo方法更新用户信息
	return o.UpdateUseInfo(ctx, userID, updateData, nil, nil)
}

// genUserID 生成用户ID
func (o *ChatDatabase) genUserID() string {
	const l = 10
	data := make([]byte, l)
	rand.Read(data)
	chars := []byte("0123456789")
	for i := 0; i < len(data); i++ {
		if i == 0 {
			data[i] = chars[1:][data[i]%9]
		} else {
			data[i] = chars[data[i]%10]
		}
	}
	return string(data)
}

// SyncRCTokenUser 同步瑞承用户信息到本地数据库
func (o *ChatDatabase) SyncRCTokenUser(ctx context.Context, userInfo *rctokenlogin.UserInfo, account string) (string, error) {
	// 尝试通过邮箱查找用户记录
	credential, err := o.credential.TakeAccount(ctx, userInfo.Email)
	if err != nil {
		if dbutil.IsDBNotFound(err) {
			// 用户不存在，创建新用户
			return o.createRCTokenUser(ctx, userInfo, account)
		}
		return "", err
	}

	// 用户已存在，智能更新用户信息
	userID := credential.UserID
	err = o.updateRCTokenUserSmart(ctx, userID, userInfo)
	if err != nil {
		return "", err
	}

	return userID, nil
}

// createRCTokenUser 创建新的瑞承用户
func (o *ChatDatabase) createRCTokenUser(ctx context.Context, userInfo *rctokenlogin.UserInfo, account string) (string, error) {
	userID := o.genUserID()
	now := time.Now()

	// 创建用户凭证记录 - 使用邮箱作为登录凭证
	credential := &chatdb.Credential{
		UserID:      userID,
		Account:     userInfo.Email, // 使用瑞承的邮箱作为登录账号
		Type:        constant.CredentialEmail,
		AllowChange: false, // 瑞承用户不允许修改账号
	}

	// 创建用户账号记录（不存储密码）
	accountRecord := &chatdb.Account{
		UserID:         userID,
		Password:       "", // 瑞承用户不存储密码
		OperatorUserID: "", // 瑞承用户创建时没有操作者
		ChangeTime:     now,
		CreateTime:     now,
	}

	// 创建用户属性记录
	attribute := &chatdb.Attribute{
		UserID:         userID,
		Account:        userInfo.ID,
		PhoneNumber:    "", // 瑞承API没有返回手机号
		AreaCode:       "",
		Email:          userInfo.Email,
		Nickname:       userInfo.Name,
		FaceURL:        "",
		Gender:         0,           // 默认值
		BirthTime:      time.Time{}, // 默认值
		ChangeTime:     now,
		CreateTime:     now,
		AllowVibration: constant.DefaultAllowVibration,
		AllowBeep:      constant.DefaultAllowBeep,
		AllowAddFriend: constant.DefaultAllowAddFriend,
		RegisterType:   constant.RCTokenRegister,
	}

	// 创建注册记录
	register := &chatdb.Register{
		UserID:      userID,
		DeviceID:    "", // 登录时获取
		IP:          "", // 登录时获取
		Platform:    "RCToken",
		AccountType: "RCToken",
		Mode:        constant.UserMode,
		CreateTime:  now,
	}

	// 保存到数据库
	err := o.RegisterUser(ctx, register, accountRecord, attribute, []*chatdb.Credential{credential})
	if err != nil {
		return "", err
	}

	return userID, nil
}

// updateRCTokenUserSmart 智能更新瑞承用户信息，不覆盖用户自定义字段
func (o *ChatDatabase) updateRCTokenUserSmart(ctx context.Context, userID string, userInfo *rctokenlogin.UserInfo) error {
	// 获取当前用户信息
	currentAttr, err := o.attribute.Take(ctx, userID)
	if err != nil {
		return err
	}

	// 只更新瑞承提供的字段，保留用户自定义的字段
	updateData := map[string]any{
		"change_time": time.Now(),
	}

	// 只更新瑞承有值的字段，如果用户已经自定义了昵称，则不覆盖
	if userInfo.Email != "" && userInfo.Email != currentAttr.Email {
		updateData["email"] = userInfo.Email
	}

	// 昵称策略：如果用户没有自定义昵称（还是瑞承的Name），则更新
	// 如果用户已经自定义了昵称，则不覆盖
	if userInfo.Name != "" && currentAttr.Nickname == currentAttr.Account {
		// 如果当前昵称还是账号名（说明用户没有自定义），则更新为瑞承的Name
		updateData["nickname"] = userInfo.Name
	}

	// 如果没有需要更新的字段，直接返回
	if len(updateData) <= 1 { // 只有change_time
		return nil
	}

	// 使用UpdateUseInfo方法更新用户信息
	return o.UpdateUseInfo(ctx, userID, updateData, nil, nil)
}
