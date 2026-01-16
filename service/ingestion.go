package service

import (
	"context"
	"eino-demo/logic/ingestion/extract"
	"eino-demo/storage/es"
	"eino-demo/types"
	"fmt"
	"mime/multipart"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/document/loader/file"
	"github.com/cloudwego/eino-ext/components/document/parser/pdf"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/semantic"
	"github.com/cloudwego/eino/components/document/parser"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/indexer"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	"eino-demo/storage/postgres"

	"github.com/cloudwego/eino/components/model"
)

// cleanText 清洗文本，去除可能导致 NaN 的特殊字符
func cleanText(text string) string {
	// 去除控制字符（除换行、制表符外）
	re := regexp.MustCompile(`[\x00-\x08\x0B-\x0C\x0E-\x1F\x7F]`)
	text = re.ReplaceAllString(text, "")

	// 去除连续的特殊字符
	re = regexp.MustCompile(`[甲]{3,}`)
	text = re.ReplaceAllString(text, "")

	// 去除过多的空白字符
	re = regexp.MustCompile(`\s+`)
	text = re.ReplaceAllString(text, " ")

	// 去除首尾空白
	text = strings.TrimSpace(text)

	return text
}

type ContractService struct {
	pgRepo    *postgres.ContractRepo
	chatModel model.ToolCallingChatModel
	embedder  embedding.Embedder
	indexer   indexer.Indexer
	esIndexer *es.ESIndexer
}

// 构造函数：依赖注入
func NewContractService(pgRepo *postgres.ContractRepo, chatModel model.ToolCallingChatModel, embedder embedding.Embedder, idx indexer.Indexer, esIndexer *es.ESIndexer) *ContractService {
	return &ContractService{
		pgRepo:    pgRepo,
		chatModel: chatModel,
		embedder:  embedder,
		indexer:   idx,
		esIndexer: esIndexer,
	}
}

