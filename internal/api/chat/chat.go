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

package chat

import (
	"context"
	"fmt"
	"io"

	"time"

	"github.com/openimsdk/chat/internal/api/util"

	"github.com/gin-gonic/gin"
	"github.com/openimsdk/chat/pkg/cache"
	"github.com/openimsdk/chat/pkg/common/apistruct"
	"github.com/openimsdk/chat/pkg/common/config"
	chatdb "github.com/openimsdk/chat/pkg/common/db/database"
	"github.com/openimsdk/chat/pkg/common/imapi"
	"github.com/openimsdk/chat/pkg/common/mctx"
	"github.com/openimsdk/chat/pkg/protocol/admin"
	chatpb "github.com/openimsdk/chat/pkg/protocol/chat"
	"github.com/openimsdk/chat/pkg/rctokenlogin"
	constantpb "github.com/openimsdk/protocol/constant"
	"github.com/openimsdk/protocol/sdkws"
	"github.com/openimsdk/tools/a2r"
	"github.com/openimsdk/tools/apiresp"
	"github.com/openimsdk/tools/errs"
	"github.com/openimsdk/tools/log"
)

func New(chatClient chatpb.ChatClient, adminClient admin.AdminClient, imApiCaller imapi.CallerInterface, api *util.Api) *Api {
	return &Api{
		Api:         api,
		chatClient:  chatClient,
		adminClient: adminClient,
		imApiCaller: imApiCaller,
	}
}

type Api struct {
	*util.Api
	chatClient   chatpb.ChatClient
	adminClient  admin.AdminClient
	imApiCaller  imapi.CallerInterface
	Support      config.Support
	CacheManager *cache.CacheManager
	RCToken      *rctokenlogin.Service
	Database     chatdb.ChatDatabaseInterface
}

// ################## ACCOUNT ##################

