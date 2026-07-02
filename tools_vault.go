package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	Toolkit["vault"] = Tool{
		Name:        "vault",
		Description: "【密码保险箱】加密存储和管理密码。参数: action (add/get/list/delete/update/set_master), service (服务名), username (用户名), password (密码), master (主密码)。首次使用需先 set_master。get 操作自动复制密码到剪贴板。",
		Execute: func(args map[string]string) string {
			action := args["action"]
			if action == "" {
				return "错误：需要 action 参数 (add/get/list/delete/update/set_master)"
			}

			vaultPath := filepath.Join(RootDir, "dna", "vault.dat")
			vaultDir := filepath.Join(RootDir, "dna")

			os.MkdirAll(vaultDir, 0700)

			masterPath := filepath.Join(RootDir, "dna", ".vault_master")
			masterKey := args["master"]

			if action == "set_master" {
				if masterKey == "" {
					return "错误：需要 master 参数设置主密码"
				}
				if len(masterKey) < 4 {
					return "错误：主密码至少 4 位"
				}
				salt := make([]byte, 16)
				rand.Read(salt)
				hash := sha256.Sum256(append([]byte(masterKey), salt...))
				masterData := struct {
					Hash string `json:"hash"`
					Salt string `json:"salt"`
				}{
					Hash: hex.EncodeToString(hash[:]),
					Salt: hex.EncodeToString(salt),
				}
				data, _ := json.Marshal(masterData)
				if err := os.WriteFile(masterPath, data, 0600); err != nil {
					return fmt.Sprintf("错误：无法保存主密码: %v", err)
				}
				return "✅ 主密码设置成功！请牢记此密码，忘记后将无法恢复保险箱数据。"
			}

			if masterKey == "" {
				return "错误：需要 master 参数（主密码）来解锁保险箱"
			}
			masterDataBytes, err := os.ReadFile(masterPath)
			if err != nil {
				return "错误：未设置主密码，请先使用 set_master 设置"
			}
			var stored struct {
				Hash string `json:"hash"`
				Salt string `json:"salt"`
			}
			json.Unmarshal(masterDataBytes, &stored)
			salt, _ := hex.DecodeString(stored.Salt)
			hash := sha256.Sum256(append([]byte(masterKey), salt...))
			if hex.EncodeToString(hash[:]) != stored.Hash {
				return "错误：主密码错误"
			}

			key := sha256.Sum256([]byte(masterKey + stored.Salt))

			var vaultData map[string]map[string]string
			vaultBytes, err := os.ReadFile(vaultPath)
			if err == nil && len(vaultBytes) > 0 {
				decrypted, err := decryptAES(vaultBytes, key[:])
				if err != nil {
					return fmt.Sprintf("错误：保险箱解密失败: %v", err)
				}
				json.Unmarshal(decrypted, &vaultData)
			}
			if vaultData == nil {
				vaultData = make(map[string]map[string]string)
			}

			switch action {
			case "add":
				service := args["service"]
				username := args["username"]
				password := args["password"]
				if service == "" || password == "" {
					return "错误：add 需要 service 和 password 参数"
				}
				if _, exists := vaultData[service]; exists {
					return fmt.Sprintf("⚠️ 服务 '%s' 已存在，使用 update 更新", service)
				}
				entry := map[string]string{"password": password}
				if username != "" {
					entry["username"] = username
				}
				vaultData[service] = entry
				if err := saveVault(vaultPath, key[:], vaultData); err != nil {
					return fmt.Sprintf("错误：保存失败: %v", err)
				}
				return fmt.Sprintf("✅ 已保存 '%s' 的密码", service)

			case "get":
				service := args["service"]
				if service == "" {
					return "错误：get 需要 service 参数"
				}
				entry, exists := vaultData[service]
				if !exists {
					return fmt.Sprintf("❌ 未找到 '%s' 的密码", service)
				}
				pwd := entry["password"]
				clipResult := ""
				if pwd != "" {
					clipResult = tryClipboardWrite(pwd)
				}
				user := entry["username"]
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("🔐 %s\n", service))
				if user != "" {
					sb.WriteString(fmt.Sprintf("用户名: %s\n", user))
				}
				sb.WriteString(fmt.Sprintf("密码: %s\n", pwd))
				if clipResult != "" {
					sb.WriteString(clipResult)
				}
				return sb.String()

			case "list":
				if len(vaultData) == 0 {
					return "📭 保险箱为空"
				}
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("📋 保险箱 (%d 条)\n", len(vaultData)))
				sb.WriteString("───\n")
				for svc, entry := range vaultData {
					user := entry["username"]
					if user != "" {
						sb.WriteString(fmt.Sprintf("🔑 %s (用户: %s)\n", svc, user))
					} else {
						sb.WriteString(fmt.Sprintf("🔑 %s\n", svc))
					}
				}
				return sb.String()

			case "delete":
				service := args["service"]
				if service == "" {
					return "错误：delete 需要 service 参数"
				}
				if _, exists := vaultData[service]; !exists {
					return fmt.Sprintf("❌ 未找到 '%s'", service)
				}
				delete(vaultData, service)
				if err := saveVault(vaultPath, key[:], vaultData); err != nil {
					return fmt.Sprintf("错误：保存失败: %v", err)
				}
				return fmt.Sprintf("🗑️ 已删除 '%s' 的密码", service)

			case "update":
				service := args["service"]
				username := args["username"]
				password := args["password"]
				if service == "" || password == "" {
					return "错误：update 需要 service 和 password 参数"
				}
				entry, exists := vaultData[service]
				if !exists {
					return fmt.Sprintf("❌ 未找到 '%s'，使用 add 添加", service)
				}
				entry["password"] = password
				if username != "" {
					entry["username"] = username
				}
				vaultData[service] = entry
				if err := saveVault(vaultPath, key[:], vaultData); err != nil {
					return fmt.Sprintf("错误：保存失败: %v", err)
				}
				return fmt.Sprintf("✅ 已更新 '%s' 的密码", service)

			default:
				return "错误：action 参数应为 add/get/list/delete/update/set_master"
			}
		},
	}
}
