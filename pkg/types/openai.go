package types

// OpenAIModelDecl is a seller-declared model with optional per-model token caps.
type OpenAIModelDecl struct {
	ID           string `json:"id"`
	Name         string `json:"name"` // OpenAI model id, e.g. gpt-4o-mini
	HourlyTokens int    `json:"hourlyTokens"`
	DailyTokens  int    `json:"dailyTokens"`
	TotalTokens  int    `json:"totalTokens"`
}

// OpenAISellerConfig holds OpenAI integration settings (no API key in JSON).
// The API key is read at runtime from the environment variable named by APIKeyEnv.
type OpenAISellerConfig struct {
	APIKeyEnv string            `json:"apiKeyEnv"` // e.g. OPENAI_API_KEY
	BaseURL   string            `json:"baseURL,omitempty"`
	Models    []OpenAIModelDecl `json:"models"`
}
