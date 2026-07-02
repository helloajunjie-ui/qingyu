package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ============================================
// 极简 IMAP 客户端（纯 Go 标准库，零依赖）
// ============================================

type imapClient struct {
	conn net.Conn
	br   *bufio.Reader
	tag  int
}

func dialIMAP(addr string, timeout time.Duration) (*imapClient, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("连接失败: %w", err)
	}
	c := &imapClient{conn: conn, br: bufio.NewReader(conn), tag: 1}
	_, err = c.readline()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("读取欢迎消息失败: %w", err)
	}
	return c, nil
}

func dialIMAP_TLS(addr string, timeout time.Duration) (*imapClient, error) {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", addr, &tls.Config{
		InsecureSkipVerify: false,
	})
	if err != nil {
		return nil, fmt.Errorf("TLS 连接失败: %w", err)
	}
	c := &imapClient{conn: conn, br: bufio.NewReader(conn), tag: 1}
	_, err = c.readline()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("读取欢迎消息失败: %w", err)
	}
	return c, nil
}

func (c *imapClient) nextTag() string {
	c.tag++
	return fmt.Sprintf("a%03d", c.tag)
}

func (c *imapClient) readline() (string, error) {
	line, err := c.br.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func (c *imapClient) readLiteral(size int) (string, error) {
	buf := make([]byte, size)
	_, err := io.ReadFull(c.br, buf)
	if err != nil {
		return "", err
	}
	// 读取末尾的 \r\n
	c.readline()
	return string(buf), nil
}

func (c *imapClient) cmd(format string, args ...interface{}) ([]string, error) {
	tag := c.nextTag()
	line := fmt.Sprintf("%s "+format, append([]interface{}{tag}, args...)...)
	_, err := fmt.Fprintf(c.conn, "%s\r\n", line)
	if err != nil {
		return nil, fmt.Errorf("发送命令失败: %w", err)
	}

	var lines []string
	for {
		resp, err := c.readline()
		if err != nil {
			return nil, fmt.Errorf("读取响应失败: %w", err)
		}
		lines = append(lines, resp)
		if strings.HasPrefix(resp, tag) {
			break
		}
	}
	return lines, nil
}

func (c *imapClient) cmdFetch(seqSet, attrs string) ([]string, error) {
	tag := c.nextTag()
	line := fmt.Sprintf("%s FETCH %s %s", tag, seqSet, attrs)
	_, err := fmt.Fprintf(c.conn, "%s\r\n", line)
	if err != nil {
		return nil, fmt.Errorf("发送命令失败: %w", err)
	}

	var lines []string
	for {
		resp, err := c.readline()
		if err != nil {
			return nil, fmt.Errorf("读取响应失败: %w", err)
		}
		lines = append(lines, resp)

		// 检查是否包含 literal 数据
		if strings.Contains(resp, "{") && strings.Contains(resp, "}") {
			// 提取 literal 大小
			var size int
			if _, err := fmt.Sscanf(resp[strings.Index(resp, "{")+1:], "%d}", &size); err == nil && size > 0 {
				literal, err := c.readLiteral(size)
				if err != nil {
					return nil, fmt.Errorf("读取 literal 失败: %w", err)
				}
				lines = append(lines, literal)
			}
		}

		if strings.HasPrefix(resp, tag) {
			break
		}
	}
	return lines, nil
}

func (c *imapClient) login(user, pass string) error {
	lines, err := c.cmd("LOGIN %s %s", user, pass)
	if err != nil {
		return err
	}
	last := lines[len(lines)-1]
	if strings.HasPrefix(last, "a") && (strings.Contains(last, "NO") || strings.Contains(last, "BAD")) {
		return fmt.Errorf("登录失败: %s", last)
	}
	return nil
}

func (c *imapClient) selectMailbox(name string) (int, error) {
	lines, err := c.cmd("SELECT %s", name)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, l := range lines {
		if strings.Contains(l, "EXISTS") {
			fmt.Sscanf(l, "* %d EXISTS", &count)
		}
	}
	return count, nil
}

func (c *imapClient) fetch(seqSet, attrs string) ([]string, error) {
	return c.cmd("FETCH %s %s", seqSet, attrs)
}

func (c *imapClient) logout() {
	c.cmd("LOGOUT")
	c.conn.Close()
}

// ============================================
// SMTP 客户端（纯 Go 标准库）
// ============================================

type smtpClient struct {
	conn net.Conn
	br   *bufio.Reader
}

func dialSMTP_TLS(addr string, timeout time.Duration) (*smtpClient, error) {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", addr, &tls.Config{
		InsecureSkipVerify: false,
	})
	if err != nil {
		return nil, fmt.Errorf("TLS 连接失败: %w", err)
	}
	c := &smtpClient{conn: conn, br: bufio.NewReader(conn)}
	// 读取服务器 banner
	_, err = c.readline()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("读取 banner 失败: %w", err)
	}
	return c, nil
}

