package types

// --- 常量定义 ---

// 使用 int 0/1 表示状态，比 string 更高效
const (
	StatusExpired = 2 // 已过期
	StatusActive  = 1 // 生效中
)

// --- 结构体定义 ---

type SearchRequest struct {
	Query string `json:"query" binding:"required"`
}

// SearchIntent LLM 解析后的用户意图
type SearchIntent struct {
	Intent        string           `json:"intent"` // "structured_only", "semantic_only", "hybrid"
	Filters       FilterConditions `json:"filters"`
	SemanticQuery string           `json:"semantic_query"`
	Keywords      []string         `json:"keywords"`
}

// FilterConditions 过滤条件 (用于 Repo 查询)
type FilterConditions struct {
	AnyParty     []string `json:"any_party,omitempty"` // 改为数组，支持多个实体
	PartyA       string   `json:"party_a,omitempty"`
	PartyB       string   `json:"party_b,omitempty"`
	ContractType string   `json:"contract_type,omitempty"`

	// 这里用 string 接收 LLM 的输出 (如 "生效中"), Service 层负责转为 int
	Status string `json:"status,omitempty"`

	DateRange   *DateRange   `json:"date_range,omitempty"`
	AmountRange *AmountRange `json:"amount_range,omitempty"`
}

type DateRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type AmountRange struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}
