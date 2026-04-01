package seller

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

var (
	ErrUserAlreadyExists   = errors.New("seller already exists")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrSellerNotFound      = errors.New("seller not found")
	ErrSellerOffDuty       = errors.New("seller is off duty")
	ErrHourlyLimitExceeded = errors.New("hourly token limit exceeded")
	ErrDailyLimitExceeded  = errors.New("daily token limit exceeded")
	ErrTotalLimitExceeded  = errors.New("total token limit exceeded")
)

type RegisterInput struct {
	Username      string
	Email         string
	Password      string
	PeerID        string
	Skills        []types.Skill
	ModelName     string
	ModelType     string
	TuningTier    string
	Price         float64
	ResourceHints types.ResourceHints
}

type LoginInput struct {
	UsernameOrEmail string
	Password        string
}

type sellerRecord struct {
	Username     string           `json:"username"`
	Email        string           `json:"email"`
	PasswordHash string           `json:"passwordHash"`
	Metadata     types.SellerNode `json:"metadata"`
}

type registryData struct {
	Sellers []sellerRecord `json:"sellers"`
}

type Registry struct {
	path string
	mu   sync.Mutex
}

type SellerStateManager struct {
	reg *Registry
}

func NewRegistry(path string) *Registry {
	return &Registry{path: path}
}

func NewSellerStateManager(reg *Registry) *SellerStateManager {
	return &SellerStateManager{reg: reg}
}

func DefaultRegistryPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "knowledgemesh", "seller_registry.json"), nil
}

func (r *Registry) Register(in RegisterInput) (types.SellerNode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	in.Username = strings.TrimSpace(in.Username)
	in.Email = strings.TrimSpace(strings.ToLower(in.Email))
	if in.Username == "" || in.Email == "" || in.Password == "" {
		return types.SellerNode{}, ErrInvalidCredentials
	}

	data, err := r.load()
	if err != nil {
		return types.SellerNode{}, err
	}
	for _, s := range data.Sellers {
		if strings.EqualFold(s.Username, in.Username) || strings.EqualFold(s.Email, in.Email) {
			return types.SellerNode{}, ErrUserAlreadyExists
		}
	}

	node := types.SellerNode{
		PeerID:        in.PeerID,
		Skills:        in.Skills,
		ModelName:     in.ModelName,
		ModelType:     in.ModelType,
		TuningTier:    in.TuningTier,
		Price:         in.Price,
		OnDuty:        true,
		ResourceHints: in.ResourceHints,
		Metadata: types.NodeMetadata{
			PeerID:        in.PeerID,
			Skills:        in.Skills,
			ModelName:     in.ModelName,
			ModelType:     in.ModelType,
			TuningTier:    in.TuningTier,
			Price:         in.Price,
			OnDuty:        true,
			ResourceHints: in.ResourceHints,
		},
	}

	data.Sellers = append(data.Sellers, sellerRecord{
		Username:     in.Username,
		Email:        in.Email,
		PasswordHash: hashPassword(in.Password),
		Metadata:     node,
	})
	if err := r.save(data); err != nil {
		return types.SellerNode{}, err
	}
	return node, nil
}

func (m *SellerStateManager) TurnOnDuty(peerID string) (types.SellerNode, error) {
	return m.reg.setOnDuty(peerID, true)
}

// TurnOnDutyWithAnthropic turns the seller on-duty and stores Anthropic integration config (API key via env only).
func (m *SellerStateManager) TurnOnDutyWithAnthropic(peerID string, cfg types.AnthropicSellerConfig) (types.SellerNode, error) {
	return m.reg.setOnDutyWithAnthropic(peerID, cfg)
}

// TurnOnDutyWithOpenAI turns the seller on-duty and stores OpenAI integration config (API key via env only).
func (m *SellerStateManager) TurnOnDutyWithOpenAI(peerID string, cfg types.OpenAISellerConfig) (types.SellerNode, error) {
	return m.reg.setOnDutyWithOpenAI(peerID, cfg)
}