func (c *smtpClient) readline() (string, error) {
	line, err := c.br.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func (c *smtpClient) cmd(format string, args ...interface{}) (string, error) {
	line := fmt.Sprintf(format, args...)
	_, err := fmt.Fprintf(c.conn, "%s\r\n", line)
	if err != nil {
		return "", fmt.Errorf("发送命令失败: %w", err)
	}
	resp, err := c.readline()
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}
	return resp, nil
}

func (c *smtpClient) sendData(data []byte) error {
	_, err := fmt.Fprintf(c.conn, "DATA\r\n")
	if err != nil {
		return err
	}
	resp, err := c.readline()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resp, "354") {
		return fmt.Errorf("DATA 被拒: %s", resp)
	}

	// 发送邮件内容，行首.需要转义
	reader := bytes.NewReader(data)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, ".") {
			line = "." + line
		}
		_, err := fmt.Fprintf(c.conn, "%s\r\n", line)
		if err != nil {
			return err
		}
	}

	// 结束
	_, err = fmt.Fprintf(c.conn, ".\r\n")
	if err != nil {
		return err
	}
	resp, err = c.readline()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resp, "250") {
		return fmt.Errorf("发送数据被拒: %s", resp)
	}
	return nil
}

func (c *smtpClient) quit() {
	c.cmd("QUIT")
	c.conn.Close()
}

// ============================================
// 地址解析
// ============================================

func parseIMAPAddress(email string) (server string, port string) {
	email = strings.ToLower(email)
	switch {
	case strings.Contains(email, "gmail.com"):
		return "imap.gmail.com", "993"
	case strings.Contains(email, "qq.com"):
		return "imap.qq.com", "993"
	case strings.Contains(email, "163.com"):
		return "imap.163.com", "993"
	case strings.Contains(email, "outlook.com"), strings.Contains(email, "hotmail.com"):
		return "outlook.office365.com", "993"
	case strings.Contains(email, "yahoo.com"):
		return "imap.mail.yahoo.com", "993"
	case strings.Contains(email, "126.com"):
		return "imap.126.com", "993"
	case strings.Contains(email, "foxmail.com"):
		return "imap.qq.com", "993"
	}
	domain := email[strings.Index(email, "@")+1:]
	return "imap." + domain, "993"
}

func parseSMTPAddress(email string) (server string, port string) {
	email = strings.ToLower(email)
	switch {
	case strings.Contains(email, "gmail.com"):
		return "smtp.gmail.com", "465"
	case strings.Contains(email, "qq.com"):
		return "smtp.qq.com", "465"
	case strings.Contains(email, "163.com"):
		return "smtp.163.com", "465"
	case strings.Contains(email, "outlook.com"), strings.Contains(email, "hotmail.com"):
		return "smtp.office365.com", "587"
	case strings.Contains(email, "yahoo.com"):
		return "smtp.mail.yahoo.com", "465"
	case strings.Contains(email, "126.com"):
		return "smtp.126.com", "465"
	case strings.Contains(email, "foxmail.com"):
		return "smtp.qq.com", "465"
	}
	domain := email[strings.Index(email, "@")+1:]
	return "smtp." + domain, "465"
}

// ============================================
// MIME 邮件构建（纯 fmt.Fprintf，无 textproto）
// ============================================

