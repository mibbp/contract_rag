package milvus

import (
	"context"
	"eino-demo/types"
	"fmt"
	"log"
	"strings"
	"time"

	"eino-demo/vars"

	"github.com/cloudwego/eino-ext/components/retriever/milvus"
	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// Retrieve 执行向量检索（接收外部创建的 Client）
// query: 语义查询语句 (semantic_query)
// filters: 标量过滤
func Retriever(ctx context.Context, cli client.Client, query string, filters *types.FilterConditions, emb embedding.Embedder) ([]*schema.Document, error) {

	// 2. 自定义 DocumentConverter，包含分数信息
	customConverter := func(ctx context.Context, result client.SearchResult) ([]*schema.Document, error) {
		docs := make([]*schema.Document, result.IDs.Len())
		for i := 0; i < result.IDs.Len(); i++ {
			// 获取 ID
			id, err := result.IDs.GetAsString(i)
			if err != nil {
				return nil, fmt.Errorf("failed to get id: %w", err)
			}

			doc := &schema.Document{
				ID:       id,
				MetaData: make(map[string]any),
			}
			// 获取分数 (关键!)
			// result.Scores 是 []float32，需要转为 float64
			if result.Scores != nil && len(result.Scores) > i {
				doc = doc.WithScore(float64(result.Scores[i]))
			}

			// 解析字段
			for _, field := range result.Fields {
				fieldName := field.Name()
				var value interface{}
				var err error

				// 根据字段名选择正确的获取方法
				switch fieldName {
				case "content":
					value, err = field.GetAsString(i)
					if err == nil {
						doc.Content = value.(string)
					}
				case "party_a", "party_b", "contract_type":
					// VarChar 类型字段
					value, err = field.GetAsString(i)
					if err == nil {
						doc.MetaData[fieldName] = value
					} else {
						log.Printf(">>> [Warning] 字段 %s 获取失败 (索引 %d): %v", fieldName, i, err)
					}
				case "sign_date", "end_date", "contract_status":
					// Int64 类型字段 (Unix 时间戳)
					value, err = field.GetAsInt64(i)
					if err == nil {
						doc.MetaData[fieldName] = value
					} else {
						log.Printf(">>> [Warning] 字段 %s 获取失败 (索引 %d): %v", fieldName, i, err)
					}
				case "amount":
					// Double 类型字段
					value, err = field.GetAsDouble(i)
					if err == nil {
						doc.MetaData[fieldName] = value
					} else {
						log.Printf(">>> [Warning] 字段 %s 获取失败 (索引 %d): %v", fieldName, i, err)
					}
				default:
					// 未知字段，尝试多种类型
					log.Printf(">>> [Info] 遇到未知字段 %s，跳过", fieldName)
					continue
				}
			}
			docs[i] = doc
		}
		return docs, nil
	}

	// 3. 配置 Retriever
	retr, err := milvus.NewRetriever(ctx, &milvus.RetrieverConfig{
		Client:            cli,
		Collection:        vars.COLLECTION,
		VectorField:       "vector",
		OutputFields:      []string{"content"}, // 先只返回 content，测试是否能工作
		DocumentConverter: customConverter,
		MetricType:        entity.L2,
		TopK:              10,
		Embedding:         emb,
	})
	if err != nil {
		return nil, fmt.Errorf("init retriever failed: %v", err)
	}

	// 4. 确保 Collection 已加载到内存（关键优化！）
	loadStart := time.Now()
	err = cli.LoadCollection(ctx, vars.COLLECTION, false)
	if err != nil {
		log.Printf("⚠️ LoadCollection warning: %v", err)
		// 不中断，继续尝试查询
	} else {
		// 等待加载完成（最多 5 秒）
		loadDeadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(loadDeadline) {
			loadState, _ := cli.GetLoadState(ctx, vars.COLLECTION, []string{})
			// 3 = LoadStateLoaded
			if loadState == 3 {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		log.Printf(">>> [Milvus] Collection 加载耗时: %v", time.Since(loadStart))
	}

	// 5. 构建 doc_id 过滤表达式（如果有）

	fmt.Println(">>> [Milvus] 全局语义检索")
	docs, err := retr.Retrieve(ctx, query, milvus.WithFilter(BuildExpr(filters)))

	if err != nil {
		return nil, fmt.Errorf("milvus retrieve failed: %v", err)
	}

	// 5. 打印结果和分数
	fmt.Printf("\n>>> [Milvus Retrieval] 找到 %d 个结果\n", len(docs))
	for i, doc := range docs {
		fmt.Printf("Rank %d | Score: %.4f | ID: %s\n", i+1, doc.Score(), doc.ID)
		fmt.Printf("Content: %s\n", truncateString(doc.Content, 200))
		fmt.Println("----------------------------------------------")
	}

	return docs, nil
}

// 构建过滤表达式
func BuildExpr(filters *types.FilterConditions) string {
	var exprs []string

	// 1. 处理 AnyParty (多个实体，每个都要匹配 party_a 或 party_b)
	if len(filters.AnyParty) > 0 {
		var partyExprs []string
		for _, party := range filters.AnyParty {
			partyExprs = append(partyExprs, fmt.Sprintf("(party_a == '%s' || party_b == '%s')", party, party))
		}
		// 将所有实体条件用 || 连接：(p1匹配) || (p2匹配) || (p3匹配)
		exprs = append(exprs, fmt.Sprintf("(%s)", strings.Join(partyExprs, " || ")))
	}

	// 2. 精确匹配 PartyA
	if filters.PartyA != "" {
		exprs = append(exprs, fmt.Sprintf("party_a == '%s'", filters.PartyA))
	}

	// 3. 精确匹配 PartyB
	if filters.PartyB != "" {
		exprs = append(exprs, fmt.Sprintf("party_b == '%s'", filters.PartyB))
	}

	// 4. 合同类型
	if filters.ContractType != "" {
		exprs = append(exprs, fmt.Sprintf("contract_type == '%s'", filters.ContractType))
	}

	// 5. 状态 (假设 Service 层已转为 int，如果是字符串则加单引号)
	if filters.Status != "" {
		// 示例：如果存储的是字符串
		exprs = append(exprs, fmt.Sprintf("contract_status == '%s'", filters.Status))
	}

	// 6. 日期范围 (需要转换为 Unix 时间戳)
	if filters.DateRange != nil {
		if filters.DateRange.Start != "" {
			// 将 "2023-01-01" 转换为 Unix 时间戳
			if t, err := parseDateToTimestamp(filters.DateRange.Start); err == nil {
				exprs = append(exprs, fmt.Sprintf("sign_date >= %d", t))
			}
		}
		if filters.DateRange.End != "" {
			// 结束日期需要设置为当天 23:59:59
			if t, err := parseDateToTimestamp(filters.DateRange.End); err == nil {
				// 加上 86399 秒 (一天减去1秒) 以覆盖全天
				exprs = append(exprs, fmt.Sprintf("sign_date <= %d", t+86399))
			}
		}
	}

	// 7. 金额范围 (假设字段名为 amount)
	if filters.AmountRange != nil {
		if filters.AmountRange.Min != nil && *filters.AmountRange.Min > 0 {
			exprs = append(exprs, fmt.Sprintf("amount >= %f", *filters.AmountRange.Min))
		}
		if filters.AmountRange.Max != nil && *filters.AmountRange.Max > 0 {
			exprs = append(exprs, fmt.Sprintf("amount <= %f", *filters.AmountRange.Max))
		}
	}

	// 使用 && 连接所有条件
	return strings.Join(exprs, " && ")
}

// parseDateToTimestamp 将 "YYYY-MM-DD" 格式的日期转换为 Unix 时间戳
func parseDateToTimestamp(dateStr string) (int64, error) {
	layout := "2006-01-02"
	t, err := time.Parse(layout, dateStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse date %s: %w", dateStr, err)
	}
	return t.Unix(), nil
}

// truncateString 截断字符串用于显示
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