// TurnOnDutyWithOllama turns the seller on-duty and stores Ollama integration config (mock backend until wired).
func (m *SellerStateManager) TurnOnDutyWithOllama(peerID string, cfg types.OllamaSellerConfig) (types.SellerNode, error) {
	return m.reg.setOnDutyWithOllama(peerID, cfg)
}

func (m *SellerStateManager) TurnOffDuty(peerID string) (types.SellerNode, error) {
	return m.reg.setOnDuty(peerID, false)
}

func (m *SellerStateManager) SetTokenLimits(peerID string, hourly, daily, total int) (types.SellerNode, error) {
	return m.reg.setTokenLimits(peerID, hourly, daily, total)
}

func (m *SellerStateManager) CheckAndConsume(peerID string, tokens int, now time.Time) (types.SellerNode, error) {
	return m.reg.checkAndConsume(peerID, tokens, now)
}

func (r *Registry) Login(in LoginInput) (types.SellerNode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	user := strings.TrimSpace(in.UsernameOrEmail)
	if user == "" || in.Password == "" {
		return types.SellerNode{}, ErrInvalidCredentials
	}

	data, err := r.load()
	if err != nil {
		return types.SellerNode{}, err
	}
	pwHash := hashPassword(in.Password)
	for _, s := range data.Sellers {
		if strings.EqualFold(s.Username, user) || strings.EqualFold(s.Email, user) {
			if s.PasswordHash != pwHash {
				return types.SellerNode{}, ErrInvalidCredentials
			}
			return s.Metadata, nil
		}
	}
	return types.SellerNode{}, ErrInvalidCredentials
}

func (r *Registry) load() (registryData, error) {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return registryData{}, err
	}
	b, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return registryData{Sellers: []sellerRecord{}}, nil
		}
		return registryData{}, err
	}
	var data registryData
	if err := json.Unmarshal(b, &data); err != nil {
		return registryData{}, err
	}
	if data.Sellers == nil {
		data.Sellers = []sellerRecord{}
	}
	return data, nil
}

func (r *Registry) save(data registryData) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.path, b, 0o600)
}

func (r *Registry) setOnDuty(peerID string, onDuty bool) (types.SellerNode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := r.load()
	if err != nil {
		return types.SellerNode{}, err
	}
	idx := findSellerByPeerID(data.Sellers, peerID)
	if idx == -1 {
		return types.SellerNode{}, ErrSellerNotFound
	}

	node := data.Sellers[idx].Metadata
	node.OnDuty = onDuty
	node.Metadata.OnDuty = onDuty
	data.Sellers[idx].Metadata = node

	if err := r.save(data); err != nil {
		return types.SellerNode{}, err
	}
	return node, nil
}

func (r *Registry) setOnDutyWithAnthropic(peerID string, cfg types.AnthropicSellerConfig) (types.SellerNode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := r.load()
	if err != nil {
		return types.SellerNode{}, err
	}
	idx := findSellerByPeerID(data.Sellers, peerID)
	if idx == -1 {
		return types.SellerNode{}, ErrSellerNotFound
	}

	node := data.Sellers[idx].Metadata
	node.OnDuty = true
	node.Metadata.OnDuty = true
	c := cfg
	node.Anthropic = &c
	node.OpenAI = nil
	node.Ollama = nil
	data.Sellers[idx].Metadata = node

	if err := r.save(data); err != nil {
		return types.SellerNode{}, err
	}
	return node, nil
}

func (r *Registry) setOnDutyWithOpenAI(peerID string, cfg types.OpenAISellerConfig) (types.SellerNode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := r.load()
	if err != nil {
		return types.SellerNode{}, err
	}
	idx := findSellerByPeerID(data.Sellers, peerID)
	if idx == -1 {
		return types.SellerNode{}, ErrSellerNotFound
	}

	node := data.Sellers[idx].Metadata
	node.OnDuty = true
	node.Metadata.OnDuty = true
	c := cfg
	node.OpenAI = &c
	node.Anthropic = nil
	node.Ollama = nil
	data.Sellers[idx].Metadata = node

	if err := r.save(data); err != nil {
		return types.SellerNode{}, err
	}
	return node, nil
}

