package vars

import (
	"os"
)

// GetEnv 获取环境变量，如果不存在则返回默认值
func GetEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

const (
	// 模型名称
	NOMIC      = "nomic-embed-text"
	BGEM3      = "bge-m3"
	DEEPSEEKR1 = "deepseek-r1:7b"
	QWEN7B     = "qwen2.5:7b"
	QWEN3B     = "qwen2.5:3b"
	QWENEMB    = "qwen3-embedding"

	// Milvus Collection 名称
	COLLECTION = "contract_collection_v2"

	// 检索方式
	ML = "semantic_only"
	PG = "structured_only"
	HY = "hybrid"
)

// 环境变量配置（支持 Docker 部署）
var (
	// OLLAMA
	OLLAMA_PATH = GetEnv("OLLAMA_PATH", "http://localhost:11434")

	// PG
	PGUSER = GetEnv("PGUSER", "root")
	PGPWD  = GetEnv("PGPWD", "Llh2002908")
	PGDB   = GetEnv("PGDB", "einoDB")
	PGHOST = GetEnv("PGHOST", "localhost")
	PGPORT = GetEnv("PGPORT", "5432")

	// Milvus
	MILVUSADDR = GetEnv("MILVUSADDR", "127.0.0.1:19530")

	// ES
	ESADDR = GetEnv("ESADDR", "http://localhost:9200")

	// 提示词
	EXTARACT = `
你是一个专业的合同数据录入员。请从以下合同文本中提取关键结构化信息。
当前日期: {{.CurrentDate}} (用于推算相对时间，如"有效期一年")

请严格按照以下规则提取字段 (JSON格式):

1. **party_a**: 甲方/委托方/出租方/雇主 (全称)。
2. **party_b**: 乙方/受托方/承租方/员工 (全称)。

3. **contract_type**: 合同类型。为了便于数据库索引，请优先将其归类为以下**标准类别**之一：
   - [物资采购合同, 销售合同, 房屋租赁合同, 劳动合同, 劳务派遣合同, 保密协议]
   - [软件开发合同, 技术服务合同, 居间服务合同, 咨询服务合同, 运维服务合同]
   - [借款合同, 担保合同, 股权转让协议, 投融资协议]
   - [建设工程合同, 装修工程合同, 品牌加盟合同, 框架合作协议, 补充协议]
   *如果以上均不匹配，请根据合同标题或内容提取最简短、通用的法律名称（不超过6个字，例如"赠与合同"）。

4. **sign_date**: 签署日期 (格式: YYYY-MM-DD)。如果文中未提及具体日期，留空。
5. **end_date**: 截止/到期日期 (格式: YYYY-MM-DD)。
   - 必须基于"签署日期"或"生效日期" + "有效期"进行推算。
   - 如果是"永久"、"长期"或未提及，留空。

6. **total_amount**: 合同总金额 (纯数字，单位: 元)。
   - 必须将"万元"、"亿元"、"美元"等统一换算为"人民币元"。
   - 如果不涉及金额(如保密协议)或金额不固定(如框架协议)，填 0。
   - 如果有多个金额，提取总包金额或上限金额。

7. **summary**: 简明摘要 (100字以内)。格式："A公司与B公司签署了XX合同，主要关于XX的交易/合作，总金额XX元，有效期至XX。"
8. **keywords**: 提取3-5个核心关键词 (用于全文检索)，如产品名、项目地、核心条款等。

文本内容:
{{.Content}}

Output JSON only:
`
)
