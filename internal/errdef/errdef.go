// Package errdef 集中定义服务使用的错误。
package errdef

import "errors"

var (
	// 配置校验错误
	ErrServerTokenRequired = errors.New("app.token 不能为空")

	ErrRoutesRequired      = errors.New("app.routes 不能为空")
	ErrRouteNameRequired   = errors.New("route.name 不能为空")
	ErrRouteNameDuplicated = errors.New("route.name 不能重复")
	ErrRouteNameInvalid    = errors.New("route.name 只能包含字母、数字、'-' 和 '_'")
	ErrSourceTypeRequired  = errors.New("source.type 不能为空")
	ErrTargetTypeRequired  = errors.New("target.type 不能为空")
	ErrSourceTypeUnknown   = errors.New("不支持的 source.type")
	ErrTargetTypeUnknown   = errors.New("不支持的 target.type")

	// 通知处理错误
	ErrInvalidWebhook = errors.New("webhook 请求体无效")
	ErrBugIDNotFound  = errors.New("无法从 webhook 中提取 bug ID")
)
