package types

import "time"

type ContractRawData struct {
	PartyA       string      `json:"party_a" jsonschema:"description=合同甲方的全称,required"`
	PartyB       string      `json:"party_b" jsonschema:"description=合同乙方的全称,required"`
	SignDate     *string     `json:"sign_date" jsonschema:"description=签署日期，尽量转换为YYYY-MM-DD格式，如果没找到返回空字符串"`
	EndDate      *string     `json:"end_date" jsonschema:"description=结束日期，尽量转换为YYYY-MM-DD格式，如果没找到返回空字符串"`
	ContractType string      `json:"contract_type" jsonschema:"description=合同类型，如采购合同、劳动合同"`
	TotalAmount  interface{} `json:"total_amount" jsonschema:"description=合同总金额，提取纯数字"`
	Summary      string      `json:"summary" jsonschema:"description=合同内容的简短摘要"`
	Keywords     []string    `json:"keywords" jsonschema:"description=提取合同内容的3到5个关键术语或标签"`
}

// 2. 存入 PostgreSQL 的最终模型 (强类型)
type ContractEntity struct {
	ID           string     `gorm:"type:uuid;primary_key"`
	PartyA       string     `gorm:"type:varchar(255)"`
	PartyB       string     `gorm:"type:varchar(255)"`
	SignDate     *time.Time `gorm:"type:date"` // 指针类型，允许为 null
	EndDate      *time.Time `gorm:"type:date"`
	ContractType string     `gorm:"type:varchar(50)"`
	TotalAmount  float64    `gorm:"type:decimal(15,2)"`
	Summary      string     `gorm:"type:text"`
	Keywords     []string   `gorm:"type:text[]"` // 如果 PG 也要存
	//RawText     string     `gorm:"type:text"` // 可选：存原始文本
}
