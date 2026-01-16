package main

import (
	"context"
	"eino-demo/job"
	"eino-demo/logic/chat"
	"eino-demo/storage/es"
	"eino-demo/storage/milvus"
	"eino-demo/vars"
	"fmt"
	"log"
	"time"

	"github.com/cloudwego/eino-ext/components/embedding/ollama"
	"github.com/milvus-io/milvus-sdk-go/v2/client"

	"github.com/gin-gonic/gin"

	// 引入你所有的包
	"eino-demo/api/handler"
	"eino-demo/api/router"
	"eino-demo/service"
	"eino-demo/storage/postgres"
	// "eino-demo/ingestion/model_loader" // 假设你有个初始化 model 的地方
)

func main() {
	ctx := context.Background()
	// 1. 初始化 DB
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		vars.PGHOST, vars.PGUSER, vars.PGPWD, vars.PGDB, vars.PGPORT)
	db, err := postgres.InitDB(dsn)
	if err != nil {
		panic(err)
	}

	// 2. 初始化 pg
	pgRepo := postgres.NewContractRepo(db)

	// 启动定时任务
	job.StartCronJob(pgRepo)

	// 3. 初始化 LLM Model
	model := chat.CreateOllamaChatModel(ctx, vars.OLLAMA_PATH, vars.QWEN3B)
	embedder, err := ollama.NewEmbedder(ctx, &ollama.EmbeddingConfig{
		BaseURL: vars.OLLAMA_PATH,
		Model:   vars.NOMIC,
		Timeout: 60 * time.Second,
	})
	if err != nil {
		panic(err)
	}

	// 创建全局 Milvus Client（复用）
	milvusClient, err := client.NewClient(ctx, client.Config{
		Address: vars.MILVUSADDR,
	})
	if err != nil {
		panic(fmt.Sprintf("Milvus 连接失败:%v", err))
	}
	log.Println("✅ Milvus 全局连接已创建")

	indexer, err := milvus.NewMilvusIndexerWithClient(ctx, milvusClient, embedder, vars.COLLECTION)
	if err != nil {
		panic(fmt.Sprintf("Milvus 初始化失败:%v", err))
	}

	esIndexer, err := es.NewESIndexer([]string{vars.ESADDR}, "contract_chunks_v1")
	if err != nil {
		panic(err)
	}

	// 4. 初始化 Service (业务层)
	contractSvc := service.NewContractService(pgRepo, model, embedder, indexer, esIndexer)
	retrievalSvc := service.NewRetrievalService(pgRepo, model, embedder, milvusClient, esIndexer.GetClient())
	// 5. 初始化 Handler (API 层)
	contractHandler := handler.NewContractHandler(contractSvc, retrievalSvc)

	// 6. 启动 Web Server
	r := gin.Default()
	router.RegisterRoutes(r, contractHandler)

	log.Println("Server running on :8081")
	r.Run(":8081")
}
