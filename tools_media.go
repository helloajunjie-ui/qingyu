package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"time"
)

func init() {
	Toolkit["media"] = Tool{
		Name:        "media",
		Description: "🎵 媒体控制：系统音量调节和提示音播放。参数: action (volume/beep), level (0-100, 音量百分比), times (重复次数, 默认1)",
		Execute: func(args map[string]string) string {
			action := args["action"]

			switch action {
			case "volume":
				level := args["level"]
				if level == "" {
					return "❌ 请提供音量级别 (0-100)"
				}
				vol, err := strconv.Atoi(level)
				if err != nil || vol < 0 || vol > 100 {
					return "❌ 音量级别需为 0-100 的整数"
				}
				if runtime.GOOS == "windows" {
					cmd := exec.Command("powershell", "-c",
						fmt.Sprintf(`(New-Object -ComObject WScript.Shell).SendKeys([char]173)`, vol))
					cmd.Run()
				}
				return fmt.Sprintf("🔊 音量已设置为 %d%%", vol)

			case "beep":
				timesStr := args["times"]
				times := 1
				if timesStr != "" {
					if t, err := strconv.Atoi(timesStr); err == nil && t > 0 && t <= 10 {
						times = t
					}
				}
				if runtime.GOOS == "windows" {
					for i := 0; i < times; i++ {
						fmt.Print("\a")
						time.Sleep(200 * time.Millisecond)
					}
				}
				if times == 1 {
					return "🔔 叮"
				}
				return fmt.Sprintf("🔔 叮 x%d", times)

			default:
				return "❌ 未知操作，可选: volume, beep"
			}
		},
	}
}
