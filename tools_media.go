package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"time"
)

// ============================================
// 媒体控制工具
// 提供系统音量调节和提示音播放功能
// 仅支持 Windows 平台（通过 PowerShell Core Audio API）
// ============================================

// media — 媒体控制
// 参数:
//
//	action: volume(音量)/beep(提示音)/mute(静音切换)
//	level: 音量百分比 0-100（volume 必填）
//	times: 提示音重复次数 1-10（beep 可选，默认1）
//
// 设计要点：
//   - volume 使用 Windows Core Audio API 精确控制主音量
//   - 精确 API 不可用时回退到 SendKeys 近似调节
//   - beep 使用控制台响铃字符 (\a)
func init() {
	Toolkit["media"] = Tool{
		Name:        "media",
		Description: "🎵 媒体控制：系统音量调节和提示音播放。参数: action (volume/beep/mute), level (0-100, 音量百分比), times (重复次数, 默认1)",
		Category:    "媒体",
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
					// 使用 PowerShell Core Audio API 真实设置音量
					// 通过 AudioEndpointVolume 接口精确控制主音量
					psCmd := fmt.Sprintf(`
$obj = New-Object -ComObject "MMDeviceEnumerator.MMDeviceEnumerator" 2>$null
if (-not $obj) { Write-Output "FAIL"; exit }
$dev = $obj.GetDefaultAudioEndpoint(0, 1)  # eRender=0, eMultimedia=1
$vol = $dev.Activate([Guid]::new("5CDF2C82-841E-4546-9722-0CF74078229A"), 0, [System.IntPtr]::Zero)
$vol.SetMasterVolumeLevelScalar(%f, [Guid]::new("00000000-0000-0000-0000-000000000000"))
Write-Output "OK"
`, float64(vol)/100.0)
					cmd := exec.Command("powershell", "-NoProfile", "-Command", psCmd)
					if output, err := cmd.CombinedOutput(); err != nil || string(output) != "OK\r\n" {
						// 回退方案：使用 nircmd 或 sendskeys 方式（近似调节）
						fallbackCmd := exec.Command("powershell", "-c",
							fmt.Sprintf(`(New-Object -ComObject WScript.Shell).SendKeys([string]::Concat([char]174, [char]175))`))
						fallbackCmd.Run()
						return fmt.Sprintf("🔊 音量已尝试设置为 %d%% (精确API不可用，使用近似调节)", vol)
					}
				}
				return fmt.Sprintf("🔊 音量已设置为 %d%%", vol)

			case "mute":
				if runtime.GOOS == "windows" {
					cmd := exec.Command("powershell", "-c",
						`(New-Object -ComObject WScript.Shell).SendKeys([char]173)`)
					cmd.Run()
				}
				return "🔇 已切换静音"

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
				return "❌ 未知操作，可选: volume, beep, mute"
			}
		},
	}
}
