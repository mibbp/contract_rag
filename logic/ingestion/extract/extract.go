package extract

import (
	"context"
	"eino-demo/types"
	"eino-demo/vars"
	"encoding/json"
	"fmt"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"strings"
	"time"
)

// 1. 手动定义参数 Map
//params := map[string]*schema.ParameterInfo{
//	"party_a": {
//		Type:     schema.String, // 参数类型
//		Desc:     "合同甲方的全称",   // 描述
//		Required: true,          // 是否必填
//	},
//	"sign_date": {
//		Type:     schema.String,
//		Desc:     "签署日期，格式 YYYY-MM-DD",
//		Required: false,
//	},
//}

func ExtractAndClean(ctx context.Context, model model.ToolCallingChatModel, data *schema.Document) (*types.ContractRawData, error) {
	content := data.Content
	if len(content) > 10000 {
		content = content[:10000]
	}

	prompt := strings.ReplaceAll(vars.EXTARACT, "{{.Content}}", content)
	prompt = strings.ReplaceAll(prompt, "{{.CurrentDate}}", time.Now().Format("2006-01-02"))
	// 2. 调用 LLM
	resp, err := model.Generate(ctx, []*schema.Message{
		schema.UserMessage(prompt),
	})
	if err != nil {
		return nil, err
	}

	jsonStr := resp.Content
	jsonStr = strings.TrimSpace(jsonStr)
	jsonStr = strings.TrimPrefix(jsonStr, "```json")
	jsonStr = strings.TrimPrefix(jsonStr, "```")
	jsonStr = strings.TrimSuffix(jsonStr, "```")

	// 4. 反序列化
	var info types.ContractRawData
	err = json.Unmarshal([]byte(jsonStr), &info)
	if err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %v, raw: %s", err, jsonStr)
	}

	return &info, nil

}

//func extractContractInfo(ctx context.Context, model model.ToolCallingChatModel, data *schema.Document) (*ContractRawData, error) {
//
//	paramsOneOf, err := utils.GoStruct2ParamsOneOf[types.ContractRawData]()
//
//	// 1. 定义工具结构
//	// Eino 会利用反射将 ContractRawData 转换为 JSON Schema 发给 LLM
//	extractTool := schema.ToolInfo{
//		Name:        "extract_contract_metadata",
//		Desc:        "从合同文本中提取关键元数据（甲乙方、签署日期、终止日期、合同类型、金额、简要内容）",
//		ParamsOneOf: paramsOneOf,
//	}
//
//	// 2. 绑定工具
//	//err = model.BindTools([]*schema.ToolInfo{&extractTool})
//	toolModel, err := model.WithTools([]*schema.ToolInfo{&extractTool})
//	if err != nil {
//		return nil, err
//	}
//
//	// 3. 构造 Prompt
//	// 技巧：截取文本。如果 PDF 只有 5 页，发全文；如果 50 页，只发前 2000 字和后 2000 字。
//	messages := []*schema.Message{
//		schema.SystemMessage("你是一个专业的合同数据录入员。请仔细阅读合同，提取关键元数据（甲乙方、日期、金额、简要内容）。"),
//		schema.UserMessage(data.Content),
//	}
//
//	// 4. 调用模型
//	resp, err := toolModel.Generate(ctx, messages)
//	if err != nil {
//		return nil, err
//	}
//
//	// 5. 解析工具调用结果
//	// 检查 LLM 是否真的调用了工具
//
//	if len(resp.ToolCalls) > 0 {
//		toolCall := resp.ToolCalls[0]
//		var result ContractRawData
//		//fmt.Println("--------------------------------------")
//		//fmt.Println(toolCall)
//		// 将 LLM 生成的 JSON 参数反序列化回 Go Struct
//		err := json.Unmarshal([]byte(toolCall.Function.Arguments), &result)
//		if err != nil {
//			return nil, fmt.Errorf("parse json error: %v", err)
//		}
//		//fmt.Println("--------------------------------")
//		//fmt.Println(result)
//		return &result, nil
//	}
//
//	return nil, fmt.Errorf("llm did not return structured data")
//}
//
//func convertToEntity(raw *ContractRawData) *ContractEntity {
//	entity := &ContractEntity{
//		PartyA:   raw.PartyA,
//		PartyB:   raw.PartyB,
//		Summary:  raw.Summary,
//		Keywords: raw.Keywords,
//	}
//
//	// --- 日期清洗 ---
//	// LLM 可能返回 "2023年10月5日" 或 "2023-10-05" 或 "Unknown"
//	if raw.SignDate != "" {
//		// dateparse 非常强大，能识别中文日期、斜杠日期等多种格式
//		parsedTime, err := dateparse.ParseAny(raw.SignDate)
//		if err == nil {
//			entity.SignDate = &parsedTime
//		} else {
//			// 如果解析失败，可以在这里加正则兜底逻辑
//			// 或者直接留空
//			entity.SignDate = nil
//		}
//	}
//
//	// --- 金额清洗 ---
//	// LLM 可能返回 "1,000,000.00" 或 "100万"
//	// 1. 去除逗号
//	amountStr := strings.ReplaceAll(raw.TotalAmount, ",", "")
//	// 2. 提取数字
//	re := regexp.MustCompile(`[0-9]+(\.[0-9]+)?`)
//	matches := re.FindString(amountStr)
//	if matches != "" {
//		val, err := strconv.ParseFloat(matches, 64)
//		if err == nil {
//			// 简单处理"万"字单位（如果在 Prompt 里没强制 LLM 转的话）
//			if strings.Contains(raw.TotalAmount, "万") {
//				val *= 10000
//			}
//			entity.TotalAmount = val
//		}
//	}
//
//	return entity
//}
