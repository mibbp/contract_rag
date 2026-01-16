package es

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// Filter ES 检索的过滤条件
type Filter struct {
	AnyParty       []string   // 参与方过滤（支持多个实体，匹配 party_a 或 party_b）
	PartyA         string     // 甲方过滤
	PartyB         string     // 乙方过滤
	ContractType   string     // 合同类型过滤
	ContractStatus *int       // 合同状态过滤（0=过期, 1=生效）
	SignDateStart  *time.Time // 签约日期起始
	SignDateEnd    *time.Time // 签约日期截止
	EndDateStart   *time.Time // 截止日期起始
	EndDateEnd     *time.Time // 截止日期截止
	AmountMin      *float64   // 金额最小值
	AmountMax      *float64   // 金额最大值
	DocIDs         []string   // 文档 ID 列表（用于混合检索时限定范围）
}

// Retrieve 执行 ES 检索
// query: 关键词查询语句（用于 BM25）
// filters: 可选的过滤条件（nil 表示无过滤）
// topK: 返回结果数量
func Retriever(ctx context.Context, client *elasticsearch.Client, index string, query string, filters *Filter, topK int) ([]*schema.Document, error) {

	// 1. 构建查询语句
	esQuery := buildESQuery(query, filters, topK)

	// 2. 序列化查询
	var buf strings.Builder
	if err := json.NewEncoder(&buf).Encode(esQuery); err != nil {
		return nil, fmt.Errorf("error encoding query: %s", err)
	}

	log.Printf(">>> [ES] Query: %s", buf.String())

	// 3. 执行搜索
	req := esapi.SearchRequest{
		Index: []string{index},
		Body:  strings.NewReader(buf.String()),
	}

	res, err := req.Do(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("error getting response: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("error response: %s", res.String())
	}

	// 4. 解析结果
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error parsing response body: %s", err)
	}

	// 5. 提取 hits
	hits, ok := result["hits"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	hitsList, ok := hits["hits"].([]interface{})
	if !ok {
		return []*schema.Document{}, nil // 无结果
	}

	// 6. 转换为 []*schema.Document
	docs := make([]*schema.Document, 0, len(hitsList))
	for _, hit := range hitsList {
		hitMap, ok := hit.(map[string]interface{})
		if !ok {
			continue
		}

		// 提取 _id 和 _source
		id, _ := hitMap["_id"].(string)
		source, ok := hitMap["_source"].(map[string]interface{})
		if !ok {
			continue
		}

		// 提取分数
		var score float64
		if scoreVal, ok := hitMap["_score"].(float64); ok {
			score = scoreVal
		}

		// 构造 Document
		doc := &schema.Document{
			ID:       id,
			Content:  toString(source["content"]),
			MetaData: make(map[string]any),
		}
		doc = doc.WithScore(score)

		// 提取元数据
		if val, ok := source["doc_id"]; ok {
			doc.MetaData["doc_id"] = val
		}
		if val, ok := source["chunk_id"]; ok {
			doc.MetaData["chunk_id"] = val
		}
		if val, ok := source["party_a"]; ok {
			doc.MetaData["party_a"] = val
		}
		if val, ok := source["party_b"]; ok {
			doc.MetaData["party_b"] = val
		}
		if val, ok := source["sign_date"]; ok {
			doc.MetaData["sign_date"] = val
		}
		if val, ok := source["end_date"]; ok {
			doc.MetaData["end_date"] = val
		}
		if val, ok := source["amount"]; ok {
			doc.MetaData["amount"] = val
		}
		if val, ok := source["contract_type"]; ok {
			doc.MetaData["contract_type"] = val
		}
		if val, ok := source["contract_status"]; ok {
			doc.MetaData["contract_status"] = val
		}

		docs = append(docs, doc)
	}

	log.Printf(">>> [ES] Retrieved %d results", len(docs))
	for i, doc := range docs {
		log.Printf("Rank %d | Score: %.4f | ID: %s | Content: %v | metadata: %v\n", i+1, doc.Score(), doc.ID, doc.Content, doc.MetaData)
	}

	return docs, nil
}

// SearchByParties 只在 party_a/party_b 字段进行模糊匹配（用于结构化检索）
// 返回去重后的 doc_id 列表
func SearchByParties(ctx context.Context, client *elasticsearch.Client, index string, parties []string) ([]string, error) {
	if len(parties) == 0 {
		return []string{}, nil
	}

	// 1. 构建 should 条件（匹配 party_a 或 party_b）
	var shouldConditions []map[string]interface{}
	for _, party := range parties {
		shouldConditions = append(shouldConditions, map[string]interface{}{
			"match": map[string]interface{}{
				"party_a": party,
			},
		})
		shouldConditions = append(shouldConditions, map[string]interface{}{
			"match": map[string]interface{}{
				"party_b": party,
			},
		})
	}

	esQuery := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"should":               shouldConditions,
				"minimum_should_match": 1, // 至少匹配一个条件
			},
		},
		"size": 100, // 返回足够多的结果
		"_source": []string{"doc_id"}, // 只返回 doc_id，减少传输
	}

	// 2. 序列化查询
	var buf strings.Builder
	if err := json.NewEncoder(&buf).Encode(esQuery); err != nil {
		return nil, fmt.Errorf("error encoding query: %s", err)
	}

	log.Printf(">>> [ES Party Search] Query: %s", buf.String())

	// 3. 执行搜索
	req := esapi.SearchRequest{
		Index: []string{index},
		Body:  strings.NewReader(buf.String()),
	}

	res, err := req.Do(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("error getting response: %s", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("error response: %s", res.String())
	}

	// 4. 解析结果
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error parsing response body: %s", err)
	}

	// 5. 提取 hits 并去重 doc_ids
	hits, ok := result["hits"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid response format")
	}

	hitsList, ok := hits["hits"].([]interface{})
	if !ok {
		return []string{}, nil // 无结果
	}

	docIDSet := make(map[string]struct{})
	for _, hit := range hitsList {
		hitMap, ok := hit.(map[string]interface{})
		if !ok {
			continue
		}

		source, ok := hitMap["_source"].(map[string]interface{})
		if !ok {
			continue
		}

		if docID, ok := source["doc_id"].(string); ok {
			docIDSet[docID] = struct{}{}
		}
	}

	// 转换为数组
	docIDs := make([]string, 0, len(docIDSet))
	for id := range docIDSet {
		docIDs = append(docIDs, id)
	}

	log.Printf(">>> [ES Party Search] 找到 %d 个唯一 doc_id", len(docIDs))
	return docIDs, nil
}

