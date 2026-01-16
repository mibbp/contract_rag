package transform

import (
	"context"
	"github.com/cloudwego/eino/schema"
)

type TransformerOption struct {
}

type Transformer interface {
	Transform(ctx context.Context, src []*schema.Document, opts ...TransformerOption) ([]*schema.Document, error)
}
