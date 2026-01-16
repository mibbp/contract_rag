package processors

import (
	"context"
	"github.com/cloudwego/eino/schema"
	"strings"
	"unicode/utf8"
)

func Processor(ctx context.Context, src []*schema.Document) ([]*schema.Document, error) {
	// 1. 清洗数据：去除无法处理的字符和空白文档
	var cleanDocs []*schema.Document
	for _, doc := range src {
		// 移除 Null 字节 (常见 PDF 解析错误)
		content := strings.ReplaceAll(doc.Content, "\x00", "")

		// 移除无效的 UTF-8 字符
		if !utf8.ValidString(content) {
			v := make([]rune, 0, len(content))
			for i, r := range content {
				if r == utf8.RuneError {
					_, size := utf8.DecodeRuneInString(content[i:])
					if size == 1 {
						continue
					}
				}
				v = append(v, r)
			}
			content = string(v)
		}

		// 去除首尾空白
		content = strings.TrimSpace(content)

		// 如果内容为空，直接跳过，否则 Embedding 会报错
		if content == "" {
			println("Warning: Found empty document chunk, skipping...")
			continue
		}

		doc.Content = content
		cleanDocs = append(cleanDocs, doc)
	}
	return cleanDocs, nil
}
