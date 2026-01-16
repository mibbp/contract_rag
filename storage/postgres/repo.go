package postgres

import (
	"context"
	"eino-demo/types"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ContractRepo 封装对 Contract 表的所有操作
type ContractRepo struct {
	db *gorm.DB
}

// NewContractRepo 构造函数
func NewContractRepo(db *gorm.DB) *ContractRepo {
	return &ContractRepo{db: db}
}

// Create 创建新合同记录
func (r *ContractRepo) Create(ctx context.Context, contract *Contract) error {
	// WithContext 允许你在超时的时候取消数据库操作
	return r.db.WithContext(ctx).Create(contract).Error
}

// GetByDocID 根据 UUID 查询合同详情
func (r *ContractRepo) GetByDocID(ctx context.Context, docID string) (*Contract, error) {
	var contract Contract
	err := r.db.WithContext(ctx).
		Where("doc_id = ?", docID).
		First(&contract).Error
	if err != nil {
		return nil, err
	}
	return &contract, nil
}

// GetByDocID 根据 FileName 查询合同详情
func (r *ContractRepo) GetByFileName(ctx context.Context, filename string) (*Contract, error) {
	var contract Contract
	err := r.db.WithContext(ctx).
		Where("file_name = ?", filename).
		First(&contract).Error
	if err != nil {
		return nil, err
	}
	return &contract, nil
}

// SearchByKeyword 简单的 SQL 模糊搜索 (如果不用 ES 的话可以用这个兜底)
func (r *ContractRepo) SearchByKeyword(ctx context.Context, keyword string) ([]Contract, error) {
	var results []Contract
	pattern := "%" + keyword + "%"
	err := r.db.WithContext(ctx).
		Where("party_a LIKE ? OR party_b LIKE ? OR file_name LIKE ?", pattern, pattern, pattern).
		Limit(10).
		Find(&results).Error
	return results, err
}

func (r *ContractRepo) Delete(ctx context.Context, id string) error {
	// 这里的 &Contract{} 是为了告诉 GORM 要删哪张表
	// WithContext(ctx) 确保链路追踪和超时控制生效
	result := r.db.WithContext(ctx).Where("doc_id = ?", id).Delete(&Contract{})

	return result.Error
}

// SearchContracts 核心：根据结构化条件筛选 DocID
// docIDs: 可选的文档ID列表（用于 ES 先过滤后再传给 PG）
func (r *ContractRepo) SearchContracts(ctx context.Context, conditions *types.FilterConditions, docIDs ...[]string) ([]string, error) {
	// 只查 doc_id，性能最高
	tx := r.db.WithContext(ctx).Model(&Contract{}).Select("doc_id")

	// 1. 如果传入了 docIDs（ES 先过滤的结果），用 IN 查询缩小范围
	if docIDs != nil && len(docIDs) > 0 && docIDs[0] != nil && len(docIDs[0]) > 0 {
		tx = tx.Where("doc_id IN ?", docIDs[0])
	}

	// 2. 处理模糊参与方 (AnyParty) - 仅当没有传入 docIDs 时才使用（避免重复过滤）
	// 如果 docIDs 已经由 ES 过滤了，这里就不需要再用 LIKE 查 AnyParty
	if (len(docIDs) == 0 || docIDs[0] == nil || len(docIDs[0]) == 0) && len(conditions.AnyParty) > 0 {
		// 构建子查询：(party_a LIKE '%p1%' OR party_b LIKE '%p1%') OR (party_a LIKE '%p2%' OR party_b LIKE '%p2%')
		var orConditions []string
		var orValues []interface{}
		for _, party := range conditions.AnyParty {
			pattern := "%" + party + "%"
			orConditions = append(orConditions, "(party_a LIKE ? OR party_b LIKE ?)")
			orValues = append(orValues, pattern, pattern)
		}
		// 用 OR 连接所有实体的条件
		tx = tx.Where(strings.Join(orConditions, " OR "), orValues...)
	}

	// 3. 处理精确参与方
	if conditions.PartyA != "" {
		tx = tx.Where("party_a LIKE ?", "%"+conditions.PartyA+"%")
	}
	if conditions.PartyB != "" {
		tx = tx.Where("party_b LIKE ?", "%"+conditions.PartyB+"%")
	}

	// 4. 合同类型
	if conditions.ContractType != "" {
		// 假设类型可能存在于 contract_type 字段或 file_name 中
		tx = tx.Where("contract_type = ? OR file_name LIKE ?", conditions.ContractType, "%"+conditions.ContractType+"%")
	}

	if conditions.Status != "" {
		if conditions.Status == "已过期" || conditions.Status == "expired" {
			tx = tx.Where("contract_status = ?", types.StatusExpired)
		} else {
			tx = tx.Where("contract_status = ?", types.StatusActive)
		}
	} else {
		// 默认策略：如果用户没提，是否只查生效中？这里假设默认查生效中
		// tx = tx.Where("contract_status = ?", types.StatusActive)
	}

	// 5. 日期范围
	if conditions.DateRange != nil {
		if conditions.DateRange.Start != "" {
			tx = tx.Where("sign_date >= ?", conditions.DateRange.Start)
		}
		if conditions.DateRange.End != "" {
			tx = tx.Where("sign_date <= ?", conditions.DateRange.End)
		}
	}

	// 6. 金额范围
	if conditions.AmountRange != nil {
		if conditions.AmountRange.Min != nil {
			tx = tx.Where("total_amount >= ?", *conditions.AmountRange.Min)
		}
		if conditions.AmountRange.Max != nil {
			tx = tx.Where("total_amount <= ?", *conditions.AmountRange.Max)
		}
	}

	var resultDocIDs []string
	err := tx.Find(&resultDocIDs).Error
	return resultDocIDs, err
}

// ExpireContracts 用于定时任务批量更新过期状态
func (r *ContractRepo) ExpireContracts(ctx context.Context, now time.Time) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&Contract{}).
		Where("contract_status = ? AND end_date < ?", types.StatusActive, now).
		Update("contract_status", types.StatusExpired)
	return result.RowsAffected, result.Error
}
