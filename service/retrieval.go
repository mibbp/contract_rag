package service

import (
	"context"
	"eino-demo/logic/ingestion/transform/score"
	"eino-demo/logic/retrieval"
	"eino-demo/storage/es"
	"eino-demo/storage/milvus"
	"eino-demo/storage/postgres"
	"eino-demo/types"
	"eino-demo/vars"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
)

type RetrievalService struct {
	pgRepo       *postgres.ContractRepo
	chatModel    model.ToolCallingChatModel
	embedder     embedding.Embedder
	milvusClient client.Client
	esClient     *elasticsearch.Client
}

func NewRetrievalService(pgRepo *postgres.ContractRepo, chatModel model.ToolCallingChatModel, embedder embedding.Embedder, milvusClient client.Client, esClient *elasticsearch.Client) *RetrievalService {
	return &RetrievalService{
		pgRepo:       pgRepo,
		chatModel:    chatModel,
		embedder:     embedder,
		milvusClient: milvusClient,
		esClient:     esClient,
	}
}

// Search 意图识别 + 检索实现
func (s *RetrievalService) Search(ctx context.Context, query string) (string, error) {
	searchStart := time.Now()

	analyzeQuery, err := retrieval.AnalyzeQuery(ctx, query, s.chatModel)
	if err != nil {
		return "无法分析用户输入", err
	}
	fmt.Printf(">>> [Intent] %+v\n", analyzeQuery)
	fmt.Printf(">>> [性能] 意图识别耗时: %v\n", time.Since(searchStart))

	// 根据意图分发
	if analyzeQuery.Intent == vars.PG {
		// structured_only: 结构化检索
		var esDocIDs []string
		var err error

		// 1. 如果有公司名过滤，先用 ES 模糊匹配
		if len(analyzeQuery.Filters.AnyParty) > 0 {
			esStart := time.Now()
			esDocIDs, err = es.SearchByParties(ctx, s.esClient, "contract_chunks_v1", analyzeQuery.Filters.AnyParty)
			if err != nil {
				return fmt.Sprintf("ES 查询失败: %v", err), err
			}
			fmt.Printf(">>> [ES Party Search] 找到 %d 个唯一文档, 耗时: %v\n", len(esDocIDs), time.Since(esStart))

			// 如果 ES 没找到任何结果，直接返回
			if len(esDocIDs) == 0 {
				return "抱歉，没有找到符合条件的合同", nil
			}
		}

		// 2. 用 PG 应用其他过滤条件（日期、金额、类型等）
		pgStart := time.Now()
		var docIDs []string
		docIDs, err = s.pgRepo.SearchContracts(ctx, &analyzeQuery.Filters, esDocIDs)
		fmt.Printf(">>> [PG Filter] 从 ES 结果中用其他条件筛选，找到 %d 份合同, 耗时: %v\n", len(docIDs), time.Since(pgStart))
		if err != nil {
			return "PG查询失败", err
		}
		if len(docIDs) == 0 {
			return "抱歉，没有找到符合条件的合同", nil
		}

		// 3. 批量查询 PG 获取完整合同信息
		for _, id := range docIDs {
			contract, err := s.pgRepo.GetByDocID(ctx, id)
			if err != nil {
				continue
			}
			fmt.Printf("  - %s (金额: %.2f)\n", contract.FileName, contract.TotalAmount)
		}
		return fmt.Sprintf("根据条件，共找到 %d 份合同。", len(docIDs)), nil

	} else {
		// hybrid: Milvus + ES 混合检索
		fmt.Println(">>> [Hybrid Search] 开始混合检索...")

		// 1. Milvus 向量检索
		milvusStart := time.Now()
		milvusDocs, err := milvus.Retriever(ctx, s.milvusClient, analyzeQuery.SemanticQuery, &analyzeQuery.Filters, s.embedder)
		if err != nil {
			return "", fmt.Errorf("Milvus 检索失败: %v", err)
		}
		fmt.Printf(">>> [Milvus] 找到 %d 个结果, 耗时: %v\n", len(milvusDocs), time.Since(milvusStart))

		// 2. ES 关键词检索
		esStart := time.Now()
		esFilters := s.convertFiltersToES(&analyzeQuery.Filters)
		esQuery := fmt.Sprintf("%s %s", analyzeQuery.SemanticQuery, strings.Join(analyzeQuery.Keywords, " "))
		esDocs, err := es.Retriever(ctx, s.esClient, "contract_chunks_v1", esQuery, esFilters, 10)
		if err != nil {
			return "", fmt.Errorf("ES 检索失败: %v", err)
		}
		fmt.Printf(">>> [ES] 找到 %d 个结果, 耗时: %v\n", len(esDocs), time.Since(esStart))

		// 3. Reranker 合并两个结果集（归一化、去重、加权融合）
		rerankStart := time.Now()
		rerankedDocs := score.HybridReranker(milvusDocs, esDocs, nil)
		fmt.Printf(">>> [性能] Reranker 融合耗时: %v\n", time.Since(rerankStart))

		// 4. 打印最终结果
		score.PrintRerankedResults(rerankedDocs)

		// 5. 返回融合后的结果
		totalTime := time.Since(searchStart)
		fmt.Printf(">>> [性能总览] 检索总耗时: %v (意图识别: %.2f%%, Milvus: %.2f%%, ES: %.2f%%, Reranker: %.2f%%)\n",
			totalTime,
			float64(time.Since(searchStart)-totalTime)/float64(totalTime)*100, // 占位符，实际需要记录各阶段时间
			0, 0, 0) // 简化显示

		return fmt.Sprintf("混合检索完成：融合后 %d 条结果，总耗时 %v", len(rerankedDocs), totalTime), nil
	}
}

// convertFiltersToES 将 types.FilterConditions 转换为 es.Filter
func (s *RetrievalService) convertFiltersToES(filters *types.FilterConditions) *es.Filter {
	if filters == nil {
		return nil
	}

	esFilter := &es.Filter{
		AnyParty:     filters.AnyParty, // 传递数组
		PartyA:       filters.PartyA,
		PartyB:       filters.PartyB,
		ContractType: filters.ContractType,
	}

	// 处理 contract_status
	if filters.Status != "" {
		status := 1 // 默认生效中
		if filters.Status == "已过期" || filters.Status == "过期" {
			status = 0
		}
		esFilter.ContractStatus = &status
	}

	// 处理日期范围
	if filters.DateRange != nil {
		if filters.DateRange.Start != "" {
			if t, err := time.Parse("2006-01-02", filters.DateRange.Start); err == nil {
				esFilter.SignDateStart = &t
			}
		}
		if filters.DateRange.End != "" {
			if t, err := time.Parse("2006-01-02", filters.DateRange.End); err == nil {
				esFilter.SignDateEnd = &t
			}
		}
	}

	// 处理金额范围
	if filters.AmountRange != nil {
		esFilter.AmountMin = filters.AmountRange.Min
		esFilter.AmountMax = filters.AmountRange.Max
	}

	return esFilter
}

// truncate 截断字符串
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
