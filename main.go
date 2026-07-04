// 青羽 (Qingyu) — 桌面 AI 伴侣
//
// 基于 Wails2 构建的桌面应用，提供：
//   - 类人交互界面（玻璃拟态 UI + 情绪心跳动画）
//   - 长期记忆系统（结构化 JSON 存储 + 衰减/归档机制）
//   - 工具书柜体系（40+ 工具：文件/网络/邮件/Office/PIM/密码保险箱等）
//   - 自律循环（自主思考 + 情绪状态机 + 主动聊天）
//   - 分层模型调度（轻量模型处理简单任务，主模型处理复杂推理）
//
// 架构分层：
//
//	main.go          — 应用入口，Wails2 初始化
//	app.go           — 核心逻辑：Chat/自律循环/心跳/工具执行/自检
//	memory.go        — 记忆系统：存储/检索/衰减/归档/迁移
//	toolkit.go       — 工具框架：Tool 定义/加密辅助/审计日志
//	settings.go      — 配置管理：settings.json 加载/热更新
//	summarizer.go    — 摘要引擎：LLM 调用/分层模型调度
//	cache.go         — 缓存引擎：内存+磁盘双层缓存
//	tools_*.go       — 各分类工具实现
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

// main 青羽桌面应用入口
// 1. 创建 App 实例
// 2. 从 settings.json 读取窗口配置
// 3. 启动 Wails2 运行时（透明无框窗口 + 玻璃效果）
// 4. 绑定 App 结构体到前端 JS 上下文
func main() {
	app := NewApp()
	win := GetSettings().Window

	err := wails.Run(&options.App{
		Title:       win.Title,
		Width:       win.Width,
		Height:      win.Height,
		MinWidth:    win.MinWidth,
		MinHeight:   win.MinHeight,
		AlwaysOnTop: win.AlwaysOnTop,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		OnStartup:        app.startup,
		Windows: &windows.Options{
			WebviewIsTransparent: win.Transparent,
			WindowIsTranslucent:  win.Transparent,
			DisableWindowIcon:    win.DisableIcon,
		},
		Frameless: win.Frameless,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