func (r *Registry) setOnDutyWithOllama(peerID string, cfg types.OllamaSellerConfig) (types.SellerNode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := r.load()
	if err != nil {
		return types.SellerNode{}, err
	}
	idx := findSellerByPeerID(data.Sellers, peerID)
	if idx == -1 {
		return types.SellerNode{}, ErrSellerNotFound
	}

	node := data.Sellers[idx].Metadata
	node.OnDuty = true
	node.Metadata.OnDuty = true
	c := cfg
	node.Ollama = &c
	node.OpenAI = nil
	node.Anthropic = nil
	data.Sellers[idx].Metadata = node

	if err := r.save(data); err != nil {
		return types.SellerNode{}, err
	}
	return node, nil
}

func (r *Registry) setTokenLimits(peerID string, hourly, daily, total int) (types.SellerNode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := r.load()
	if err != nil {
		return types.SellerNode{}, err
	}
	idx := findSellerByPeerID(data.Sellers, peerID)
	if idx == -1 {
		return types.SellerNode{}, ErrSellerNotFound
	}

	node := data.Sellers[idx].Metadata
	node.TokenLimits.HourlyTokens = hourly
	node.TokenLimits.DailyTokens = daily
	node.TokenLimits.TotalTokens = total
	node.Metadata.TokenLimits = node.TokenLimits
	data.Sellers[idx].Metadata = node

	if err := r.save(data); err != nil {
		return types.SellerNode{}, err
	}
	return node, nil
}

func (r *Registry) checkAndConsume(peerID string, tokens int, now time.Time) (types.SellerNode, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := r.load()
	if err != nil {
		return types.SellerNode{}, err
	}
	idx := findSellerByPeerID(data.Sellers, peerID)
	if idx == -1 {
		return types.SellerNode{}, ErrSellerNotFound
	}

	node := data.Sellers[idx].Metadata
	if !node.OnDuty {
		return types.SellerNode{}, ErrSellerOffDuty
	}

	hourBucketUnix := now.UTC().Truncate(time.Hour).Unix()
	dayBucketUnix := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC).Unix()
	if node.Usage.HourBucketUnix != hourBucketUnix {
		node.Usage.HourlyTokens = 0
		node.Usage.HourBucketUnix = hourBucketUnix
	}
	if node.Usage.DayBucketUnix != dayBucketUnix {
		node.Usage.DailyTokens = 0
		node.Usage.DayBucketUnix = dayBucketUnix
	}

	if node.TokenLimits.HourlyTokens > 0 && node.Usage.HourlyTokens+tokens > node.TokenLimits.HourlyTokens {
		return types.SellerNode{}, ErrHourlyLimitExceeded
	}
	if node.TokenLimits.DailyTokens > 0 && node.Usage.DailyTokens+tokens > node.TokenLimits.DailyTokens {
		return types.SellerNode{}, ErrDailyLimitExceeded
	}
	if node.TokenLimits.TotalTokens > 0 && node.Usage.TotalTokens+tokens > node.TokenLimits.TotalTokens {
		return types.SellerNode{}, ErrTotalLimitExceeded
	}

	node.Usage.HourlyTokens += tokens
	node.Usage.DailyTokens += tokens
	node.Usage.TotalTokens += tokens
	node.Usage.RequestsServed++
	data.Sellers[idx].Metadata = node

	if err := r.save(data); err != nil {
		return types.SellerNode{}, err
	}
	return node, nil
}

func findSellerByPeerID(sellers []sellerRecord, peerID string) int {
	for i, s := range sellers {
		if s.Metadata.PeerID == peerID {
			return i
		}
	}
	return -1
}

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}