func (o *Api) SendVerifyCode(c *gin.Context) {
	req, err := a2r.ParseRequest[chatpb.SendVerifyCodeReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	ip, err := o.GetClientIP(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.Ip = ip
	resp, err := o.chatClient.SendVerifyCode(c, req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, resp)
}

func (o *Api) VerifyCode(c *gin.Context) {
	a2r.Call(c, chatpb.ChatClient.VerifyCode, o.chatClient)
}

func (o *Api) RegisterUser(c *gin.Context) {
	req, err := a2r.ParseRequest[chatpb.RegisterUserReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	ip, err := o.GetClientIP(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.Ip = ip

	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiCtx := mctx.WithApiToken(c, imToken)
	rpcCtx := o.WithAdminUser(c)

	checkResp, err := o.chatClient.CheckUserExist(rpcCtx, &chatpb.CheckUserExistReq{User: req.User})
	if err != nil {
		log.ZDebug(rpcCtx, "Not else", errs.Unwrap(err))
		apiresp.GinError(c, err)
		return
	}
	if checkResp.IsRegistered {
		isUserNotExist, err := o.imApiCaller.AccountCheckSingle(apiCtx, checkResp.Userid)
		if err != nil {
			apiresp.GinError(c, err)
			return
		}
		// if User is  not exist in SDK server. You need delete this user and register new user again.
		if isUserNotExist {
			_, err := o.chatClient.DelUserAccount(rpcCtx, &chatpb.DelUserAccountReq{UserIDs: []string{checkResp.Userid}})
			log.ZDebug(c, "Delete Succsssss", checkResp.Userid)
			if err != nil {
				apiresp.GinError(c, err)
				return
			}
		}
	}

	respRegisterUser, err := o.chatClient.RegisterUser(c, req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	userInfo := &sdkws.UserInfo{
		UserID:     respRegisterUser.UserID,
		Nickname:   req.User.Nickname,
		FaceURL:    req.User.FaceURL,
		CreateTime: time.Now().UnixMilli(),
	}
	err = o.imApiCaller.RegisterUser(apiCtx, []*sdkws.UserInfo{userInfo})
	if err != nil {
		apiresp.GinError(c, err)
		return
	}

	if resp, err := o.adminClient.FindDefaultFriend(rpcCtx, &admin.FindDefaultFriendReq{}); err == nil {
		_ = o.imApiCaller.ImportFriend(apiCtx, respRegisterUser.UserID, resp.UserIDs)
	}
	if resp, err := o.adminClient.FindDefaultGroup(rpcCtx, &admin.FindDefaultGroupReq{}); err == nil {
		_ = o.imApiCaller.InviteToGroup(apiCtx, respRegisterUser.UserID, resp.GroupIDs)
	}
	var resp apistruct.UserRegisterResp
	if req.AutoLogin {
		resp.ImToken, err = o.imApiCaller.GetUserToken(apiCtx, respRegisterUser.UserID, req.Platform)
		if err != nil {
			apiresp.GinError(c, err)
			return
		}
	}
	resp.ChatToken = respRegisterUser.ChatToken
	resp.UserID = respRegisterUser.UserID
	apiresp.GinSuccess(c, &resp)
}

func (o *Api) Login(c *gin.Context) {
	req, err := a2r.ParseRequest[chatpb.LoginReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	ip, err := o.GetClientIP(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req.Ip = ip
	resp, err := o.chatClient.Login(c, req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	adminToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiCtx := mctx.WithApiToken(c, adminToken)

	// 确保用户在OpenIM中存在（特别是LDAP用户）
	if err := o.ensureUserInOpenIM(apiCtx, resp.UserID, req.Email); err != nil {
		apiresp.GinError(c, err)
		return
	}

	imToken, err := o.imApiCaller.GetUserToken(apiCtx, resp.UserID, req.Platform)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, &apistruct.LoginResp{
		ImToken:   imToken,
		UserID:    resp.UserID,
		ChatToken: resp.ChatToken,
	})
}

// RCTokenLogin 瑞承Token登录
func (o *Api) RCTokenLogin(c *gin.Context) {
	req, err := a2r.ParseRequest[chatpb.RCTokenLoginReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}

	// 调用RPC方法
	resp, err := o.chatClient.RCTokenLogin(c, req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}

	// 获取OpenIM管理员token
	adminToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiCtx := mctx.WithApiToken(c, adminToken)

	// 确保用户在OpenIM中存在
	if err := o.ensureUserInOpenIM(apiCtx, resp.UserID, ""); err != nil {
		apiresp.GinError(c, err)
		return
	}

	// 获取ImToken
	imToken, err := o.imApiCaller.GetUserToken(apiCtx, resp.UserID, req.Platform)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}

	// 返回完整响应
	apiresp.GinSuccess(c, &apistruct.LoginResp{
		ImToken:   imToken,
		UserID:    resp.UserID,
		ChatToken: resp.ChatToken,
	})
}

// SupportUrls 获取客服用户列表
func (o *Api) SupportUrls(c *gin.Context) {
	source := c.Query("source")
	rcToken := c.Query("token")
	var supportUsers []apistruct.SupportUserInfo
	var currentUserID string

	if source != "" && rcToken != "" {
		// 检查缓存
		if userInfo, err := o.CacheManager.GetTokenInfo(c, rcToken); err == nil {
			// 缓存命中，直接获取userID
			currentUserID = userInfo.UserID
			log.ZInfo(c, "Token cache hit", "userID", userInfo.UserID, "name", userInfo.Name, "email", userInfo.Email)
		} else {
			// 缓存未命中，验证瑞承token
			rcUserInfo, err := o.RCToken.Authenticate(c, source, rcToken)
			if err != nil {
				apiresp.GinError(c, err)
				log.ZError(c, "请求瑞承接口获取用户信息失败", err)
				return
			}
			log.ZInfo(c, "请求瑞承接口获取用户信息:", "rcID", rcUserInfo.ID, "name", rcUserInfo.Name, "email", rcUserInfo.Email)

			// 通过邮箱查询用户在IM中的ID
			userID, err := o.Database.GetUserIDByEmail(c, rcUserInfo.Email)
			if err != nil {
				// 用户不存在，需要注册到chat和OpenIM
				log.ZInfo(c, "User not found in IM system, registering new user", "email", rcUserInfo.Email, "rcID", rcUserInfo.ID, "name", rcUserInfo.Name)

				// 注册用户到chat数据库
				userID, err = o.Database.SyncRCTokenUser(c, &rctokenlogin.UserInfo{
					ID:    rcUserInfo.ID,
					Name:  rcUserInfo.Name,
					Email: rcUserInfo.Email,
				}, rcUserInfo.Email)
				if err != nil {
					log.ZError(c, "Failed to register user to chat database", err, "email", rcUserInfo.Email)
					// 注册失败，不设置currentUserID，这样不会生成聊天URL
					currentUserID = ""
				} else {
					// 注册成功，确保用户在OpenIM中存在
					if err := o.ensureUserInOpenIM(c, userID, rcUserInfo.Name); err != nil {
						log.ZError(c, "Failed to ensure user in OpenIM", err, "userID", userID)
						// OpenIM注册失败，也不设置currentUserID
						currentUserID = ""
					} else {
						currentUserID = userID
						log.ZInfo(c, "User registered successfully", "email", rcUserInfo.Email, "userID", userID, "name", rcUserInfo.Name, "rcID", rcUserInfo.ID)
					}
				}
			} else {
				// 用户已存在
				currentUserID = userID
				log.ZInfo(c, "User found by email", "email", rcUserInfo.Email, "userID", userID, "name", rcUserInfo.Name, "rcID", rcUserInfo.ID)
			}

			// 缓存完整的用户信息，包含userID，过期时间30分钟
			cacheUserInfo := &cache.UserInfo{
				ID:     rcUserInfo.ID,
				Name:   rcUserInfo.Name,
				Email:  rcUserInfo.Email,
				UserID: userID, // 直接缓存userID
			}
			if err := o.CacheManager.SetTokenInfo(c, rcToken, cacheUserInfo, 30*time.Minute); err != nil {
				log.ZWarn(c, "Failed to cache token info", err)
			}
		}
	}

	// 返回客服用户列表，包含聊天URL
	for _, user := range o.Support.SupportUsers {
		chatURL := ""
		if currentUserID != "" {
			chatURL = fmt.Sprintf("/chat/si_%s_%s", user.UserID, currentUserID)
		}

		supportUsers = append(supportUsers, apistruct.SupportUserInfo{
			UserID:   user.UserID,
			Nickname: user.Nickname,
			ChatURL:  chatURL,
		})
	}
	log.ZInfo(c, "source", source, "rcToken", rcToken, "客服用户列表", "supportUsers", supportUsers)

	apiresp.GinSuccess(c, &apistruct.SupportUrlsResp{
		SupportUsers: supportUsers,
	})
}

func (o *Api) ResetPassword(c *gin.Context) {
	a2r.Call(c, chatpb.ChatClient.ResetPassword, o.chatClient)
}

func (o *Api) ChangePassword(c *gin.Context) {
	req, err := a2r.ParseRequest[chatpb.ChangePasswordReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	resp, err := o.chatClient.ChangePassword(c, req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}

	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	err = o.imApiCaller.ForceOffLine(mctx.WithApiToken(c, imToken), req.UserID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, resp)
}

// ################## USER ##################

func (o *Api) UpdateUserInfo(c *gin.Context) {
	req, err := a2r.ParseRequest[chatpb.UpdateUserInfoReq](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	respUpdate, err := o.chatClient.UpdateUserInfo(c, req)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}

	var imToken string
	imToken, err = o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	var (
		nickName string
		faceURL  string
	)
	if req.Nickname != nil {
		nickName = req.Nickname.Value
	} else {
		nickName = respUpdate.NickName
	}
	if req.FaceURL != nil {
		faceURL = req.FaceURL.Value
	} else {
		faceURL = respUpdate.FaceUrl
	}
	err = o.imApiCaller.UpdateUserInfo(mctx.WithApiToken(c, imToken), req.UserID, nickName, faceURL)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, apistruct.UpdateUserInfoResp{})
}

func (o *Api) FindUserPublicInfo(c *gin.Context) {
	a2r.Call(c, chatpb.ChatClient.FindUserPublicInfo, o.chatClient)
}

func (o *Api) FindUserFullInfo(c *gin.Context) {
	a2r.Call(c, chatpb.ChatClient.FindUserFullInfo, o.chatClient)
}

func (o *Api) SearchUserFullInfo(c *gin.Context) {
	a2r.Call(c, chatpb.ChatClient.SearchUserFullInfo, o.chatClient)
}

func (o *Api) SearchUserPublicInfo(c *gin.Context) {
	a2r.Call(c, chatpb.ChatClient.SearchUserPublicInfo, o.chatClient)
}

func (o *Api) GetTokenForVideoMeeting(c *gin.Context) {
	a2r.Call(c, chatpb.ChatClient.GetTokenForVideoMeeting, o.chatClient)
}

// ################## APPLET ##################

func (o *Api) FindApplet(c *gin.Context) {
	a2r.Call(c, admin.AdminClient.FindApplet, o.adminClient)
}

// ################## CONFIG ##################

func (o *Api) GetClientConfig(c *gin.Context) {
	a2r.Call(c, admin.AdminClient.GetClientConfig, o.adminClient)
}

// ################## CALLBACK ##################

func (o *Api) OpenIMCallback(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	req := &chatpb.OpenIMCallbackReq{
		Command: c.Query(constantpb.CallbackCommand),
		Body:    string(body),
	}
	if _, err := o.chatClient.OpenIMCallback(c, req); err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, nil)
}

func (o *Api) SearchFriend(c *gin.Context) {
	req, err := a2r.ParseRequest[struct {
		UserID string `json:"userID"`
		chatpb.SearchUserInfoReq
	}](c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if req.UserID == "" {
		req.UserID = mctx.GetOpUserID(c)
	}
	imToken, err := o.imApiCaller.ImAdminTokenWithDefaultAdmin(c)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	userIDs, err := o.imApiCaller.FriendUserIDs(mctx.WithApiToken(c, imToken), req.UserID)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	if len(userIDs) == 0 {
		apiresp.GinSuccess(c, &chatpb.SearchUserInfoResp{})
		return
	}
	req.SearchUserInfoReq.UserIDs = userIDs
	resp, err := o.chatClient.SearchUserInfo(c, &req.SearchUserInfoReq)
	if err != nil {
		apiresp.GinError(c, err)
		return
	}
	apiresp.GinSuccess(c, resp)
}

func (o *Api) LatestApplicationVersion(c *gin.Context) {
	a2r.Call(c, admin.AdminClient.LatestApplicationVersion, o.adminClient)
}

func (o *Api) PageApplicationVersion(c *gin.Context) {
	a2r.Call(c, admin.AdminClient.PageApplicationVersion, o.adminClient)
}

// ensureUserInOpenIM 确保用户在OpenIM中存在
func (o *Api) ensureUserInOpenIM(ctx context.Context, userID string, email string) error {
	// 为ctx添加operationID，OpenIM框架要求所有RPC调用都必须包含operationID
	ctx = context.WithValue(ctx, constantpb.OperationID, "ensureUserInOpenIM_"+userID)

	// 从chat数据库获取用户信息
	userInfo, err := o.chatClient.FindUserPublicInfo(ctx, &chatpb.FindUserPublicInfoReq{UserIDs: []string{userID}})
	if err != nil {
		return err
	}

	if len(userInfo.Users) == 0 {
		return errs.New("user not found in chat database").Wrap()
	}

	chatUser := userInfo.Users[0]
	imUser := &sdkws.UserInfo{
		UserID:     userID,
		Nickname:   chatUser.Nickname,
		FaceURL:    chatUser.FaceURL,
		CreateTime: time.Now().UnixMilli(),
	}

	// 检查用户是否已在OpenIM中存在
	users, err := o.imApiCaller.GetUsersInfo(ctx, []string{userID})
	if err != nil || len(users) == 0 {
		// 用户不存在，注册用户
		err = o.imApiCaller.RegisterUser(ctx, []*sdkws.UserInfo{imUser})
		if err != nil {
			return err
		}
		log.ZInfo(ctx, "User registered in OpenIM", "userID", userID, "nickname", chatUser.Nickname)
	} else {
		// 用户已存在，检查是否需要更新信息
		existingUser := users[0]
		if existingUser.Nickname != chatUser.Nickname {
			// 更新用户昵称
			err = o.imApiCaller.UpdateUserInfo(ctx, userID, chatUser.Nickname, chatUser.FaceURL)
			if err != nil {
				log.ZWarn(ctx, "Failed to update user nickname in OpenIM", err, "userID", userID)
			} else {
				log.ZInfo(ctx, "User nickname updated in OpenIM", "userID", userID, "nickname", chatUser.Nickname)
			}
		}
		log.ZInfo(ctx, "User already exists in OpenIM", "userID", userID, "nickname", chatUser.Nickname)
	}

	return nil
}
