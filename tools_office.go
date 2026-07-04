// Office 文档工具集
//
// 提供 Word (.docx)、Excel (.xlsx)、PowerPoint (.pptx) 文档的
// 读取、创建、修改功能。基于纯 Go 标准库 zip/xml 解析，零外部依赖。
// 所有工具通过 init() 注册到全局 Toolkit。
package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func init() {
	// read_docx: 读取 Word (.docx) 文件文本内容
	Toolkit["read_docx"] = Tool{
		Name:        "read_docx",
		Description: "读取 Word 文档 (.docx) 的文本内容。参数: path (文件路径)",
		Category:    "文档",
		Execute: func(args map[string]string) string {
			path := args["path"]
			if path == "" {
				return "错误：未提供文件路径"
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(RootDir, path)
			}

			text, err := extractDocxText(path)
			if err != nil {
				return fmt.Sprintf("读取失败: %v", err)
			}
			if len(text) > 3000 {
				text = text[:3000] + "\n\n... (内容过长，已截断)"
			}
			return fmt.Sprintf("📄 Word 文档 [%s] 内容:\n%s", path, text)
		},
	}

	// read_pptx: 读取 PowerPoint (.pptx) 文件文本内容
	Toolkit["read_pptx"] = Tool{
		Name:        "read_pptx",
		Description: "读取 PowerPoint 演示文稿 (.pptx) 的文本内容。参数: path (文件路径)",
		Category:    "文档",
		Execute: func(args map[string]string) string {
			path := args["path"]
			if path == "" {
				return "错误：未提供文件路径"
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(RootDir, path)
			}

			text, err := extractPptxText(path)
			if err != nil {
				return fmt.Sprintf("读取失败: %v", err)
			}
			if len(text) > 3000 {
				text = text[:3000] + "\n\n... (内容过长，已截断)"
			}
			return fmt.Sprintf("📽️ PowerPoint 文档 [%s] 内容:\n%s", path, text)
		},
	}

	// read_xlsx: 读取 Excel (.xlsx) 文件内容为 CSV 格式
	Toolkit["read_xlsx"] = Tool{
		Name:        "read_xlsx",
		Description: "读取 Excel 工作簿 (.xlsx) 的内容，返回 CSV 格式文本。参数: path (文件路径), sheet (可选，工作表名称，默认第一个)",
		Category:    "文档",
		Execute: func(args map[string]string) string {
			path := args["path"]
			if path == "" {
				return "错误：未提供文件路径"
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(RootDir, path)
			}

			sheetName := args["sheet"]
			text, err := extractXlsxText(path, sheetName)
			if err != nil {
				return fmt.Sprintf("读取失败: %v", err)
			}
			if len(text) > 3000 {
				text = text[:3000] + "\n\n... (内容过长，已截断)"
			}
			return fmt.Sprintf("📊 Excel 文档 [%s] 内容:\n%s", path, text)
		},
	}

	// create_docx: 创建简单的 Word 文档
	Toolkit["create_docx"] = Tool{
		Name:        "create_docx",
		Description: "创建一个简单的 Word 文档 (.docx)。参数: filename (文件名，保存到 workspace), title (文档标题), content (正文内容，支持多行)",
		Category:    "文档",
		Execute: func(args map[string]string) string {
			filename := args["filename"]
			title := args["title"]
			content := args["content"]

			if filename == "" {
				return "错误：未提供文件名"
			}
			if !strings.HasSuffix(strings.ToLower(filename), ".docx") {
				filename += ".docx"
			}

			safePath := filepath.Join(RootDir, WorkspaceDir, filepath.Base(filename))

			if err := createMinimalDocx(safePath, title, content); err != nil {
				return fmt.Sprintf("创建失败: %v", err)
			}

			return fmt.Sprintf("✅ Word 文档已创建: %s (%d 字符内容)", safePath, len(content))
		},
	}

	// create_xlsx: 创建简单的 Excel 工作簿
	Toolkit["create_xlsx"] = Tool{
		Name:        "create_xlsx",
		Description: "创建一个简单的 Excel 工作簿 (.xlsx)。参数: filename (文件名，保存到 workspace), data (CSV 格式数据，第一行为表头), sheet_name (可选，工作表名称，默认 Sheet1)",
		Category:    "文档",
		Execute: func(args map[string]string) string {
			filename := args["filename"]
			data := args["data"]
			sheetName := args["sheet_name"]

			if filename == "" {
				return "错误：未提供文件名"
			}
			if data == "" {
				return "错误：未提供数据"
			}
			if !strings.HasSuffix(strings.ToLower(filename), ".xlsx") {
				filename += ".xlsx"
			}
			if sheetName == "" {
				sheetName = "Sheet1"
			}

			safePath := filepath.Join(RootDir, WorkspaceDir, filepath.Base(filename))

			if err := createMinimalXlsx(safePath, sheetName, data); err != nil {
				return fmt.Sprintf("创建失败: %v", err)
			}

			return fmt.Sprintf("✅ Excel 工作簿已创建: %s", safePath)
		},
	}

	// docx_to_txt: Word 转纯文本
	Toolkit["docx_to_txt"] = Tool{
		Name:        "docx_to_txt",
		Description: "将 Word 文档 (.docx) 转换为纯文本文件 (.txt)。参数: path (源文件路径), output (可选，输出文件名，默认同名的 .txt)",
		Category:    "文档",
		Execute: func(args map[string]string) string {
			path := args["path"]
			if path == "" {
				return "错误：未提供源文件路径"
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(RootDir, path)
			}

			output := args["output"]
			if output == "" {
				output = strings.TrimSuffix(path, filepath.Ext(path)) + ".txt"
			}
			if !filepath.IsAbs(output) {
				output = filepath.Join(RootDir, WorkspaceDir, output)
			}

			text, err := extractDocxText(path)
			if err != nil {
				return fmt.Sprintf("转换失败: %v", err)
			}

			if err := os.WriteFile(output, []byte(text), 0644); err != nil {
				return fmt.Sprintf("写入输出文件失败: %v", err)
			}

			return fmt.Sprintf("✅ 转换完成: %s → %s (%d 字符)", path, output, len(text))
		},
	}

	// xlsx_to_csv: Excel 转 CSV
	Toolkit["xlsx_to_csv"] = Tool{
		Name:        "xlsx_to_csv",
		Description: "将 Excel 工作簿 (.xlsx) 的指定工作表转换为 CSV 文件。参数: path (源文件路径), sheet (可选，工作表名称，默认第一个), output (可选，输出文件名，默认同名的 .csv)",
		Category:    "文档",
		Execute: func(args map[string]string) string {
			path := args["path"]
			if path == "" {
				return "错误：未提供源文件路径"
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(RootDir, path)
			}

			sheetName := args["sheet"]
			output := args["output"]
			if output == "" {
				output = strings.TrimSuffix(path, filepath.Ext(path)) + ".csv"
			}
			if !filepath.IsAbs(output) {
				output = filepath.Join(RootDir, WorkspaceDir, output)
			}

			text, err := extractXlsxText(path, sheetName)
			if err != nil {
				return fmt.Sprintf("转换失败: %v", err)
			}

			if err := os.WriteFile(output, []byte(text), 0644); err != nil {
				return fmt.Sprintf("写入输出文件失败: %v", err)
			}

			return fmt.Sprintf("✅ 转换完成: %s → %s", path, output)
		},
	}

	// open_document: 用系统默认程序打开文档
	Toolkit["open_document"] = Tool{
		Name:        "open_document",
		Description: "用系统默认程序打开 Office 文档（Word/Excel/PowerPoint/PDF 等）。参数: path (文件路径)",
		Category:    "文档",
		Execute: func(args map[string]string) string {
			path := args["path"]
			if path == "" {
				return "错误：未提供文件路径"
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(RootDir, path)
			}

			if _, err := os.Stat(path); os.IsNotExist(err) {
				return fmt.Sprintf("文件不存在: %s", path)
			}

			cmd := exec.Command("cmd", "/c", "start", "", path)
			if err := cmd.Start(); err != nil {
				return fmt.Sprintf("打开失败: %v", err)
			}

			return fmt.Sprintf("📂 已打开: %s", path)
		},
	}

	// edit_docx: 修改已有 Word 文档内容（追加或替换）
	// 策略：解包 docx → 修改 document.xml → 重新打包
	Toolkit["edit_docx"] = Tool{
		Name:        "edit_docx",
		Description: "修改已有的 Word 文档 (.docx) 内容。参数: path (文件路径), mode (操作模式: append=末尾追加, replace=全文替换), content (新内容)",
		Category:    "文档",
		Execute: func(args map[string]string) string {
			path := args["path"]
			mode := args["mode"]
			content := args["content"]

			if path == "" || content == "" {
				return "错误：需要提供 path 和 content 参数"
			}
			if mode == "" {
				mode = "append"
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(RootDir, path)
			}

			if _, err := os.Stat(path); os.IsNotExist(err) {
				return fmt.Sprintf("文件不存在: %s", path)
			}

			if err := modifyDocx(path, mode, content); err != nil {
				return fmt.Sprintf("修改失败: %v", err)
			}

			return fmt.Sprintf("✅ Word 文档已修改 [%s]: %s", mode, path)
		},
	}

	// edit_xlsx: 修改已有 Excel 工作簿（追加行或替换整个工作表）
	Toolkit["edit_xlsx"] = Tool{
		Name:        "edit_xlsx",
		Description: "修改已有的 Excel 工作簿 (.xlsx)。参数: path (文件路径), mode (操作模式: append=末尾追加行, replace=替换整个工作表), data (CSV 格式数据), sheet (可选，工作表名称，默认第一个)",
		Category:    "文档",
		Execute: func(args map[string]string) string {
			path := args["path"]
			mode := args["mode"]
			data := args["data"]
			sheetName := args["sheet"]

			if path == "" || data == "" {
				return "错误：需要提供 path 和 data 参数"
			}
			if mode == "" {
				mode = "append"
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(RootDir, path)
			}

			if _, err := os.Stat(path); os.IsNotExist(err) {
				return fmt.Sprintf("文件不存在: %s", path)
			}

			if err := modifyXlsx(path, mode, sheetName, data); err != nil {
				return fmt.Sprintf("修改失败: %v", err)
			}

			return fmt.Sprintf("✅ Excel 工作簿已修改 [%s]: %s", mode, path)
		},
	}
}

