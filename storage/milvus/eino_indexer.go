package milvus

import (
	"context"
	"eino-demo/vars"
	"errors"
	"fmt"

	"github.com/cloudwego/eino-ext/components/indexer/milvus"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

func NewMilvusEinoIndexer(ctx context.Context, embedder embedding.Embedder, milvusAddr string, collectionName string) (indexer.Indexer, error) {
	fmt.Println(">>>>>>>>>>>>>>>>>>>>>>>进入NewMilvusEinoIndexer")
	cli, err := client.NewClient(ctx, client.Config{
		Address: milvusAddr,
	})
	if err != nil {
		return nil, errors.New("创建client err:" + err.Error())
	}
	defer cli.Close()

	vecs, err := embedder.EmbedStrings(ctx, []string{"test"})
	if err != nil {
		return nil, fmt.Errorf("Embedder 坏了: %v", err)
	}
	dim := len(vecs[0])

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
			PrimaryKey: true,
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
			Name:     "metadata",           // 元数据
			DataType: entity.FieldTypeJSON, // 或者使用 Map，视 Milvus 版本而定，JSON 通用性好
		},
	}
	indexer, err := milvus.NewIndexer(ctx, &milvus.IndexerConfig{
		Client:     cli,
		Collection: vars.COLLECTION,
		Fields:     fields,
		MetricType: milvus.L2,
		Embedding:  embedder,
	})
	if err != nil {
		return nil, errors.New("创建indexer err:" + err.Error())
	}
	fmt.Println("创建indexer成功")

	return indexer, nil
}
