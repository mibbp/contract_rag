package retrieval

import (
	"bytes"
	"context"
	"eino-demo/types"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// analyzeQuery 意图识别实现
func AnalyzeQuery(ctx context.Context, query string, chatModel model.ToolCallingChatModel) (*types.SearchIntent, error) {
	promptTmpl := `
你是一个合同检索助手。当前日期: {{.CurrentDate}}。
请分析用户查询，提取JSON格式的过滤条件。

规则：
1. **intent**:
   - "structured_only": 仅统计或列出合同，无需检索具体条款内容
     * 示例: "张三签了多少份合同?", "列出所有2024年的合同", "金额大于10万的合同有哪些", "乙方是陈七的合同"
     * 特征: 只关心数量、列表、统计信息
   - "hybrid": 需要检索合同的具体条款内容
     * 示例: "2023年张三的服务器采购合同中关于验收的规定", "违约金一般怎么定", "不可抗力条款怎么处理"
     * 特征: 关心具体条款、规定、内容细节
     * 注意: 即使没有结构化过滤条件（如"违约金怎么定"），也是 hybrid 检索，只是 filters 为空

2. **filters** (对象类型，不是数组):
   - **"any_party"**: 提取人名/公司名的**数组**（重要！）
     * 必须拆分"和"、"与"、"及"连接的多个实体为独立元素
     * 示例：
       - "张三和腾讯签署的合同" → ["张三", "腾讯"]
       - "钱九和未来置业公司签署的股权转让协议" → ["钱九", "未来置业公司"]
       - "与阿里巴巴及字节跳动合作" → ["阿里巴巴", "字节跳动"]
     * 单个实体时也是数组：["张三"]

   - "party_a"/"party_b": 仅**明确指定**甲乙方角色时提取（如"张三作为甲方"）
   - "contract_type": 提取如"采购","租赁","保密"
   - "date_range": 格式为 {"start": "YYYY-MM-DD", "end": "YYYY-MM-DD"}，只有一个时间时只填对应字段
   - "amount_range": 格式为 {"min": 数字(元), "max": 数字(元)}，如：
     * "大于30000" → {"min": 30000}
     * "小于100000" → {"max": 100000}
     * "30000到100000之间" → {"min": 30000, "max": 100000}
   - 注意：无过滤条件时返回空对象 {}，不要返回空数组 []

3. **semantic_query**: 去除已提取的元数据，并转化为适配向量化检索的自然语言查询。
   - 去除：人名、公司名、具体日期、金额数字等已结构化的信息
   - 保留：核心业务问题、条款内容、行为描述
   - 优化：将口语化表达转为书面语，确保语义完整且简洁
   - 示例：
     * "张三2023年签的服务器采购合同怎么退款" → "服务器采购合同退款条款"
     * "给我看看关于腾讯的那个合同里的违约责任" → "违约责任条款"
     * "如果发生不可抗力怎么处理" → "不可抗力处理方式"

4. **keywords**: 提取关键词数组，用于 ES BM25 检索。

Output JSON format examples:
{
  "intent": "structured_only",
  "filters": {
    "amount_range": {"min": 30000}
  },
  "semantic_query": "",
  "keywords": ["金额", "大于", "30000", "合同"]
}

{
  "intent": "hybrid",
  "filters": {
    "any_party": ["张三", "腾讯"],
    "date_range": {"start": "2023-01-01", "end": "2023-12-31"}
  },
  "semantic_query": "服务器采购合同验收条款",
  "keywords": ["服务器", "采购", "验收"]
}

{
  "intent": "hybrid",
  "filters": {
    "any_party": ["钱九", "未来置业公司"]
  },
  "semantic_query": "股权转让协议债务条款",
  "keywords": ["股权", "转让", "债务"]
}

{
  "intent": "hybrid",
  "filters": {},
  "semantic_query": "合同违约责任承担方式",
  "keywords": ["违约", "责任", "赔偿"]
}

Output JSON only. No markdown.
`
	// 渲染 Prompt
	t, _ := template.New("p").Parse(promptTmpl)
	var buf bytes.Buffer
	err := t.Execute(&buf, map[string]string{"CurrentDate": time.Now().Format("2006-01-02")})
	if err != nil {
		return nil, err
	}

	// 调用 LLM
	resp, err := chatModel.Generate(ctx, []*schema.Message{
		schema.SystemMessage(buf.String()),
		schema.UserMessage(query),
	})
	if err != nil {
		return nil, err
	}

	// 清洗 JSON
	//raw := strings.TrimSpace(resp.Content)
	//raw = strings.TrimPrefix(raw, "```json")
	//raw = strings.TrimSuffix(raw, "```")
	raw := resp.Content
	fmt.Printf(">>> [LLM Raw Response]: %s\n", raw)

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start != -1 && end != -1 && end > start {
		raw = raw[start : end+1]
	} else {
		// 如果没找到大括号，说明肯定不是合法的 JSON 对象
		fmt.Println(">>> [Error] 无法在响应中找到 JSON 对象")
		// 这里可以决定是返回 error 还是降级
	}

	// 清洗 filters: [] -> filters: {}
	raw = strings.Replace(raw, `"filters": []`, `"filters": {}`, -1)

	var intent types.SearchIntent
	if err := json.Unmarshal([]byte(raw), &intent); err != nil {
		fmt.Println(">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>")
		fmt.Println(intent)
		fmt.Printf(">>> [Error] JSON 解析失败: %v\n", err) // 打印具体错误
		// 兜底：解析失败则降级为 hybrid 检索
		return &types.SearchIntent{Intent: "hybrid", SemanticQuery: query, Keywords: []string{}}, nil
	}
	// 兜底：如果没识别出 intent
	if intent.Intent == "" {
		intent.Intent = "hybrid"
	}

	return &intent, nil
}
