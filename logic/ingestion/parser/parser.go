package parser

import (
	"context"
	"github.com/cloudwego/eino/schema"
	"io"
)

// todo 自实现pdf解析

type Option struct {
	URI       string
	ExtraMeta map[string]any
}

type Parser interface {
	Parse(ctx context.Context, reader io.Reader, opts ...Option) ([]*schema.Document, error)
}
