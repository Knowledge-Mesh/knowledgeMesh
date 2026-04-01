package types

// AnthropicModelDecl is a seller-declared model with optional per-model token caps.
type AnthropicModelDecl struct {
	ID           string `json:"id"`
	Name         string `json:"name"` // Anthropic model id, e.g. claude-3-5-haiku-20241022
	HourlyTokens int    `json:"hourlyTokens"`
	DailyTokens  int    `json:"dailyTokens"`
	TotalTokens  int    `json:"totalTokens"`
}

// AnthropicSellerConfig holds Anthropic integration settings for a seller (no secret in JSON).
// API key is read at runtime from the environment variable named by APIKeyEnv.
type AnthropicSellerConfig struct {
	APIKeyEnv string               `json:"apiKeyEnv"` // e.g. ANTHROPIC_API_KEY
	Models    []AnthropicModelDecl `json:"models"`
}