// ============================================
// docx 解析：从 ZIP 中提取 word/document.xml 的文本
// ============================================

type docxDocument struct {
	XMLName xml.Name `xml:"http://schemas.openxmlformats.org/wordprocessingml/2006/main document"`
	Body    docxBody `xml:"body"`
}

type docxBody struct {
	Paragraphs []docxParagraph `xml:"p"`
}

type docxParagraph struct {
	Runs []docxRun `xml:"r"`
}

type docxRun struct {
	Text string `xml:"t"`
}

func extractDocxText(path string) (string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("无法打开 docx 文件: %v", err)
	}
	defer r.Close()

	var docXML []byte
	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", fmt.Errorf("无法读取 document.xml: %v", err)
			}
			docXML, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return "", fmt.Errorf("读取 document.xml 失败: %v", err)
			}
			break
		}
	}

	if docXML == nil {
		return "", fmt.Errorf("未找到 word/document.xml，文件可能已损坏")
	}

	var doc docxDocument
	if err := xml.Unmarshal(docXML, &doc); err != nil {
		return "", fmt.Errorf("解析 document.xml 失败: %v", err)
	}

	var sb strings.Builder
	for _, p := range doc.Body.Paragraphs {
		for _, r := range p.Runs {
			sb.WriteString(r.Text)
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// ============================================
// pptx 解析：从 ZIP 中提取幻灯片文本
// ============================================

func extractPptxText(path string) (string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("无法打开 pptx 文件: %v", err)
	}
	defer r.Close()

	var sb strings.Builder
	slideCount := 0

	for _, f := range r.File {
		if strings.HasPrefix(f.Name, "ppt/slides/slide") && strings.HasSuffix(f.Name, ".xml") {
			rc, err := f.Open()
			if err != nil {
				continue
			}

			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}

			slideCount++
			sb.WriteString(fmt.Sprintf("--- 幻灯片 %d ---\n", slideCount))

			text := extractTextFromSlideXML(string(data))
			sb.WriteString(text)
			sb.WriteString("\n")
		}
	}

	if slideCount == 0 {
		return "", fmt.Errorf("未找到任何幻灯片")
	}

	return sb.String(), nil
}

