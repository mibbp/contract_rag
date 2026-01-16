package transform

import (
	"context"
	"math"

	"github.com/cloudwego/eino/components/embedding"
)

// CleanEmbedder 包装原始 embedder，处理 NaN/Inf 值
type CleanEmbedder struct {
	inner embedding.Embedder
}

// NewCleanEmbedder 创建带 NaN 清理功能的 embedder
func NewCleanEmbedder(inner embedding.Embedder) *CleanEmbedder {
	return &CleanEmbedder{inner: inner}
}

// EmbedStrings 包装原始方法，清理 NaN/Inf 值
// 返回类型: [][]float64
func (e *CleanEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	// 调用原始方法
	vectors, err := e.inner.EmbedStrings(ctx, texts, opts...)
	if err != nil {
		return nil, err
	}

	// 清理每个向量中的 NaN/Inf 值
	cleanedCount := 0
	for _, vec := range vectors {
		for j, val := range vec {
			// 检查是否为 NaN 或 Inf
			if math.IsNaN(val) || math.IsInf(val, 0) {
				// 将 NaN/Inf 替换为 0.0
				vec[j] = 0.0
				cleanedCount++
			}
		}
	}

	// 如果有清理操作，记录日志
	if cleanedCount > 0 {
		println("⚠️ 检测到 NaN/Inf 值，已清理为 0.0，清理了", cleanedCount, "个维度")
	}

	return vectors, nil
}