// buildMail 构建带附件的邮件
func buildMail(from, to, subject, body string, attachments []string) ([]byte, error) {
	var buf bytes.Buffer

	// 编码主题
	encodedSubject := mime.QEncoding.Encode("utf-8", subject)

	// 写邮件头
	fmt.Fprintf(&buf, "From: %s\r\n", from)
	fmt.Fprintf(&buf, "To: %s\r\n", to)
	fmt.Fprintf(&buf, "Subject: %s\r\n", encodedSubject)
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))

	if len(attachments) == 0 {
		// 纯文本邮件
		fmt.Fprintf(&buf, "Content-Type: text/plain; charset=\"utf-8\"\r\n")
		fmt.Fprintf(&buf, "Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&buf, "\r\n")
		encoded := base64.StdEncoding.EncodeToString([]byte(body))
		for i := 0; i < len(encoded); i += 76 {
			end := i + 76
			if end > len(encoded) {
				end = len(encoded)
			}
			fmt.Fprintf(&buf, "%s\r\n", encoded[i:end])
		}
	} else {
		// 带附件的 multipart/mixed 邮件
		boundary := fmt.Sprintf("=_qingyu_%d", time.Now().UnixNano())
		fmt.Fprintf(&buf, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary)
		fmt.Fprintf(&buf, "\r\n")

		// 正文部分
		fmt.Fprintf(&buf, "--%s\r\n", boundary)
		fmt.Fprintf(&buf, "Content-Type: text/plain; charset=\"utf-8\"\r\n")
		fmt.Fprintf(&buf, "Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&buf, "\r\n")
		encoded := base64.StdEncoding.EncodeToString([]byte(body))
		for i := 0; i < len(encoded); i += 76 {
			end := i + 76
			if end > len(encoded) {
				end = len(encoded)
			}
			fmt.Fprintf(&buf, "%s\r\n", encoded[i:end])
		}
		fmt.Fprintf(&buf, "\r\n")

		// 附件部分
		for _, attachPath := range attachments {
			attachData, err := os.ReadFile(attachPath)
			if err != nil {
				return nil, fmt.Errorf("读取附件失败 %s: %w", attachPath, err)
			}

			fileName := filepath.Base(attachPath)
			ext := strings.ToLower(filepath.Ext(fileName))
			mimeType := "application/octet-stream"
			switch ext {
			case ".jpg", ".jpeg":
				mimeType = "image/jpeg"
			case ".png":
				mimeType = "image/png"
			case ".gif":
				mimeType = "image/gif"
			case ".pdf":
				mimeType = "application/pdf"
			case ".doc", ".docx":
				mimeType = "application/msword"
			case ".xls", ".xlsx":
				mimeType = "application/vnd.ms-excel"
			case ".zip":
				mimeType = "application/zip"
			case ".txt":
				mimeType = "text/plain"
			}

			encodedFileName := mime.QEncoding.Encode("utf-8", fileName)
			fmt.Fprintf(&buf, "--%s\r\n", boundary)
			fmt.Fprintf(&buf, "Content-Type: %s; name=\"%s\"\r\n", mimeType, encodedFileName)
			fmt.Fprintf(&buf, "Content-Transfer-Encoding: base64\r\n")
			fmt.Fprintf(&buf, "Content-Disposition: attachment; filename=\"%s\"\r\n", encodedFileName)
			fmt.Fprintf(&buf, "\r\n")
			encoded = base64.StdEncoding.EncodeToString(attachData)
			for i := 0; i < len(encoded); i += 76 {
				end := i + 76
				if end > len(encoded) {
					end = len(encoded)
				}
				fmt.Fprintf(&buf, "%s\r\n", encoded[i:end])
			}
			fmt.Fprintf(&buf, "\r\n")
		}

		fmt.Fprintf(&buf, "--%s--\r\n", boundary)
	}

	return buf.Bytes(), nil
}

// ============================================
// 邮件解析（收信正文+附件提取）
// ============================================

// parseEmailBody 解析邮件正文和附件
func parseEmailBody(raw []byte, saveDir string) (bodyText string, attachments []string, err error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		// 如果解析失败，尝试直接返回原始内容
		return string(raw), nil, nil
	}

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		// 无法解析 Content-Type，直接读 body
		b, _ := io.ReadAll(msg.Body)
		return string(b), nil, nil
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		mr := multipart.NewReader(msg.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}

			partMediaType, partParams, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
			contentDisposition := part.Header.Get("Content-Disposition")

			if strings.HasPrefix(contentDisposition, "attachment") || strings.HasPrefix(contentDisposition, "inline") {
				// 提取附件
				_, dispParams, _ := mime.ParseMediaType(contentDisposition)
				fileName := dispParams["filename"]
				if fileName == "" {
					fileName = partParams["name"]
				}
				if fileName == "" {
					fileName = fmt.Sprintf("attachment_%d", time.Now().UnixNano())
				}

				// 解码 MIME 编码的文件名
				if strings.Contains(fileName, "=?") {
					dec := mime.WordDecoder{}
					if decoded, err := dec.Decode(fileName); err == nil {
						fileName = decoded
					}
				}

				// 确保保存目录存在
				os.MkdirAll(saveDir, 0755)

				// 保存到 saveDir
				savePath := filepath.Join(saveDir, fileName)
				f, err := os.Create(savePath)
				if err != nil {
					continue
				}

				// 解码传输编码
				enc := part.Header.Get("Content-Transfer-Encoding")
				var reader io.Reader = part
				if strings.EqualFold(enc, "base64") {
					reader = base64.NewDecoder(base64.StdEncoding, part)
				} else if strings.EqualFold(enc, "quoted-printable") {
					reader = quotedprintable.NewReader(part)
				}

				io.Copy(f, reader)
				f.Close()
				attachments = append(attachments, savePath)
			} else {
				// 正文部分
				if strings.HasPrefix(partMediaType, "text/") {
					enc := part.Header.Get("Content-Transfer-Encoding")
					var reader io.Reader = part
					if strings.EqualFold(enc, "base64") {
						reader = base64.NewDecoder(base64.StdEncoding, part)
					} else if strings.EqualFold(enc, "quoted-printable") {
						reader = quotedprintable.NewReader(part)
					}
					b, _ := io.ReadAll(reader)
					if bodyText == "" {
						bodyText = string(b)
					}
				}
			}
		}
	} else {
		// 非 multipart，直接读
		b, _ := io.ReadAll(msg.Body)
		enc := msg.Header.Get("Content-Transfer-Encoding")
		if strings.EqualFold(enc, "base64") {
			decoded, err := base64.StdEncoding.DecodeString(string(b))
			if err == nil {
				bodyText = string(decoded)
			} else {
				bodyText = string(b)
			}
		} else if strings.EqualFold(enc, "quoted-printable") {
			decoded, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(b)))
			if err == nil {
				bodyText = string(decoded)
			} else {
				bodyText = string(b)
			}
		} else {
			bodyText = string(b)
		}
	}

	return bodyText, attachments, nil
}