// extractTextFromSlideXML 从 slide XML 中提取所有 a:t 标签文本
func extractTextFromSlideXML(xmlData string) string {
	var result strings.Builder
	startTag := "<a:t>"
	endTag := "</a:t>"

	remaining := xmlData
	for {
		startIdx := strings.Index(remaining, startTag)
		if startIdx == -1 {
			break
		}
		contentStart := startIdx + len(startTag)
		endIdx := strings.Index(remaining[contentStart:], endTag)
		if endIdx == -1 {
			break
		}
		content := remaining[contentStart : contentStart+endIdx]
		content = strings.ReplaceAll(content, "&", "&")
		content = strings.ReplaceAll(content, "<", "<")
		content = strings.ReplaceAll(content, ">", ">")
		result.WriteString(content)
		remaining = remaining[contentStart+endIdx+len(endTag):]
	}

	return result.String()
}

// ============================================
// xlsx 解析：从 ZIP 中提取工作表数据
// ============================================

type xlsxSharedStrings struct {
	XMLName xml.Name         `xml:"http://schemas.openxmlformats.org/spreadsheetml/2006/main sst"`
	Items   []xlsxSharedItem `xml:"si"`
}

type xlsxSharedItem struct {
	Text string `xml:"t"`
}

type xlsxWorkbook struct {
	XMLName xml.Name   `xml:"http://schemas.openxmlformats.org/spreadsheetml/2006/main workbook"`
	Sheets  xlsxSheets `xml:"sheets"`
}

