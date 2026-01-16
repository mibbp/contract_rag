package loaders

import (
	"context"
	"github.com/cloudwego/eino/schema"
)

type Source struct {
	URI string
}

type LoaderOption struct {
	meta any
}

type Loader interface {
	Load(ctx context.Context, src Source, opts ...LoaderOption) ([]*schema.Document, error)
}
