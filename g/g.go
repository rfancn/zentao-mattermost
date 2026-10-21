// Package g 保存应用级全局变量与配置结构，供各业务包直接使用。
package g

// App 应用名称，同时用于 hd SDK 的配置查找与日志文件命名。
const App = "zentao-mattermost"

// Config 全局配置，由 hd SDK 在启动时填充。
var Config AppConfig