type xlsxSheets struct {
	SheetList []xlsxSheetRef `xml:"sheet"`
}

type xlsxSheetRef struct {
	Name string `xml:"name,attr"`
	ID   string `xml:"sheetId,attr"`
	RID  string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
}

func extractXlsxText(path, sheetName string) (string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("无法打开 xlsx 文件: %v", err)
	}
	defer r.Close()

	// 1. 读取 sharedStrings
	sharedStrings := make(map[int]string)
	for _, f := range r.File {
		if f.Name == "xl/sharedStrings.xml" {
			rc, err := f.Open()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(rc)
			rc.Close()

			var ss xlsxSharedStrings
			if xml.Unmarshal(data, &ss) == nil {
				for i, item := range ss.Items {
					sharedStrings[i] = item.Text
				}
			}
			break
		}
	}

	// 2. 查找目标 sheet — 通过 workbook.xml.rels 将 RID 映射到文件名
	targetSheetFile := "xl/worksheets/sheet1.xml"
	if sheetName != "" {
		// 先获取 sheet 名称到 RID 的映射
		sheetRID := ""
		for _, f := range r.File {
			if f.Name == "xl/workbook.xml" {
				rc, err := f.Open()
				if err != nil {
					break
				}
				data, _ := io.ReadAll(rc)
				rc.Close()

				var wb xlsxWorkbook
				if xml.Unmarshal(data, &wb) == nil {
					for _, s := range wb.Sheets.SheetList {
						if s.Name == sheetName {
							sheetRID = s.RID
							break
						}
					}
				}
				break
			}
		}
		// 通过 workbook.xml.rels 将 RID 解析为实际文件路径
		if sheetRID != "" {
			for _, f := range r.File {
				if f.Name == "xl/_rels/workbook.xml.rels" {
					rc, err := f.Open()
					if err != nil {
						break
					}
					data, _ := io.ReadAll(rc)
					rc.Close()

					type relEntry struct {
						ID     string `xml:"Id,attr"`
						Target string `xml:"Target,attr"`
					}
					type rels struct {
						Relationships []relEntry `xml:"Relationship"`
					}
					var rl rels
					if xml.Unmarshal(data, &rl) == nil {
						for _, rel := range rl.Relationships {
							if rel.ID == sheetRID {
								targetSheetFile = filepath.Join("xl/worksheets", rel.Target)
								break
							}
						}
					}
					break
				}
			}
		}
	}

	// 3. 读取目标 sheet XML
	var sb strings.Builder
	for _, f := range r.File {
		if f.Name != targetSheetFile {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(rc)
		rc.Close()

		var ws struct {
			Rows []struct {
				Cells []struct {
					Type  string `xml:"t,attr"`
					Value string `xml:"v"`
				} `xml:"c"`
			} `xml:"sheetData>row"`
		}

		if xml.Unmarshal(data, &ws) != nil {
			continue
		}

		for _, row := range ws.Rows {
			rowVals := make([]string, 0, len(row.Cells))
			for _, cell := range row.Cells {
				val := cell.Value
				if cell.Type == "s" {
					idx := 0
					fmt.Sscanf(val, "%d", &idx)
					if s, ok := sharedStrings[idx]; ok {
						val = s
					}
				}
				rowVals = append(rowVals, val)
			}
			sb.WriteString(strings.Join(rowVals, ","))
			sb.WriteString("\n")
		}

		break
	}

	if sb.Len() == 0 {
		return "", fmt.Errorf("未找到工作表数据")
	}

	return sb.String(), nil
}

