package milvus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/cloudwego/eino-ext/components/indexer/milvus"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

func NewMilvusIndexer(ctx context.Context, embedder embedding.Embedder, milvusAddr string, collectionName string) (indexer.Indexer, error) {
	fmt.Printf(">>> [Milvus] 正在连接: %s ...\n", milvusAddr)
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cli, err := client.NewClient(connectCtx, client.Config{
		Address: milvusAddr,
	})
	if err != nil {
		return nil, errors.New(fmt.Sprintf("连接milvus失败%v", err))
	}
	fmt.Println(">>> [Milvus] 连接成功")
	return NewMilvusIndexerWithClient(ctx, cli, embedder, collectionName)
}

// NewMilvusIndexerWithClient 使用外部创建的 Client（复用连接）
func NewMilvusIndexerWithClient(ctx context.Context, cli client.Client, embedder embedding.Embedder, collectionName string) (indexer.Indexer, error) {
	fmt.Println(">>> [Milvus] 使用已有连接")

	vecs, err := embedder.EmbedStrings(ctx, []string{"test"})
	if err != nil {
		return nil, fmt.Errorf("Embedder 坏了: %v", err)
	}
	dim := len(vecs[0])
	fmt.Printf("🛑🛑🛑 [Milvus包内部] 实际使用的维度是: %d 🛑🛑🛑\n", dim)

	//has, err := cli.HasCollection(ctx, collectionName)
	//if err != nil {
	//	return nil, err
	//}
	//if has {
	//	fmt.Printf(">>> [调试] 检测到旧表 %s，正在删除以重置 Schema...\n", collectionName)
	//	_ = cli.ReleaseCollection(ctx, collectionName)
	//	_ = cli.DropCollection(ctx, collectionName)
	//}

	// 定义 Schema
	// 注意：字段名必须与 Eino 默认期望的一致，通常是 "id", "vector", "content", "extra"
	// 如果你使用了自定义的 Document 转换器，字段名可能不同，但在默认情况下如下：

	fields := []*entity.Field{
		{
			Name:       "id", // 主键
			DataType:   entity.FieldTypeVarChar,
			PrimaryKey: true,
			AutoID:     false, // Eino 通常生成 UUID 字符串作为 ID
			TypeParams: map[string]string{"max_length": "64"},
		},
		{
			Name:       "doc_id", // 全局id
			DataType:   entity.FieldTypeVarChar,
			AutoID:     false,
			TypeParams: map[string]string{"max_length": "64"},
		},
		{
			Name:       "vector", // 向量字段
			DataType:   entity.FieldTypeFloatVector,
			TypeParams: map[string]string{"dim": fmt.Sprintf("%d", dim)}, // 强制使用正确的维度
		},
		{
			Name:       "content", // 文本内容
			DataType:   entity.FieldTypeVarChar,
			TypeParams: map[string]string{"max_length": "65535"},
		},
		{
			Name: "party_a", DataType: entity.FieldTypeVarChar,
			TypeParams: map[string]string{"max_length": "255"},
		},
		{
			Name: "party_b", DataType: entity.FieldTypeVarChar,
			TypeParams: map[string]string{"max_length": "255"},
		},
		{
			Name: "sign_date", DataType: entity.FieldTypeInt64, // 👈 推荐存 Unix 时间戳，范围查询最快
		},
		{
			Name: "end_date", DataType: entity.FieldTypeInt64, // 👈 推荐存 Unix 时间戳，范围查询最快
		},
		{
			Name: "contract_type", DataType: entity.FieldTypeVarChar,
			TypeParams: map[string]string{"max_length": "255"},
		},
		{
			Name: "contract_status", DataType: entity.FieldTypeInt64,
		},
		{
			Name: "amount", DataType: entity.FieldTypeDouble,
		},
		{
			Name:     "metadata",           // 元数据
			DataType: entity.FieldTypeJSON, // 或者使用 Map，视 Milvus 版本而定，JSON 通用性好
		},
	}

	converter := func(ctx context.Context, docs []*schema.Document, vectors [][]float64) ([]interface{}, error) {
		rows := make([]interface{}, len(docs))

		for i, doc := range docs {
			// 1. 处理向量: float64 -> float32
			vec32 := make([]float32, len(vectors[i]))
			for j, v := range vectors[i] {
				vec32[j] = float32(v)
			}
			// 2. 处理 Metadata: Map -> JSON Bytes
			var docId, partyA, partyB, contractType string
			var signDate, endDate, contractStatus int64
			var amount float64
			if doc.MetaData != nil {
				if val, ok := doc.MetaData["doc_id"]; ok {
					if vStr, ok := val.(string); ok {
						docId = vStr
					}
				}
				if val, ok := doc.MetaData["party_a"]; ok {
					if vStr, ok := val.(string); ok {
						partyA = vStr
					}
				}
				if val, ok := doc.MetaData["party_b"]; ok {
					if vStr, ok := val.(string); ok {
						partyB = vStr
					}
				}
				if val, ok := doc.MetaData["amount"]; ok {
					if vF64, ok := val.(float64); ok {
						amount = vF64
					}
				}
				if val, ok := doc.MetaData["sign_date"]; ok {
					if t, ok := val.(time.Time); ok {
						signDate = t.Unix() // 转为秒级时间戳
					} else if tInt, ok := val.(int64); ok {
						signDate = tInt
					}
				}
				if val, ok := doc.MetaData["end_date"]; ok {
					if t, ok := val.(time.Time); ok {
						endDate = t.Unix() // 转为秒级时间戳
					} else if tInt, ok := val.(int64); ok {
						endDate = tInt
					}
				}
				if val, ok := doc.MetaData["contract_type"]; ok {
					if vStr, ok := val.(string); ok {
						contractType = vStr
					}
				}
				if val, ok := doc.MetaData["contract_status"]; ok {
					// 兼容 int 和 int64 类型
					if vInt64, ok := val.(int64); ok {
						fmt.Printf(">>>>>>>>>>>>>>contract_status (int64): %v\n", vInt64)
						contractStatus = vInt64
					} else if vInt, ok := val.(int); ok {
						fmt.Printf(">>>>>>>>>>>>>>contract_status (int): %v\n", vInt)
						contractStatus = int64(vInt)
					}
				}
			}
			if doc.MetaData == nil {
				doc.MetaData = make(map[string]interface{})
			}
			metaBytes, err := json.Marshal(doc.MetaData)
			if err != nil {
				metaBytes = []byte("{}")
			}

			// 3. 构造行对象 (Map)
			row := map[string]interface{}{
				"id":              doc.ID,
				"doc_id":          docId,
				"vector":          vec32,
				"content":         doc.Content,
				"party_a":         partyA,
				"party_b":         partyB,
				"amount":          amount,
				"sign_date":       signDate,
				"end_date":        endDate,
				"contract_type":   contractType,
				"contract_status": contractStatus,
				"metadata":        metaBytes,
			}
			rows[i] = row
		}
		return rows, nil
	}
	idx, err := milvus.NewIndexer(ctx, &milvus.IndexerConfig{
		Client:            cli,
		Collection:        collectionName,
		Embedding:         embedder,
		Fields:            fields,
		DocumentConverter: converter,
		MetricType:        milvus.L2,
	})
	if err != nil {
		return nil, fmt.Errorf("[NewIndexer] 建表失败: %v", err)
	}

	// 先 Release 才能操作索引
	_ = cli.ReleaseCollection(ctx, collectionName)

	// 删除默认索引 (注意字段名 "vector" 必须与你 fields 定义的一致)
	err = cli.DropIndex(ctx, collectionName, "vector")
	if err != nil {
		fmt.Printf(">>> [调试] DropIndex 提示: %v\n", err)
	}

	// 创建你想要的 HNSW 索引 (针对 BGE-M3，建议用 IP)
	// 如果你前面 fields 里的 MetricType 没法改，这里就保持 L2
	hnswIdx, _ := entity.NewIndexHNSW(entity.L2, 16, 200)
	err = cli.CreateIndex(ctx, collectionName, "vector", hnswIdx, false)
	if err != nil {
		return nil, fmt.Errorf("❌ 创建 HNSW 向量索引失败: %v", err)
	}

	fmt.Println(">>> [Milvus] 正在为标量字段创建索引...")

	err = cli.ReleaseCollection(ctx, collectionName)
	if err != nil {
		// 这里的 error 可以忽略，因为如果表本来就没 Load，Release 会报错但没关系
		fmt.Printf(">>> [调试] Release 提示 (可忽略): %v\n", err)
	}

	err = cli.CreateIndex(ctx, collectionName, "party_a", entity.NewScalarIndex(), false)
	if err != nil {
		return nil, fmt.Errorf("❌ 创建 party_a 索引失败: %v", err)
	}
	err = cli.CreateIndex(ctx, collectionName, "party_b", entity.NewScalarIndex(), false)
	if err != nil {
		return nil, fmt.Errorf("❌ 创建 party_b 索引失败: %v", err)
	}
	err = cli.CreateIndex(ctx, collectionName, "sign_date", entity.NewScalarIndex(), false)
	if err != nil {
		return nil, fmt.Errorf("❌ 创建 sign_date 索引失败: %v", err)
	}
	err = cli.CreateIndex(ctx, collectionName, "end_date", entity.NewScalarIndex(), false)
	if err != nil {
		return nil, fmt.Errorf("❌ 创建 end_date 索引失败: %v", err)
	}
	err = cli.CreateIndex(ctx, collectionName, "amount", entity.NewScalarIndex(), false)
	if err != nil {
		return nil, fmt.Errorf("❌ 创建 amount 索引失败: %v", err)
	}
	err = cli.CreateIndex(ctx, collectionName, "contract_type", entity.NewScalarIndex(), false)
	if err != nil {
		return nil, fmt.Errorf("❌ 创建 contract_type 索引失败: %v", err)
	}
	err = cli.CreateIndex(ctx, collectionName, "contract_status", entity.NewScalarIndex(), false)
	if err != nil {
		return nil, fmt.Errorf("❌ 创建 contract_status 索引失败: %v", err)
	}

	fmt.Println(">>> [Milvus] 正在 Load Collection...")
	err = cli.LoadCollection(ctx, collectionName, false)
	if err != nil {
		return nil, fmt.Errorf("Load Collection 失败: %v", err)
	}

	fmt.Println(">>> [调试] 正在查询 Milvus 确认索引是否存在...")

	// 查 party_a 的索引
	idxA, err := cli.DescribeIndex(ctx, collectionName, "party_a")
	if err != nil {
		fmt.Printf("⚠️ 查不到 party_a 索引: %v\n", err)
	} else {
		fmt.Printf("✅ party_a 索引存在! 详情: %+v\n", idxA)
	}

	// 查 sign_date 的索引
	idxDate, err := cli.DescribeIndex(ctx, collectionName, "sign_date")
	if err != nil {
		fmt.Printf("⚠️ 查不到 sign_date 索引: %v\n", err)
	} else {
		fmt.Printf("✅ sign_date 索引存在! 详情: %+v\n", idxDate)
	}
	// ========================================================

	fmt.Println("创建成功")
	return idx, nil
}