// buildESQuery 构建 ES 查询语句（BM25 + 过滤）
func buildESQuery(query string, filters *Filter, topK int) map[string]interface{} {
	// 1. 构建必须的查询条件（bool.must）
	mustQueries := []map[string]interface{}{
		{
			"match": map[string]interface{}{
				"content": map[string]interface{}{
					"query": query,
				},
			},
		},
	}

	// 2. 构建过滤条件（bool.filter）
	filterQueries := buildFilterQueries(filters)

	// 3. 组合查询
	esQuery := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must":   mustQueries,
				"filter": filterQueries,
			},
		},
		"size": topK,
	}

	return esQuery
}

// buildFilterQueries 构建过滤条件列表
func buildFilterQueries(filters *Filter) []map[string]interface{} {
	if filters == nil {
		return nil
	}

	var filterQueries []map[string]interface{}

	// 处理 AnyParty（多个实体，使用 bool should 查询）
	if len(filters.AnyParty) > 0 {
		// 构建 should 条件列表
		var shouldConditions []map[string]interface{}
		for _, party := range filters.AnyParty {
			shouldConditions = append(shouldConditions, map[string]interface{}{
				"term": map[string]interface{}{
					"party_a": party,
				},
			})
			shouldConditions = append(shouldConditions, map[string]interface{}{
				"term": map[string]interface{}{
					"party_b": party,
				},
			})
		}
		// 用 bool should 包装（至少匹配一个）
		filterQueries = append(filterQueries, map[string]interface{}{
			"bool": map[string]interface{}{
				"should":               shouldConditions,
				"minimum_should_match": 1, // 至少匹配一个条件
			},
		})
	}

	// 精确匹配过滤
	if filters.PartyA != "" {
		filterQueries = append(filterQueries, map[string]interface{}{
			"term": map[string]interface{}{
				"party_a": filters.PartyA,
			},
		})
	}
	if filters.PartyB != "" {
		filterQueries = append(filterQueries, map[string]interface{}{
			"term": map[string]interface{}{
				"party_b": filters.PartyB,
			},
		})
	}
	if filters.ContractType != "" {
		filterQueries = append(filterQueries, map[string]interface{}{
			"term": map[string]interface{}{
				"contract_type": filters.ContractType,
			},
		})
	}
	if filters.ContractStatus != nil {
		filterQueries = append(filterQueries, map[string]interface{}{
			"term": map[string]interface{}{
				"contract_status": *filters.ContractStatus,
			},
		})
	}

	// 范围查询
	rangeFilters := make(map[string]interface{})

	if filters.SignDateStart != nil || filters.SignDateEnd != nil {
		signDateRange := make(map[string]interface{})
		if filters.SignDateStart != nil {
			signDateRange["gte"] = filters.SignDateStart.Format(time.RFC3339)
		}
		if filters.SignDateEnd != nil {
			signDateRange["lte"] = filters.SignDateEnd.Format(time.RFC3339)
		}
		rangeFilters["sign_date"] = signDateRange
	}

	if filters.EndDateStart != nil || filters.EndDateEnd != nil {
		endDateRange := make(map[string]interface{})
		if filters.EndDateStart != nil {
			endDateRange["gte"] = filters.EndDateStart.Format(time.RFC3339)
		}
		if filters.EndDateEnd != nil {
			endDateRange["lte"] = filters.EndDateEnd.Format(time.RFC3339)
		}
		rangeFilters["end_date"] = endDateRange
	}

	if filters.AmountMin != nil || filters.AmountMax != nil {
		amountRange := make(map[string]interface{})
		if filters.AmountMin != nil {
			amountRange["gte"] = *filters.AmountMin
		}
		if filters.AmountMax != nil {
			amountRange["lte"] = *filters.AmountMax
		}
		rangeFilters["amount"] = amountRange
	}

	if len(rangeFilters) > 0 {
		filterQueries = append(filterQueries, map[string]interface{}{
			"range": rangeFilters,
		})
	}

	// doc_id 列表过滤（用于混合检索）
	if len(filters.DocIDs) > 0 {
		filterQueries = append(filterQueries, map[string]interface{}{
			"terms": map[string]interface{}{
				"doc_id": filters.DocIDs,
			},
		})
	}

	return filterQueries
}

// toString 安全地将任意类型转为 string
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if str, ok := v.(string); ok {
		return str
	}
	return fmt.Sprintf("%v", v)
}