// ============================================
// 创建最小 docx 文件
// ============================================

func createMinimalDocx(path, title, content string) error {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	writeZipFile(w, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`)

	writeZipFile(w, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`)

	writeZipFile(w, "word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`)

	// 将内容按行分割为段落
	lines := strings.Split(content, "\n")
	var paraXML strings.Builder
	for _, line := range lines {
		if line == "" {
			line = " "
		}
		paraXML.WriteString(fmt.Sprintf(`<w:p><w:r><w:t>%s</w:t></w:r></w:p>`, xmlEscape(line)))
	}

	docXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:pPr><w:rPr><w:b/><w:sz w:val="28"/></w:rPr></w:pPr><w:r><w:rPr><w:b/><w:sz w:val="28"/></w:rPr><w:t>%s</w:t></w:r></w:p>
    %s
  </w:body>
</w:document>`, xmlEscape(title), paraXML.String())
	writeZipFile(w, "word/document.xml", docXML)

	writeZipFile(w, "word/styles.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:type="paragraph" w:default="1" w:styleId="Normal">
    <w:name w:val="Normal"/>
    <w:rPr><w:sz w:val="22"/><w:rFonts w:ascii="等线" w:hAnsi="等线"/></w:rPr>
  </w:style>
</w:styles>`)

	w.Close()
	return os.WriteFile(path, buf.Bytes(), 0644)
}

// ============================================
// 创建最小 xlsx 文件
// ============================================

func createMinimalXlsx(path, sheetName, csvData string) error {
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)

	writeZipFile(w, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>
</Types>`)

	writeZipFile(w, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`)

	writeZipFile(w, "xl/_rels/workbook.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/>
</Relationships>`)

	wbXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheets><sheet name="%s" sheetId="1" r:id="rId1" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"/></sheets>
</workbook>`, xmlEscape(sheetName))
	writeZipFile(w, "xl/workbook.xml", wbXML)

	// 解析 CSV 数据
	reader := csv.NewReader(strings.NewReader(csvData))
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("CSV 解析失败: %v", err)
	}

	var allTexts []string
	var sheetRows strings.Builder

	colLetter := func(i int) string {
		return string(rune('A' + i))
	}

	for rowIdx, record := range records {
		sheetRows.WriteString("<row>")
		for colIdx, cellVal := range record {
			ref := fmt.Sprintf("%s%d", colLetter(colIdx), rowIdx+1)
			strIdx := len(allTexts)
			allTexts = append(allTexts, cellVal)
			sheetRows.WriteString(fmt.Sprintf(`<c r="%s" t="s"><v>%d</v></c>`, ref, strIdx))
		}
		sheetRows.WriteString("</row>")
	}

	sheetXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>%s</sheetData>
</worksheet>`, sheetRows.String())
	writeZipFile(w, "xl/worksheets/sheet1.xml", sheetXML)

	var ssItems strings.Builder
	for _, s := range allTexts {
		ssItems.WriteString(fmt.Sprintf(`<si><t>%s</t></si>`, xmlEscape(s)))
	}
	ssXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="%d" uniqueCount="%d">%s</sst>`,
		len(allTexts), len(allTexts), ssItems.String())
	writeZipFile(w, "xl/sharedStrings.xml", ssXML)

	w.Close()
	return os.WriteFile(path, buf.Bytes(), 0644)
}

// ============================================
// modifyDocx: 修改已有 Word 文档
// mode: append → 在末尾追加段落, replace → 全文替换
// ============================================

