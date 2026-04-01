package types

// OllamaModelDecl is a seller-declared model with optional per-model token caps.
type OllamaModelDecl struct {
	ID           string `json:"id"`
	Name         string `json:"name"` // Ollama model tag, e.g. llama3:latest
	HourlyTokens int    `json:"hourlyTokens"`
	DailyTokens  int    `json:"dailyTokens"`
	TotalTokens  int    `json:"totalTokens"`
}

// OllamaSellerConfig holds Ollama endpoint and declared models (no secrets).
// BaseURL is the Ollama API root, e.g. http://127.0.0.1:11434 (used by a real backend later).
type OllamaSellerConfig struct {
	BaseURL string            `json:"baseURL,omitempty"`
	Models  []OllamaModelDecl `json:"models"`
}
