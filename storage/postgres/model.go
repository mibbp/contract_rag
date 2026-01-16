package postgres

import (
	"eino-demo/types"
	"time"
)

// Contract 对应数据库里的 contracts 表
type Contract struct {
	// DocID 不使用 gorm.Model 的自增 ID，而是手动指定的 UUID
	DocID          string     `gorm:"column:doc_id;primaryKey;type:uuid"`
	FileName       string     `gorm:"column:file_name;type:varchar(255);not null"`
	PartyA         string     `gorm:"column:party_a;index"`
	PartyB         string     `gorm:"column:party_b;index"`
	ContractType   string     `gorm:"column:contract_type;type:varchar(50);index"`          // 合同类型
	ContractStatus int        `gorm:"column:contract_status;type:smallint;default:1;index"` // 如：生效中, 已过期
	SignDate       *time.Time `gorm:"column:sign_date;index"`
	EndDate        *time.Time `gorm:"column:end_date;index"` // 截止日期
	TotalAmount    float64    `gorm:"column:total_amount;type:decimal(15,2)"`
	//RawContent  string     `gorm:"column:raw_content;type:text"`
	Summary string `gorm:"column:summary;type:text"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName 强制指定表名
func (Contract) TableName() string {
	return "contracts"
}

func (c *Contract) IsActive() bool {
	return c.ContractStatus == types.StatusActive
}
