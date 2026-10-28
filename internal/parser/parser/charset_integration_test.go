// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func TestCSVParser_GBK(t *testing.T) {
	csvText := "产品,分类,价格,备注\n笔记本电脑,电子,6999.00,\"特价销售，送鼠标\"\n无线耳机,配件,399.00,\"热销\"\n"
	gbkBytes, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(csvText))
	if err != nil {
		t.Fatalf("GBK encode: %v", err)
	}

	p := NewCSVParser()
	res := p.ParseWithResult(context.Background(), "products.csv", gbkBytes)
	if res.Err != nil {
		t.Fatalf("CSVParser.ParseWithResult failed: %v", res.Err)
	}
	if len(res.JSON) == 0 {
		t.Fatalf("CSVParser returned no JSON items")
	}
	var rowText strings.Builder
	for _, item := range ItemsToMaps(res.JSON) {
		text := tableItemHTML(t, item)
		rowText.WriteString(text)
	}
	if strings.ContainsRune(rowText.String(), '\ufffd') {
		t.Errorf("CSV output contains replacement runes: %s", rowText.String())
	}
	for _, want := range []string{"笔记本电脑", "电子", "特价销售，送鼠标", "无线耳机"} {
		if !strings.Contains(rowText.String(), want) {
			t.Errorf("CSV output missing expected substring %q; got: %s", want, rowText.String())
		}
	}
	if enc, _ := res.File["encoding"].(string); enc != "gb18030" && enc != "gbk" && enc != "gb2312" {
		t.Errorf("CSV File.encoding = %q, want gbk/gb2312/gb18030", enc)
	}
}

func TestEPUBParser_GB2312(t *testing.T) {
	// Create an in-memory EPUB zip with GB2312-encoded chapter XHTML
	chapterContent := `<?xml version="1.0" encoding="GB2312"?>
<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.1//EN" "http://www.w3.org/TR/xhtml11/DTD/xhtml11.dtd">
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>第一章</title></head>
<body><h1>第一章 破晓</h1><p>这是一段包含中文的EPUB正文内容。</p></body>
</html>`
	chapterGBK, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(chapterContent))
	if err != nil {
		t.Fatalf("GBK encode: %v", err)
	}

	containerXML := `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`
	contentOPF := `<?xml version="1.0" encoding="utf-8"?><package xmlns="http://www.idpf.org/2007/opf" unique-identifier="BookID" version="2.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>测试</dc:title></metadata><manifest><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="c1"/></spine></package>`

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("META-INF/container.xml")
	w.Write([]byte(containerXML))
	w, _ = zw.Create("OEBPS/content.opf")
	w.Write([]byte(contentOPF))
	w, _ = zw.Create("OEBPS/c1.xhtml")
	w.Write(chapterGBK)
	zw.Close()

	p := NewEPUBParser()
	res := p.ParseWithResult(context.Background(), "book.epub", buf.Bytes())
	if res.Err != nil {
		t.Fatalf("EPUBParser.ParseWithResult failed: %v", res.Err)
	}
	if len(res.JSON) == 0 {
		t.Fatal("want non-empty JSON items from EPUB")
	}
	text := epubTestText(res)
	if strings.ContainsRune(text, '\ufffd') {
		t.Errorf("EPUB text contains replacement runes: %s", text)
	}
	for _, want := range []string{"第一章 破晓", "这是一段包含中文的EPUB正文内容。"} {
		if !strings.Contains(text, want) {
			t.Errorf("EPUB text missing %q; got: %s", want, text)
		}
	}
	if enc, _ := res.File["encoding"].(string); enc != "gb2312" && enc != "gbk" && enc != "gb18030" {
		t.Errorf("EPUB File.encoding = %q, want gb2312/gbk/gb18030", enc)
	}
}