func modifyDocx(path, mode, content string) error {
	// 1. 读取原始 docx
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("无法打开文件: %v", err)
	}
	defer r.Close()

	// 2. 读取所有文件到内存
	type zipEntry struct {
		name string
		data []byte
	}
	var entries []zipEntry

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		entries = append(entries, zipEntry{name: f.Name, data: data})
	}
	r.Close()

	// 3. 修改 document.xml
	lines := strings.Split(content, "\n")
	var newParaXML strings.Builder
	for _, line := range lines {
		if line == "" {
			line = " "
		}
		newParaXML.WriteString(fmt.Sprintf(`<w:p><w:r><w:t>%s</w:t></w:r></w:p>`, xmlEscape(line)))
	}

	for i, e := range entries {
		if e.name == "word/document.xml" {
			docStr := string(e.data)

			if mode == "replace" {
				// 替换 body 中所有内容
				bodyStart := strings.Index(docStr, "<w:body>")
				bodyEnd := strings.Index(docStr, "</w:body>")
				if bodyStart == -1 || bodyEnd == -1 {
					return fmt.Errorf("无法解析文档结构")
				}
				docStr = docStr[:bodyStart+8] + newParaXML.String() + docStr[bodyEnd:]
			} else {
				// append: 在 </w:body> 前插入
				bodyEnd := strings.Index(docStr, "</w:body>")
				if bodyEnd == -1 {
					return fmt.Errorf("无法解析文档结构")
				}
				docStr = docStr[:bodyEnd] + newParaXML.String() + docStr[bodyEnd:]
			}

			entries[i].data = []byte(docStr)
			break
		}
	}

	// 4. 重新打包
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	for _, e := range entries {
		fw, err := w.Create(e.name)
		if err != nil {
			continue
		}
		fw.Write(e.data)
	}
	w.Close()

	return os.WriteFile(path, buf.Bytes(), 0644)
}

// ============================================
// modifyXlsx: 修改已有 Excel 工作簿
// mode: append → 追加行, replace → 替换整个工作表
// ============================================

