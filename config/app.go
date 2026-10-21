// Package config 保存应用名称与全局配置。
package config

// App 应用名称，同时用于 hd SDK 的配置查找与日志文件命名。
const App = "zentao-mattermost"

// Config 是应用程序的全局配置实例
var Config RootConfig
