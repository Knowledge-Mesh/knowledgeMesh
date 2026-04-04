package control

// SellerModelRecord is a declared model with limits, rate, and active flag (JSON / DB).
type SellerModelRecord struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	SkillName    string         `json:"skillName"`
	ModelName    string         `json:"modelName"`
	ModelType    string         `json:"modelType"`
	TuningTier   string         `json:"tuningTier"`
	HourlyTokens int            `json:"hourlyTokens"`
	DailyTokens  int            `json:"dailyTokens"`
	TotalTokens  int            `json:"totalTokens"`
	RatePerToken float64        `json:"ratePerToken"`
	Active       bool           `json:"active"`
	Meta         map[string]any `json:"meta,omitempty"`
}