// decodeMIMEText 解码 MIME 编码文本
func decodeMIMEText(s string) string {
	dec := mime.WordDecoder{}
	if decoded, err := dec.Decode(s); err == nil {
		return decoded
	}
	return s
}

// ============================================
// 工具注册
// ============================================

func init() {
	// ─── check_email: 查收邮件（支持正文+附件） ───
	Toolkit["check_email"] = Tool{
		Name:        "check_email",
		Description: "【查收邮件】通过 IMAP 检查邮箱最新邮件，支持读取正文和下载附件。参数: email (邮箱地址), password (密码/应用专用密码), count (可选，获取邮件数量，默认3), folder (可选，文件夹，默认INBOX), body (可选，是否读取正文，true/false，默认false), save_attachments (可选，是否保存附件到workspace，true/false，默认false)。Gmail 需使用应用专用密码。",
		Category:    "网络",
		Execute: func(args map[string]string) string {
			email := args["email"]
			password := args["password"]
			if email == "" || password == "" {
				return "❌ 需要 email 和 password 参数"
			}

			count := 3
			if args["count"] != "" {
				fmt.Sscanf(args["count"], "%d", &count)
				if count < 1 || count > 20 {
					count = 3
				}
			}

			folder := "INBOX"
			if args["folder"] != "" {
				folder = args["folder"]
			}

			readBody := strings.EqualFold(args["body"], "true")
			saveAttachments := strings.EqualFold(args["save_attachments"], "true")

			server, port := parseIMAPAddress(email)
			addr := fmt.Sprintf("%s:%s", server, port)
			timeout := time.Duration(GetSettings().Timeouts.IMAPSMTP) * time.Second

			client, err := dialIMAP_TLS(addr, timeout)
			if err != nil {
				return fmt.Sprintf("❌ 无法连接到 %s: %v", server, err)
			}
			defer client.logout()

			if err := client.login(email, password); err != nil {
				return fmt.Sprintf("❌ 登录失败: %v", err)
			}

			total, err := client.selectMailbox(folder)
			if err != nil {
				return fmt.Sprintf("❌ 无法打开文件夹 %s: %v", folder, err)
			}
			if total == 0 {
				return fmt.Sprintf("📭 邮箱 [%s] 的 %s 为空", email, folder)
			}

			start := total - count + 1
			if start < 1 {
				start = 1
			}
			seqSet := fmt.Sprintf("%d:%d", start, total)

			// 先获取邮件头列表
			lines, err := client.fetch(seqSet, "(FLAGS BODY[HEADER.FIELDS (FROM SUBJECT DATE)])")
			if err != nil {
				return fmt.Sprintf("❌ 获取邮件失败: %v", err)
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("📬 %s (%s) — 最新 %d 封邮件\n", email, folder, total-start+1))
			sb.WriteString(strings.Repeat("─", 40))
			sb.WriteString("\n")

			// 解析邮件头列表
			type emailSummary struct {
				seq     int
				from    string
				subject string
				date    string
			}
			var summaries []emailSummary

			for i := 0; i < len(lines); i++ {
				line := lines[i]
				if strings.HasPrefix(line, "* ") && strings.Contains(line, "FETCH") {
					var seq int
					fmt.Sscanf(line, "* %d FETCH", &seq)

					var from, subject, date string
					for j := i + 1; j < len(lines); j++ {
						sub := lines[j]
						if strings.HasPrefix(sub, "* ") || strings.HasPrefix(sub, "a") {
							break
						}
						sub = strings.TrimSpace(sub)
						if strings.HasPrefix(sub, "FROM:") || strings.HasPrefix(sub, "From:") {
							from = strings.TrimSpace(sub[5:])
						} else if strings.HasPrefix(sub, "SUBJECT:") || strings.HasPrefix(sub, "Subject:") {
							subject = strings.TrimSpace(sub[8:])
						} else if strings.HasPrefix(sub, "DATE:") || strings.HasPrefix(sub, "Date:") {
							date = strings.TrimSpace(sub[5:])
						}
					}
					summaries = append(summaries, emailSummary{seq, from, subject, date})
				}
			}

			for idx, s := range summaries {
				sb.WriteString(fmt.Sprintf("\n📧 #%d (seq:%d)\n", idx+1, s.seq))
				if s.from != "" {
					sb.WriteString(fmt.Sprintf("  发件人: %s\n", decodeMIMEText(s.from)))
				}
				if s.date != "" {
					sb.WriteString(fmt.Sprintf("  时间: %s\n", s.date))
				}
				if s.subject != "" {
					sb.WriteString(fmt.Sprintf("  主题: %s\n", decodeMIMEText(s.subject)))
				}
			}

			// 如果需要读取正文或保存附件
			if (readBody || saveAttachments) && len(summaries) > 0 {
				sb.WriteString("\n" + strings.Repeat("━", 40) + "\n")

				for idx, s := range summaries {
					// 获取完整邮件内容
					bodyLines, err := client.cmdFetch(fmt.Sprintf("%d", s.seq), "(BODY[])")
					if err != nil {
						sb.WriteString(fmt.Sprintf("\n⚠️ 无法读取邮件 #%d 内容: %v\n", idx+1, err))
						continue
					}

					// 提取 literal 数据
					var rawData []byte
					for _, l := range bodyLines {
						if strings.HasPrefix(l, "* ") && strings.Contains(l, "{") {
							continue
						}
						if strings.HasPrefix(l, "a") && strings.Contains(l, "FETCH") {
							continue
						}
						rawData = append(rawData, []byte(l)...)
						rawData = append(rawData, '\n')
					}

					sb.WriteString(fmt.Sprintf("\n📧 #%d 详情\n", idx+1))

					if readBody && len(rawData) > 0 {
						bodyText, attachments, err := parseEmailBody(rawData, filepath.Join(WorkDir, "attachments"))
						if err == nil {
							// 截断过长的正文
							if len(bodyText) > 1000 {
								bodyText = bodyText[:1000] + "\n... (内容过长已截断)"
							}
							if bodyText != "" {
								sb.WriteString(fmt.Sprintf("  正文:\n%s\n", bodyText))
							}
							if saveAttachments && len(attachments) > 0 {
								sb.WriteString(fmt.Sprintf("  📎 附件已保存 (%d 个):\n", len(attachments)))
								for _, a := range attachments {
									sb.WriteString(fmt.Sprintf("    - %s\n", a))
								}
							}
						}
					}
				}
			}

			sb.WriteString(fmt.Sprintf("\n📊 共 %d 封邮件", total))
			return sb.String()
		},
	}

	// ─── send_email: 发送邮件（支持附件） ───
	Toolkit["send_email"] = Tool{
		Name:        "send_email",
		Description: "【发送邮件】通过 SMTP 发送邮件，支持附件。参数: email (发件邮箱地址), password (密码/应用专用密码), to (收件人邮箱), subject (邮件主题), body (邮件正文), attachments (可选，附件路径，多个用逗号分隔)。Gmail 需使用应用专用密码。",
		Category:    "网络",
		Execute: func(args map[string]string) string {
			email := args["email"]
			password := args["password"]
			to := args["to"]
			subject := args["subject"]
			body := args["body"]

			if email == "" || password == "" || to == "" || subject == "" || body == "" {
				return "❌ 需要 email, password, to, subject, body 参数"
			}

			// 解析附件列表
			var attachments []string
			if args["attachments"] != "" {
				for _, a := range strings.Split(args["attachments"], ",") {
					a = strings.TrimSpace(a)
					if a != "" {
						if _, err := os.Stat(a); err != nil {
							return fmt.Sprintf("❌ 附件不存在: %s", a)
						}
						attachments = append(attachments, a)
					}
				}
			}

			// 构建邮件
			mailData, err := buildMail(email, to, subject, body, attachments)
			if err != nil {
				return fmt.Sprintf("❌ 构建邮件失败: %v", err)
			}

			// 连接 SMTP 服务器
			server, port := parseSMTPAddress(email)
			addr := fmt.Sprintf("%s:%s", server, port)
			timeout := 15 * time.Second

			client, err := dialSMTP_TLS(addr, timeout)
			if err != nil {
				return fmt.Sprintf("❌ 无法连接到 SMTP 服务器 %s: %v", server, err)
			}
			defer client.quit()

			// EHLO
			resp, err := client.cmd("EHLO qingyu")
			if err != nil {
				return fmt.Sprintf("❌ EHLO 失败: %v", err)
			}
			if !strings.HasPrefix(resp, "250") {
				return fmt.Sprintf("❌ EHLO 被拒: %s", resp)
			}

			// AUTH LOGIN
			resp, err = client.cmd("AUTH LOGIN")
			if err != nil {
				return fmt.Sprintf("❌ AUTH 失败: %v", err)
			}
			if !strings.HasPrefix(resp, "334") {
				return fmt.Sprintf("❌ AUTH 被拒: %s", resp)
			}

			// 发送用户名（Base64）
			resp, err = client.cmd(base64.StdEncoding.EncodeToString([]byte(email)))
			if err != nil {
				return fmt.Sprintf("❌ 发送用户名失败: %v", err)
			}
			if !strings.HasPrefix(resp, "334") {
				return fmt.Sprintf("❌ 用户名被拒: %s", resp)
			}

			// 发送密码（Base64）
			resp, err = client.cmd(base64.StdEncoding.EncodeToString([]byte(password)))
			if err != nil {
				return fmt.Sprintf("❌ 发送密码失败: %v", err)
			}
			if !strings.HasPrefix(resp, "235") {
				return fmt.Sprintf("❌ 认证失败: %s\n提示：Gmail 需使用应用专用密码，QQ邮箱需使用授权码", resp)
			}

			// MAIL FROM
			resp, err = client.cmd("MAIL FROM:<%s>", email)
			if err != nil {
				return fmt.Sprintf("❌ MAIL FROM 失败: %v", err)
			}
			if !strings.HasPrefix(resp, "250") {
				return fmt.Sprintf("❌ MAIL FROM 被拒: %s", resp)
			}

			// RCPT TO
			resp, err = client.cmd("RCPT TO:<%s>", to)
			if err != nil {
				return fmt.Sprintf("❌ RCPT TO 失败: %v", err)
			}
			if !strings.HasPrefix(resp, "250") {
				return fmt.Sprintf("❌ RCPT TO 被拒: %s", resp)
			}

			// DATA
			if err := client.sendData(mailData); err != nil {
				return fmt.Sprintf("❌ 发送邮件数据失败: %v", err)
			}

			attachInfo := ""
			if len(attachments) > 0 {
				attachInfo = fmt.Sprintf(" 📎 %d 个附件", len(attachments))
			}
			return fmt.Sprintf("✅ 邮件已发送至 %s\n  主题: %s%s", to, subject, attachInfo)
		},
	}
}
