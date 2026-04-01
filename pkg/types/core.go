package types

type Skill struct {
	Name       string  `json:"name"`
	ModelName  string  `json:"modelName"`
	ModelType  string  `json:"modelType"`
	TuningTier string  `json:"tuningTier"`
	Price      float64 `json:"price"`
}

type RateLimits struct {
	MaxInputTokens  int `json:"maxInputTokens"`
	MaxOutputTokens int `json:"maxOutputTokens"`
	MaxTotalTokens  int `json:"maxTotalTokens"`
	HourlyTokens    int `json:"hourlyTokens"`
	DailyTokens     int `json:"dailyTokens"`
	TotalTokens     int `json:"totalTokens"`
}

type UsageCounters struct {
	RequestsServed int   `json:"requestsServed"`
	InputTokens    int   `json:"inputTokens"`
	OutputTokens   int   `json:"outputTokens"`
	TotalTokens    int   `json:"totalTokens"`
	HourlyTokens   int   `json:"hourlyTokens"`
	DailyTokens    int   `json:"dailyTokens"`
	HourBucketUnix int64 `json:"hourBucketUnix"`
	DayBucketUnix  int64 `json:"dayBucketUnix"`
}

type ResourceHints struct {
	CPUCores int   `json:"cpuCores"`
	MemoryMB int64 `json:"memoryMb"`
	GPUs     int   `json:"gpus"`
}

type NodeMetadata struct {
	PeerID        string        `json:"peerId"`
	Skills        []Skill       `json:"skills"`
	ModelName     string        `json:"modelName"`
	ModelType     string        `json:"modelType"`
	TuningTier    string        `json:"tuningTier"`
	Price         float64       `json:"price"`
	Reputation    float64       `json:"reputation"`
	OnDuty        bool          `json:"onDuty"`
	TokenLimits   RateLimits    `json:"tokenLimits"`
	ResourceHints ResourceHints `json:"resourceHints"`
}

type SellerNode struct {
	PeerID        string        `json:"peerId"`
	Skills        []Skill       `json:"skills"`
	ModelName     string        `json:"modelName"`
	ModelType     string        `json:"modelType"`
	TuningTier    string        `json:"tuningTier"`
	Price         float64       `json:"price"`
	Reputation    float64       `json:"reputation"`
	OnDuty        bool          `json:"onDuty"`
	TokenLimits   RateLimits    `json:"tokenLimits"`
	ResourceHints ResourceHints `json:"resourceHints"`
	Usage         UsageCounters `json:"usage"`
	Metadata      NodeMetadata  `json:"metadata"`
}

type BuyerNode struct {
	PeerID      string        `json:"peerId"`
	Skills      []Skill       `json:"skills"`
	ModelName   string        `json:"modelName"`
	ModelType   string        `json:"modelType"`
	TuningTier  string        `json:"tuningTier"`
	Price       float64       `json:"price"`
	Reputation  float64       `json:"reputation"`
	OnDuty      bool          `json:"onDuty"`
	TokenLimits RateLimits    `json:"tokenLimits"`
	Usage       UsageCounters `json:"usage"`
	Metadata    NodeMetadata  `json:"metadata"`
}

type InferenceRequest struct {
	RequestID   string     `json:"requestId"`
	BuyerPeerID string     `json:"buyerPeerId"`
	ModelName   string     `json:"modelName"`
	ModelType   string     `json:"modelType"`
	TuningTier  string     `json:"tuningTier"`
	Skill       Skill      `json:"skill"`
	Input       string     `json:"input"`
	MaxPrice    float64    `json:"maxPrice"`
	TokenLimits RateLimits `json:"tokenLimits"`
}

type InferenceResponse struct {
	RequestID      string        `json:"requestId"`
	SellerPeerID   string        `json:"sellerPeerId"`
	ModelName      string        `json:"modelName"`
	ModelType      string        `json:"modelType"`
	TuningTier     string        `json:"tuningTier"`
	Output         string        `json:"output"`
	PriceCharged   float64       `json:"priceCharged"`
	Reputation     float64       `json:"reputation"`
	OnDuty         bool          `json:"onDuty"`
	Success        bool          `json:"success"`
	Error          string        `json:"error,omitempty"`
	TokenUsage     UsageCounters `json:"tokenUsage"`
	SellerMetadata NodeMetadata  `json:"sellerMetadata"`
}