func modifyXlsx(path, mode, sheetName, csvData string) error {
	// 1. 读取原始 xlsx
	r, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("无法打开文件: %v", err)
	}
	defer r.Close()

	type zipEntry struct {
		name string
		data []byte
	}
	var entries []zipEntry

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		entries = append(entries, zipEntry{name: f.Name, data: data})
	}
	r.Close()

	// 2. 解析 CSV 新数据
	reader := csv.NewReader(strings.NewReader(csvData))
	newRecords, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("CSV 解析失败: %v", err)
	}

	// 3. 找到目标 sheet 并修改 — 通过 workbook.xml.rels 将 RID 映射到文件名
	targetSheet := "xl/worksheets/sheet1.xml"
	if sheetName != "" {
		// 从 workbook.xml 查找 sheet 名称对应的 RID
		sheetRID := ""
		for _, e := range entries {
			if e.name == "xl/workbook.xml" {
				var wb xlsxWorkbook
				if xml.Unmarshal(e.data, &wb) == nil {
					for _, s := range wb.Sheets.SheetList {
						if s.Name == sheetName {
							sheetRID = s.RID
							break
						}
					}
				}
				break
			}
		}
		// 通过 workbook.xml.rels 将 RID 解析为实际文件路径
		if sheetRID != "" {
			for _, e := range entries {
				if e.name == "xl/_rels/workbook.xml.rels" {
					type relEntry struct {
						ID     string `xml:"Id,attr"`
						Target string `xml:"Target,attr"`
					}
					type rels struct {
						Relationships []relEntry `xml:"Relationship"`
					}
					var rl rels
					if xml.Unmarshal(e.data, &rl) == nil {
						for _, rel := range rl.Relationships {
							if rel.ID == sheetRID {
								targetSheet = filepath.Join("xl/worksheets", rel.Target)
								break
							}
						}
					}
					break
				}
			}
		}
	}

	// 4. 读取 sharedStrings
	sharedStrings := make(map[int]string)
	nextStrIdx := 0
	for i, e := range entries {
		if e.name == "xl/sharedStrings.xml" {
			var ss xlsxSharedStrings
			if xml.Unmarshal(e.data, &ss) == nil {
				for j, item := range ss.Items {
					sharedStrings[j] = item.Text
					nextStrIdx = j + 1
				}
			}

			// 追加新的 shared strings
			var newTexts []string
			for _, record := range newRecords {
				for _, cellVal := range record {
					newTexts = append(newTexts, cellVal)
				}
			}

			var ssItems strings.Builder
			// 保留原有
			for k := 0; k < nextStrIdx; k++ {
				if s, ok := sharedStrings[k]; ok {
					ssItems.WriteString(fmt.Sprintf(`<si><t>%s</t></si>`, xmlEscape(s)))
				}
			}
			// 追加新
			for _, s := range newTexts {
				ssItems.WriteString(fmt.Sprintf(`<si><t>%s</t></si>`, xmlEscape(s)))
			}

			totalCount := nextStrIdx + len(newTexts)
			ssXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="%d" uniqueCount="%d">%s</sst>`,
				totalCount, totalCount, ssItems.String())
			entries[i].data = []byte(ssXML)
			break
		}
	}

	// 5. 修改 sheet XML
	colLetter := func(i int) string {
		return string(rune('A' + i))
	}

	for i, e := range entries {
		if e.name == targetSheet {
			var newRows strings.Builder

			if mode == "replace" {
				// 替换：直接用新数据，使用累积索引避免行列数不一致问题
				curStrIdx := nextStrIdx
				for rowIdx, record := range newRecords {
					newRows.WriteString("<row>")
					for colIdx := range record {
						ref := fmt.Sprintf("%s%d", colLetter(colIdx), rowIdx+1)
						newRows.WriteString(fmt.Sprintf(`<c r="%s" t="s"><v>%d</v></c>`, ref, curStrIdx))
						curStrIdx++
					}
					newRows.WriteString("</row>")
				}
			} else {
				// append：保留原有行，追加新行
				// 先解析原有行数以确定起始行号
				var ws struct {
					Rows []struct {
						Cells []struct {
							Type  string `xml:"t,attr"`
							Value string `xml:"v"`
						} `xml:"c"`
					} `xml:"sheetData>row"`
				}
				existingRowCount := 0
				if xml.Unmarshal(e.data, &ws) == nil {
					existingRowCount = len(ws.Rows)
				}

				// 保留原有 sheetData 内容
				sheetStr := string(e.data)
				sheetDataEnd := strings.LastIndex(sheetStr, "</sheetData>")
				if sheetDataEnd == -1 {
					return fmt.Errorf("无法解析工作表结构")
				}

				// 在 </sheetData> 前插入新行，使用累积索引
				curStrIdx := nextStrIdx
				for rowIdx, record := range newRecords {
					newRows.WriteString("<row>")
					for colIdx := range record {
						ref := fmt.Sprintf("%s%d", colLetter(colIdx), existingRowCount+rowIdx+1)
						newRows.WriteString(fmt.Sprintf(`<c r="%s" t="s"><v>%d</v></c>`, ref, curStrIdx))
						curStrIdx++
					}
					newRows.WriteString("</row>")
				}

				sheetStr = sheetStr[:sheetDataEnd] + newRows.String() + sheetStr[sheetDataEnd:]
				entries[i].data = []byte(sheetStr)
				break
			}

			// replace 模式：重建 sheet XML
			sheetXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>%s</sheetData>
</worksheet>`, newRows.String())
			entries[i].data = []byte(sheetXML)
			break
		}
	}

	// 6. 重新打包
	buf := new(bytes.Buffer)
	w := zip.NewWriter(buf)
	for _, e := range entries {
		fw, err := w.Create(e.name)
		if err != nil {
			continue
		}
		fw.Write(e.data)
	}
	w.Close()

	return os.WriteFile(path, buf.Bytes(), 0644)
}

// ============================================
// 辅助函数
// ============================================

func writeZipFile(w *zip.Writer, name, content string) {
	fw, err := w.Create(name)
	if err != nil {
		return
	}
	fw.Write([]byte(content))
}

func xmlEscape(s string) string {
	s = strings.NewReplacer(
		"\x26", "\x26amp;",
		"\x3c", "\x26lt;",
		"\x3e", "\x26gt;",
		"'", "\x26apos;",
		`"`, "\x26quot;",
	).Replace(s)
	return s
}

