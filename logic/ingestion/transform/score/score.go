package score

import (
	"context"
	"fmt"
	"sort"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

type Config struct {
	// ScoreFieldKey specifies the key in metadata that stores the document score. Use Score() method to get score by default.
	ScoreFieldKey *string
}

// NewReranker creates a score-based document reranker optimized for LLM context processing.
//
// The reranker reorganizes documents based on their scores in a specific pattern:
// - Documents with higher scores are placed at both the beginning and end of the array
// - Documents with lower scores are placed in the middle
//
// This arrangement is based on research showing that LLMs exhibit better performance
// when relevant information appears at the beginning or end of the input context,
// known as the "primacy and recency effect" (https://arxiv.org/abs/2307.03172).
//
// The score can be obtained either from:
// - Document's Score() method (default)
// - A custom metadata field specified by ScoreFieldKey in the config
func NewReranker(ctx context.Context, config *Config) (document.Transformer, error) {
	var getter func(doc *schema.Document) float64
	if config.ScoreFieldKey == nil {
		getter = func(doc *schema.Document) float64 {
			return doc.Score()
		}
	} else {
		key := *config.ScoreFieldKey
		getter = func(doc *schema.Document) float64 {
			if doc.MetaData == nil {
				return 0
			}
			v, ok := doc.MetaData[key]
			if !ok {
				return 0
			}
			vv, okk := v.(float64)
			if !okk {
				return 0
			}
			return vv
		}
	}
	return &reranker{scoreGetter: getter}, nil
}

type reranker struct {
	scoreGetter func(doc *schema.Document) float64
}

func (r *reranker) Transform(ctx context.Context, src []*schema.Document, opts ...document.TransformerOption) ([]*schema.Document, error) {
	copied := make([]*schema.Document, len(src))
	copy(copied, src)
	sortDocs := sortedDocuments{
		docs:        copied,
		scoreGetter: r.scoreGetter,
	}
	sort.Sort(sortDocs)

	ret := make([]*schema.Document, len(src))
	for i, d := range copied {
		if i%2 == 0 {
			ret[i/2] = d
		} else {
			ret[len(ret)-1-i/2] = d
		}
	}
	return ret, nil
}

func (r *reranker) GetType() string {
	return "ScoreReranker"
}

type sortedDocuments struct {
	docs        []*schema.Document
	scoreGetter func(doc *schema.Document) float64
}

func (s sortedDocuments) Len() int {
	return len(s.docs)
}
func (s sortedDocuments) Less(i, j int) bool {
	return s.scoreGetter(s.docs[i]) > s.scoreGetter(s.docs[j])
}
func (s sortedDocuments) Swap(i, j int) {
	s.docs[i], s.docs[j] = s.docs[j], s.docs[i]
}

// ==================== 混合检索 Reranker ====================

// HybridRerankerConfig 混合检索重排配置
type HybridRerankerConfig struct {
	MilvusWeight float64 // Milvus 向量检索权重，默认 0.6
	ESWeight     float64 // ES 关键词检索权重，默认 0.4
	TopK         int     // 最终返回结果数量，默认 10
}

// DefaultHybridRerankerConfig 默认混合检索配置
func DefaultHybridRerankerConfig() *HybridRerankerConfig {
	return &HybridRerankerConfig{
		MilvusWeight: 0.6,
		ESWeight:     0.4,
		TopK:         10,
	}
}

// RerankedDocument 重新排序后的文档（带来源标记）
type RerankedDocument struct {
	*schema.Document
	FinalScore float64  // 最终融合分数
	Sources    []string // 来源标记：["milvus", "es"] 或 ["milvus"] 或 ["es"]
}

// HybridReranker 合并 Milvus 和 ES 的检索结果
// 实现步骤：
// 1. 分数归一化（Min-Max 归一化到 [0,1]）
// 2. 按 ID 去重（同一文档在两个结果集中都出现时，分数累加）
// 3. 加权融合（finalScore = milvusScore * 0.6 + esScore * 0.4）
// 4. 按 FinalScore 降序排序
// 5. 返回 TopK 结果
func HybridReranker(milvusDocs, esDocs []*schema.Document, config *HybridRerankerConfig) []*RerankedDocument {
	if config == nil {
		config = DefaultHybridRerankerConfig()
	}

	// 1. 归一化分数到 [0, 1] 区间（使用 Min-Max 归一化）
	normalizeScores(milvusDocs)
	normalizeScores(esDocs)

	// 2. 按 ID 分组聚合（去重 + 分数累加）
	docMap := make(map[string]*RerankedDocument)

	// 处理 Milvus 结果
	for _, doc := range milvusDocs {
		if doc == nil {
			continue
		}
		if _, exists := docMap[doc.ID]; !exists {
			// 首次出现，创建新记录
			docMap[doc.ID] = &RerankedDocument{
				Document:   doc,
				FinalScore: doc.Score() * config.MilvusWeight,
				Sources:    []string{"milvus"},
			}
		} else {
			// 已存在（ES 中已有），累加 Milvus 分数
			existing := docMap[doc.ID]
			existing.FinalScore += doc.Score() * config.MilvusWeight
			existing.Sources = append(existing.Sources, "milvus")
		}
	}

	// 处理 ES 结果
	for _, doc := range esDocs {
		if doc == nil {
			continue
		}
		if _, exists := docMap[doc.ID]; !exists {
			// 首次出现，创建新记录
			docMap[doc.ID] = &RerankedDocument{
				Document:   doc,
				FinalScore: doc.Score() * config.ESWeight,
				Sources:    []string{"es"},
			}
		} else {
			// 已存在（Milvus 中已有），累加 ES 分数
			existing := docMap[doc.ID]
			existing.FinalScore += doc.Score() * config.ESWeight
			existing.Sources = append(existing.Sources, "es")
		}
	}

	// 3. 转换为数组
	results := make([]*RerankedDocument, 0, len(docMap))
	for _, doc := range docMap {
		results = append(results, doc)
	}

	// 4. 按 FinalScore 降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})

	// 5. 返回 TopK
	if len(results) > config.TopK {
		results = results[:config.TopK]
	}

	return results
}

// normalizeScores Min-Max 归一化到 [0, 1] 区间
// 公式：normalized = (score - min) / (max - min)
func normalizeScores(docs []*schema.Document) {
	if len(docs) == 0 {
		return
	}

	// 找出最大值和最小值
	var maxScore, minScore float64
	maxScore = docs[0].Score()
	minScore = docs[0].Score()

	for _, doc := range docs {
		score := doc.Score()
		if score > maxScore {
			maxScore = score
		}
		if score < minScore {
			minScore = score
		}
	}

	// 如果所有分数相同，避免除以零
	if maxScore == minScore {
		for _, doc := range docs {
			doc = doc.WithScore(1.0) // 所有文档设为相同分数
		}
		return
	}

	// 归一化
	for _, doc := range docs {
		normalized := (doc.Score() - minScore) / (maxScore - minScore)
		doc = doc.WithScore(normalized)
	}
}

// PrintRerankedResults 打印重排序后的结果（调试用）
func PrintRerankedResults(results []*RerankedDocument) {
	fmt.Println("\n========== Reranker 结果 ==========")
	fmt.Printf("总数: %d\n\n", len(results))

	for i, doc := range results {
		fmt.Printf("Rank %d | FinalScore: %.4f | Sources: %v\n", i+1, doc.FinalScore, doc.Sources)
		fmt.Printf("  ID: %s\n", doc.ID)
		fmt.Printf("  Content: %s\n", doc.Content)
		fmt.Printf("  MetaData: party_a=%v, party_b=%v\n",
			doc.MetaData["party_a"], doc.MetaData["party_b"])
		fmt.Println("--------------------------------------")
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