func (s *ContractService) UploadAndProcess(ctx context.Context, fileHeader *multipart.FileHeader) ([]string, error) {
	startTime := time.Now()
	fmt.Println(">>> [DEBUG] 4. 进入 Service")
	srcFile, err := fileHeader.Open()
	defer srcFile.Close()
	if err != nil {
		return nil, err
	}
	// pdf解析器
	p, err := pdf.NewPDFParser(ctx, &pdf.Config{ToPages: false})
	if err != nil {
		panic(err)
	}
	docs, err := p.Parse(ctx, srcFile, parser.WithURI(fileHeader.Filename))
	if err != nil {

		return nil, fmt.Errorf("parse pdf failed: %v", err)
	}
	fmt.Printf(">>> [性能] PDF 解析耗时: %v\n", time.Since(startTime))
	for _, doc := range docs {
		if doc.MetaData == nil {
			doc.MetaData = make(map[string]any)
		}
		// 手动把文件名塞进去，因为后面查重和存库要用
		doc.MetaData[file.MetaKeyFileName] = fileHeader.Filename
	}

	var docsID []string
	for _, doc := range docs {
		docStartTime := time.Now()
		fileName := doc.MetaData[file.MetaKeyFileName]
		//查重
		one, err := s.pgRepo.GetByFileName(ctx, fileName.(string))
		if one != nil {
			fmt.Printf(">>> [DEBUG] 跳过: 文件已存在数据库中 (%s)\n", fileName)
			continue
		}

		// 结构化提取 存储postgresql
		llmStart := time.Now()
		entity, err := extract.ExtractAndClean(ctx, s.chatModel, doc)
		if err != nil {
			fmt.Println("结构化提取失败", err)
			continue
		}
		fmt.Printf(">>> [性能] LLM 结构化提取耗时: %v\n", time.Since(llmStart))

		// 1. 处理 SignDate (string -> time.Time)
		var signDate *time.Time
		if entity.SignDate != nil && *entity.SignDate != "" {
			t, _ := time.Parse("2006-01-02", *entity.SignDate)
			signDate = &t
		}

		// 2. 处理 EndDate (string -> time.Time)
		var endDate *time.Time
		if entity.EndDate != nil && *entity.EndDate != "" {
			t, _ := time.Parse("2006-01-02", *entity.EndDate)
			endDate = &t
		}

		// 3. 计算 Status (0 或 1) 默认1生效
		status := types.StatusActive
		// 如果有截止日期，且截止日期早于现在，则为已过期 (0)
		if endDate != nil && endDate.Before(time.Now()) {
			status = types.StatusExpired
		}

		var totalAmount float64

		if entity.TotalAmount != nil {
			fmt.Printf(">>>>>>>>>>>>>>>content: %v\n", doc.Content)
			fmt.Printf(">>>>>>>>>>>>>>>>金额：%v\n", entity.TotalAmount)
			switch v := entity.TotalAmount.(type) {
			case float64:
				// 情况1: LLM 返回了数字 (例如 0, 10000)
				totalAmount = v
			case string:
				// 情况2: LLM 返回了字符串 (例如 "1000", "1,000", "100万")
				if v != "" {
					cleanAmount := strings.ReplaceAll(v, ",", "")
					// 简单处理 "万" (根据 Prompt 情况，如果 Prompt 没强制纯数字的话)
					multiplier := 1.0
					if strings.Contains(cleanAmount, "万") {
						cleanAmount = strings.ReplaceAll(cleanAmount, "万", "")
						multiplier = 10000.0
					}

					if val, err := strconv.ParseFloat(cleanAmount, 64); err == nil {
						totalAmount = val * multiplier
					}
				}
			}
		}
		fmt.Printf(">>>>>>>>>>>>>>>>>>>>>>> 清洗金额: %v\n", totalAmount)

		// 生成全局唯一的 DocID
		docID := uuid.New().String()
		now := time.Now()
		err = s.pgRepo.Create(ctx, &postgres.Contract{
			DocID:          docID,
			FileName:       doc.MetaData[file.MetaKeyFileName].(string),
			PartyA:         entity.PartyA,
			PartyB:         entity.PartyB,
			SignDate:       signDate,
			EndDate:        endDate,
			ContractStatus: status,
			ContractType:   entity.ContractType,
			TotalAmount:    totalAmount,
			Summary:        entity.Summary,
			CreatedAt:      now,
			UpdatedAt:      now,
		})
		if err != nil {
			fmt.Println("postgresql存储失败", err)
			continue
		}
		fmt.Println(">>> [DEBUG] 8. 存入数据库成功:", fileName)

		// 切分
		//splitter, _ := recursive.NewSplitter(ctx, &recursive.Config{
		//	ChunkSize:   200,
		//	OverlapSize: 40,
		//	Separators:  []string{"\n\n", "\n", "。", "！", ".", "?", "!"},
		//})
		splitter, err := semantic.NewSplitter(ctx, &semantic.Config{
			Embedding:    s.embedder,
			BufferSize:   5,  // ⬇️ 从 10 降到 5（减少 embedding 计算）
			MinChunkSize: 200,
			Separators:   []string{"\n\n", "\n", "。", "！", "？", "，"},
			LenFunc: func(s string) int {
				// 使用 unicode 字符数而不是字节数
				return len([]rune(s))
			},
			Percentile: 0.85,  // ⬇️ 从 0.9 降到 0.85（更激进合并）
			//IDGenerator:  nil,
		})

		splitStart := time.Now()
		chunks, err := splitter.Transform(ctx, []*schema.Document{doc})
		if err != nil {
			_ = s.pgRepo.Delete(ctx, docID)
			fmt.Printf("切分失败，已回滚PG记录：%v\n", err)
			continue
		}
		fmt.Printf(">>> [性能] 语义切分耗时: %v, 切分出 %d 个 chunk\n", time.Since(splitStart), len(chunks))
		//fmt.Printf(">>>>>>>>>>>>>>>>doc: %v", doc)

		var cleanChunks []*schema.Document
		for _, chunk := range chunks {
			chunk.Content = cleanText(chunk.Content)
			if len(strings.TrimSpace(chunk.Content)) != 0 {
				cleanChunks = append(cleanChunks, chunk)
			}
		}
		if len(cleanChunks) == 0 {
			fmt.Printf(">>>>>>>>>>>>>空chunks原文档: %v\n", doc)
			continue
		}

		chunks = cleanChunks
		for _, chunk := range chunks {
			if len(strings.TrimSpace(chunk.Content)) == 0 {
				fmt.Println(">>>>>>>>>>>>>>>>>>>>>>>>空chunk")
				continue
			}
			chunk.ID = uuid.New().String()

			if chunk.MetaData == nil {
				chunk.MetaData = make(map[string]any)
			}
			chunk.MetaData["doc_id"] = docID
			chunk.MetaData["party_a"] = entity.PartyA
			chunk.MetaData["party_b"] = entity.PartyB
			chunk.MetaData["amount"] = totalAmount
			chunk.MetaData["contract_type"] = entity.ContractType
			chunk.MetaData["contract_status"] = status
			// ✅ 使用解析后的 *time.Time 变量，而不是 *string
			if signDate != nil {
				chunk.MetaData["sign_date"] = *signDate
			} else {
				fmt.Println("sign_date = nil")
			}
			if endDate != nil {
				chunk.MetaData["end_date"] = *endDate
			} else {
				fmt.Println("end_date = nil")
			}
		}

		// es存储
		esStart := time.Now()
		err = s.esIndexer.Store(ctx, docID, chunks, entity.Keywords)
		if err != nil {
			_ = s.pgRepo.Delete(ctx, docID)
			fmt.Printf("es存储失败，已回滚PG记录：%v\n", err)
			return nil, err
		}
		fmt.Printf(">>> [性能] ES 存储耗时: %v\n", time.Since(esStart))

		// 向量化存储
		milvusStart := time.Now()
		_, err = s.indexer.Store(ctx, chunks)
		if err != nil {
			fmt.Printf("❌ Milvus 存储失败! 错误详情: %v\n", err)
			fmt.Printf("存储失败原文档：%v\n", doc)
			fmt.Println("存储失败切片")
			for i, chunk := range chunks {
				fmt.Printf("编号%d content: %v\n", i, chunk.Content)
				fmt.Printf("metadata: %v\n", chunk.MetaData)
			}
			_ = s.pgRepo.Delete(ctx, docID)
			_ = s.esIndexer.DeleteByDocID(ctx, docID)
			fmt.Printf("Milvus 存储失败，已回滚PG记录和ES记录：%v\n", err)
			continue
		}
		fmt.Printf(">>> [性能] Milvus 存储耗时: %v\n", time.Since(milvusStart))
		fmt.Printf(">>> [性能] 单个文档总耗时: %v\n\n", time.Since(docStartTime))
		docsID = append(docsID, docID)
	}

	fmt.Printf("\n>>> [性能总览] 处理完成，共 %d 个文档，总耗时: %v\n", len(docsID), time.Since(startTime))
	return docsID, err
}