// ===== 【拓展工具集迭代】office_table_quick_parse — 轻量化表格解析 =====
func init() {
	Toolkit["office_table_quick_parse"] = Tool{
		Name:        "office_table_quick_parse",
		Description: "【本地整理】快速解析表格文件（CSV/TSV/简单表格文本），返回结构化数据。参数: path (文件路径), delimiter (分隔符: comma/tab/space/auto,默认auto), has_header (是否有表头: true/false,默认true), max_rows (最大行数,默认50)",
		Category:    "实用",
		Execute: func(args map[string]string) string {
			path := args["path"]
			if path == "" {
				return "❌ 请提供文件路径"
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(RootDir, path)
			}

			delimiter := args["delimiter"]
			if delimiter == "" {
				delimiter = "auto"
			}
			hasHeader := args["has_header"] != "false"
			maxRows := 50
			if n := args["max_rows"]; n != "" {
				fmt.Sscanf(n, "%d", &maxRows)
			}
			if maxRows < 1 || maxRows > 200 {
				maxRows = 50
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Sprintf("❌ 读取文件失败: %v", err)
			}

			content := string(data)
			lines := strings.Split(strings.TrimSpace(content), "\n")
			if len(lines) == 0 {
				return "❌ 文件为空"
			}

			// 自动检测分隔符
			detectDelim := func(line string) string {
				commas := strings.Count(line, ",")
				tabs := strings.Count(line, "\t")
				spaces := strings.Count(line, " ")
				if commas >= tabs && commas >= spaces && commas > 0 {
					return ","
				}
				if tabs >= commas && tabs >= spaces && tabs > 0 {
					return "\t"
				}
				if spaces >= commas && spaces >= tabs && spaces > 0 {
					return " "
				}
				return ","
			}

			sep := delimiter
			switch delimiter {
			case "comma":
				sep = ","
			case "tab":
				sep = "\t"
			case "space":
				sep = " "
			case "auto":
				sep = detectDelim(lines[0])
			}

			// 解析
			parseLine := func(line string) []string {
				var fields []string
				if sep == "," {
					// 简单 CSV 解析（支持引号）
					inQuote := false
					current := strings.Builder{}
					for _, ch := range line {
						switch {
						case ch == '"':
							inQuote = !inQuote
						case ch == ',' && !inQuote:
							fields = append(fields, strings.TrimSpace(current.String()))
							current.Reset()
						default:
							current.WriteRune(ch)
						}
					}
					fields = append(fields, strings.TrimSpace(current.String()))
				} else {
					fields = strings.Split(line, sep)
					for i := range fields {
						fields[i] = strings.TrimSpace(fields[i])
					}
				}
				return fields
			}

			startRow := 0
			var headers []string
			if hasHeader && len(lines) > 0 {
				headers = parseLine(lines[0])
				startRow = 1
			}

			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("📊 表格解析: %s\n", path))
			sb.WriteString(fmt.Sprintf("  分隔符: %s\n", sep))
			sb.WriteString(fmt.Sprintf("  总行数: %d\n", len(lines)))
			if hasHeader && len(headers) > 0 {
				sb.WriteString(fmt.Sprintf("  列数: %d\n", len(headers)))
				sb.WriteString(fmt.Sprintf("  表头: %s\n", strings.Join(headers, " | ")))
			}

			// 显示数据行
			displayRows := maxRows
			if len(lines)-startRow < displayRows {
				displayRows = len(lines) - startRow
			}
			if displayRows > 0 {
				sb.WriteString("  数据:\n")
				for i := 0; i < displayRows; i++ {
					fields := parseLine(lines[startRow+i])
					rowStr := strings.Join(fields, " | ")
					if len([]rune(rowStr)) > 200 {
						rowStr = string([]rune(rowStr)[:200]) + "..."
					}
					sb.WriteString(fmt.Sprintf("  [%d] %s\n", i+1, rowStr))
				}
				if len(lines)-startRow > displayRows {
					sb.WriteString(fmt.Sprintf("  ... 还有 %d 行\n", len(lines)-startRow-displayRows))
				}
			}

			return sb.String()
		},
	}
}
