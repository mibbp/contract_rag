package es

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/elastic/go-elasticsearch/v8"

	"github.com/cloudwego/eino/schema"
	"github.com/elastic/go-elasticsearch/v8/esutil"
)

type ESIndexer struct {
	client *elasticsearch.Client
	index  string
}

// GetClient 返回 ES 客户端（用于检索）
func (e *ESIndexer) GetClient() *elasticsearch.Client {
	return e.client
}

// NewESIndexer 初始化 ES 客户端并确保索引存在
func NewESIndexer(addresses []string, indexName string) (*ESIndexer, error) {
	cfg := elasticsearch.Config{
		Addresses: addresses,
	}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("error creating the client: %s", err)
	}

	indexer := &ESIndexer{client: es, index: indexName}

	// 初始化索引 Mapping (定义字段类型)
	if err := indexer.initMapping(context.Background()); err != nil {
		return nil, err
	}

	return indexer, nil
}

func (e *ESIndexer) initMapping(ctx context.Context) error {
	// 1. 检查索引是否存在
	res, err := e.client.Indices.Exists([]string{e.index})
	if err != nil {
		return err
	}
	if res.StatusCode == 200 {
		return nil // 已存在，跳过
	}

	// 2. 定义 Mapping (核心：配置 ik_max_word)
	mapping := `
	{
	  "settings": {
		"number_of_shards": 1,
		"number_of_replicas": 0
	  },
	  "mappings": {
		"properties": {
		  "doc_id":    { "type": "keyword" },
		  "chunk_id":  { "type": "keyword" },
		  "content": {
			"type": "text",
			"analyzer": "ik_max_word",
			"search_analyzer": "ik_smart"
		  },
		  "keywords":  { "type": "keyword" },
		  "party_a": {
			"type": "text",
			"analyzer": "ik_max_word",
			"fields": {
			  "keyword": { "type": "keyword" }
			}
		  },
		  "party_b": {
			"type": "text",
			"analyzer": "ik_max_word",
			"fields": {
			  "keyword": { "type": "keyword" }
			}
		  },
		  "sign_date":       { "type": "date" },
		  "end_date":        { "type": "date" },
		  "amount":          { "type": "double" },
		  "contract_type":   { "type": "keyword" },
		  "contract_status": { "type": "short" }
		}
	  }
	}`

	log.Printf(">>> [ES] Creating index %s with IK analyzer...", e.index)
	res, err = e.client.Indices.Create(
		e.index,
		e.client.Indices.Create.WithBody(strings.NewReader(mapping)),
	)
	if err != nil {
		return fmt.Errorf("create index error: %v", err)
	}
	if res.IsError() {
		return fmt.Errorf("create index response error: %s", res.String())
	}
	return nil
}

// Store 批量存储
func (e *ESIndexer) Store(ctx context.Context, docID string, chunks []*schema.Document, keywords []string) error {
	bi, err := esutil.NewBulkIndexer(esutil.BulkIndexerConfig{
		Index:         e.index,
		Client:        e.client,
		FlushInterval: 1, // 开发环境立即刷新
	})
	if err != nil {
		return err
	}

	for _, chunk := range chunks {
		// 构造数据
		docModel := map[string]interface{}{
			"doc_id":          docID,
			"chunk_id":        chunk.ID,
			"content":         chunk.Content,
			"keywords":        keywords, // LLM 提取出的关键词列表
			"party_a":         chunk.MetaData["party_a"],
			"party_b":         chunk.MetaData["party_b"],
			"sign_date":       chunk.MetaData["sign_date"],
			"end_date":        chunk.MetaData["end_date"],
			"amount":          chunk.MetaData["amount"],
			"contract_type":   chunk.MetaData["contract_type"],
			"contract_status": chunk.MetaData["contract_status"],
		}

		// 提取结构化字段（如果存在）
		if val, ok := chunk.MetaData["party_a"]; ok {
			docModel["party_a"] = val
		}
		if val, ok := chunk.MetaData["party_b"]; ok {
			docModel["party_b"] = val
		}
		if val, ok := chunk.MetaData["sign_date"]; ok {
			docModel["sign_date"] = val
		}
		if val, ok := chunk.MetaData["end_date"]; ok {
			docModel["end_date"] = val
		}
		if val, ok := chunk.MetaData["amount"]; ok {
			docModel["amount"] = val
		}
		if val, ok := chunk.MetaData["contract_type"]; ok {
			docModel["contract_type"] = val
		}
		if val, ok := chunk.MetaData["contract_status"]; ok {
			docModel["contract_status"] = val
		}

		data, _ := json.Marshal(docModel)

		// 加入批量队列
		err = bi.Add(ctx, esutil.BulkIndexerItem{
			Action:     "index",
			DocumentID: chunk.ID, // 使用 ChunkID 作为 ES 的 _id，避免重复
			Body:       strings.NewReader(string(data)),
		})
		if err != nil {
			return err
		}
	}

	if err := bi.Close(ctx); err != nil {
		return err
	}
	return nil
}

func (e *ESIndexer) DeleteByDocID(ctx context.Context, docID string) error {
	// 构造查询语句：{"query": {"term": {"doc_id": "xxx"}}}
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"term": map[string]interface{}{
				"doc_id": docID, // 注意：doc_id 字段必须是 keyword 类型
			},
		},
	}

	// JSON 序列化
	var buf strings.Builder
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return fmt.Errorf("error encoding query: %s", err)
	}

	// 调用 DeleteByQuery API
	res, err := e.client.DeleteByQuery(
		[]string{e.index}, // 索引名
		strings.NewReader(buf.String()),
		e.client.DeleteByQuery.WithContext(ctx),
		e.client.DeleteByQuery.WithRefresh(true), // 强制刷新，确保立即生效（开发环境用，生产环境可去掉）
	)

	if err != nil {
		return fmt.Errorf("ES delete request failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("ES delete response error: %s", res.String())
	}

	log.Printf(">>> [ES] 已回滚/删除 DocID=%s 的相关数据", docID)
	return nil
}
