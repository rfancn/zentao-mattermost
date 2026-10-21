// Package errdef 集中定义服务使用的错误。
package errdef

import "errors"

var (
	// 配置校验错误
	ErrServerTokenRequired      = errors.New("server.token 不能为空")
	ErrZentaoURLRequired        = errors.New("zentao.url 不能为空")
	ErrZentaoAuthRequired       = errors.New("zentao.token 或 zentao.account+password 必须配置一项")
	ErrMattermostURLRequired    = errors.New("mattermost.url 不能为空")
	ErrMattermostTokenRequired  = errors.New("mattermost.token 不能为空")
	ErrMattermostTeamIDRequired = errors.New("mattermost.team_id 不能为空")
	ErrChannelTypeInvalid       = errors.New("channel.type 只能是 P 或 O")

	// 通知处理错误
	ErrBugIDNotFound = errors.New("无法从 webhook 中提取 bug ID")
	ErrNoAssignee    = errors.New("bug 当前没有指派对象")
)
