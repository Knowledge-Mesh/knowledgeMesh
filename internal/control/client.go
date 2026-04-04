package control

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

// Client calls the control pane HTTP API (outbound, used by buyer mesh / CLI).
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient returns a client for the given control pane base URL.
func NewClient(baseURL string) *Client {
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return &Client{
		BaseURL: u,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) doJSON(method, path string, body any, out any, bearer string) error {
	if c.BaseURL == "" {
		return fmt.Errorf("control client: empty base URL")
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		if e.Error != "" {
			return fmt.Errorf("control: %s", e.Error)
		}
		return fmt.Errorf("control: HTTP %d", resp.StatusCode)
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}

// RegisterBuyer registers with the control pane; returns buyer id.
func (c *Client) RegisterBuyer(name, email, password string) (buyerID string, err error) {
	var out struct {
		BuyerID string `json:"buyerId"`
	}
	err = c.doJSON(http.MethodPost, "/v1/control/buyers/register", map[string]string{
		"name":     name,
		"email":    email,
		"password": password,
	}, &out, "")
	if err != nil {
		return "", err
	}
	return out.BuyerID, nil
}

// LoginBuyer returns access token and buyer metadata.
func (c *Client) LoginBuyer(email, password string) (accessToken, buyerID, name, em string, err error) {
	var out struct {
		AccessToken string `json:"accessToken"`
		BuyerID     string `json:"buyerId"`
		Name        string `json:"name"`
		Email       string `json:"email"`
	}
	err = c.doJSON(http.MethodPost, "/v1/control/buyers/login", map[string]string{
		"email":    email,
		"password": password,
	}, &out, "")
	if err != nil {
		return "", "", "", "", err
	}
	return out.AccessToken, out.BuyerID, out.Name, out.Email, nil
}

// RegisterSeller registers a seller account on the control pane.
func (c *Client) RegisterSeller(name, email, password string) (sellerID string, err error) {
	var out struct {
		SellerID string `json:"sellerId"`
	}
	err = c.doJSON(http.MethodPost, "/v1/control/sellers/register", map[string]string{
		"name":     name,
		"email":    email,
		"password": password,
	}, &out, "")
	if err != nil {
		return "", err
	}
	return out.SellerID, nil
}

// LoginSeller returns an access token and seller profile from the control pane.
func (c *Client) LoginSeller(email, password string) (accessToken string, prof SellerProfile, err error) {
	var out struct {
		AccessToken string                `json:"accessToken"`
		SellerID    string                `json:"sellerId"`
		Name        string                `json:"name"`
		Email       string                `json:"email"`
		OnDuty      bool                  `json:"onDuty"`
		PeerID      string                `json:"peerId"`
		Models      []SellerModelRecord   `json:"models"`
	}
	err = c.doJSON(http.MethodPost, "/v1/control/sellers/login", map[string]string{
		"email":    email,
		"password": password,
	}, &out, "")
	if err != nil {
		return "", SellerProfile{}, err
	}
	return out.AccessToken, SellerProfile{
		SellerID: out.SellerID,
		Name:     out.Name,
		Email:    out.Email,
		OnDuty:   out.OnDuty,
		PeerID:   out.PeerID,
		Models:   out.Models,
	}, nil
}

// GetSellerMe fetches the current seller (Bearer token).
func (c *Client) GetSellerMe(token string) (SellerProfile, error) {
	var out struct {
		SellerID string              `json:"sellerId"`
		Name     string              `json:"name"`
		Email    string              `json:"email"`
		OnDuty   bool                `json:"onDuty"`
		PeerID   string              `json:"peerId"`
		Models   []SellerModelRecord `json:"models"`
	}
	err := c.doJSON(http.MethodGet, "/v1/control/sellers/me", nil, &out, token)
	if err != nil {
		return SellerProfile{}, err
	}
	return SellerProfile{
		SellerID: out.SellerID,
		Name:     out.Name,
		Email:    out.Email,
		OnDuty:   out.OnDuty,
		PeerID:   out.PeerID,
		Models:   out.Models,
	}, nil
}

// PutSellerDuty updates on-duty flag.
func (c *Client) PutSellerDuty(token string, onDuty bool) (SellerProfile, error) {
	var out map[string]any
	err := c.doJSON(http.MethodPut, "/v1/control/sellers/me/duty", map[string]bool{"onDuty": onDuty}, &out, token)
	if err != nil {
		return SellerProfile{}, err
	}
	return decodeSellerProfileMap(out)
}

// PutSellerModels replaces all models.
func (c *Client) PutSellerModels(token string, models []SellerModelRecord) (SellerProfile, error) {
	var out map[string]any
	err := c.doJSON(http.MethodPut, "/v1/control/sellers/me/models", map[string]any{"models": models}, &out, token)
	if err != nil {
		return SellerProfile{}, err
	}
	return decodeSellerProfileMap(out)
}

// PostSellerPresence reports libp2p peer id to the control pane.
func (c *Client) PostSellerPresence(token, peerID string) (SellerProfile, error) {
	var out map[string]any
	err := c.doJSON(http.MethodPost, "/v1/control/sellers/me/presence", map[string]string{"peerId": peerID}, &out, token)
	if err != nil {
		return SellerProfile{}, err
	}
	return decodeSellerProfileMap(out)
}

func decodeSellerProfileMap(m map[string]any) (SellerProfile, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return SellerProfile{}, err
	}
	var p SellerProfile
	if err := json.Unmarshal(b, &p); err != nil {
		return SellerProfile{}, err
	}
	return p, nil
}

// InferenceMatchResponse is returned by the control pane after matchmaking.
type InferenceMatchResponse struct {
	RequestID    string  `json:"requestId"`
	SellerID     string  `json:"sellerId"`
	SellerPeerID string  `json:"sellerPeerId"`
	Price        float64 `json:"price"`
	Reputation   float64 `json:"reputation"`
}

// PostInferenceMatch authenticates the buyer and returns the selected seller connection metadata.
func (c *Client) PostInferenceMatch(token string, req types.InferenceRequest) (InferenceMatchResponse, error) {
	var out InferenceMatchResponse
	err := c.doJSON(http.MethodPost, "/v1/control/buyers/me/inference/match", req, &out, token)
	return out, err
}

// PostBuyerInferenceTracking records buyer-side prompt / tracking metadata for an inference request.
func (c *Client) PostBuyerInferenceTracking(token, requestID, phase string, meta map[string]any) error {
	return c.doJSON(http.MethodPost, "/v1/control/buyers/me/inference/tracking", map[string]any{
		"requestId": requestID,
		"phase":     phase,
		"meta":      meta,
	}, nil, token)
}

// PostBuyerInferenceComplete settles billing for a completed inference (buyer wallet debit, seller credit).
func (c *Client) PostBuyerInferenceComplete(token, requestID string, totalTokens int64, success bool, meta map[string]any) error {
	return c.doJSON(http.MethodPost, "/v1/control/buyers/me/inference/complete", map[string]any{
		"requestId":   requestID,
		"totalTokens": totalTokens,
		"success":     success,
		"meta":        meta,
	}, nil, token)
}

// PostSellerInferenceTracking records seller-side execution results for audit and reconciliation.
func (c *Client) PostSellerInferenceTracking(token, requestID string, totalTokens int64, success bool, meta map[string]any) error {
	return c.doJSON(http.MethodPost, "/v1/control/sellers/me/inference/tracking", map[string]any{
		"requestId":   requestID,
		"totalTokens": totalTokens,
		"success":     success,
		"meta":        meta,
	}, nil, token)
}
