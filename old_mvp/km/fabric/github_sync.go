package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// GitHubSync periodically pushes ledger + state files to a GitHub repo.
// This ensures data survives container rebuilds on Render.
type GitHubSync struct {
	token      string // GitHub PAT
	repo       string // e.g. "ArchieIndian/km-data"
	ledgerPath string
	statePath  string
	interval   time.Duration
	encKey     []byte // 32-byte AES-256 key derived from KM_SYNC_ENCRYPTION_KEY (nil if unset)
}

func NewGitHubSync(ledgerPath, statePath string) *GitHubSync {
	token := os.Getenv("KM_GITHUB_TOKEN")
	repo := os.Getenv("KM_GITHUB_REPO")
	if token == "" || repo == "" {
		return nil
	}

	interval := 60 * time.Second
	if v := os.Getenv("KM_SYNC_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			interval = d
		}
	}

	var encKey []byte
	if raw := os.Getenv("KM_SYNC_ENCRYPTION_KEY"); raw != "" {
		h := sha256.Sum256([]byte(raw))
		encKey = h[:]
		log.Printf("[github-sync] State file encryption enabled")
	} else {
		log.Printf("[github-sync] WARNING: KM_SYNC_ENCRYPTION_KEY not set — state file will be pushed in plaintext")
	}

	return &GitHubSync{
		token:      token,
		repo:       repo,
		ledgerPath: ledgerPath,
		statePath:  statePath,
		interval:   interval,
		encKey:     encKey,
	}
}

// encryptState encrypts data with AES-256-GCM using gs.encKey.
// Returns base64(nonce + ciphertext).
func (gs *GitHubSync) encryptState(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(gs.encKey)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher.NewGCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("rand nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil) // prepends nonce
	encoded := base64.StdEncoding.EncodeToString(ciphertext)
	return []byte(encoded), nil
}

// decryptState reverses encryptState: base64-decode, split nonce, AES-256-GCM open.
func (gs *GitHubSync) decryptState(encoded []byte) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	block, err := aes.NewCipher(gs.encKey)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("cipher.NewGCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

// Start runs the periodic sync in a goroutine.
// Also loads state from GitHub on first run (bootstrap from remote).
func (gs *GitHubSync) Start() {
	log.Printf("[github-sync] Syncing to %s every %s", gs.repo, gs.interval)

	// Bootstrap: always pull remote state on startup.
	// On Render, the filesystem is ephemeral — even if a local file exists
	// from a previous deploy, it may be stale or about to be wiped.
	gs.pullRemote(gs.ledgerPath, "ledger.txt")
	gs.pullRemote(gs.statePath, "state.json")

	go func() {
		ticker := time.NewTicker(gs.interval)
		defer ticker.Stop()

		for range ticker.C {
			gs.push()
		}
	}()
}

// push uploads both files to GitHub.
func (gs *GitHubSync) push() {
	gs.pushFile(gs.ledgerPath, "ledger.txt", "sync ledger")
	gs.pushFile(gs.statePath, "state.json", "sync state")
}

// pullRemote downloads a file from GitHub, overwriting any local copy.
// On ephemeral filesystems (like Render), stale local files must not
// prevent us from restoring the latest remote state.
func (gs *GitHubSync) pullRemote(localPath, remotePath string) {
	content, err := gs.getFile(remotePath)
	if err != nil {
		log.Printf("[github-sync] No remote %s found (or error: %v) — starting fresh", remotePath, err)
		return
	}

	// Decrypt state file if encryption key is available
	if remotePath == "state.json" && gs.encKey != nil {
		decrypted, err := gs.decryptState(content)
		if err != nil {
			log.Printf("[github-sync] Failed to decrypt %s (may be plaintext from before encryption was enabled): %v", remotePath, err)
			// Fall through with raw content — it may be unencrypted legacy data
		} else {
			content = decrypted
			log.Printf("[github-sync] Decrypted %s from GitHub", remotePath)
		}
	}

	if err := os.WriteFile(localPath, content, 0644); err != nil {
		log.Printf("[github-sync] Failed to write %s: %v", localPath, err)
		return
	}

	log.Printf("[github-sync] Restored %s from GitHub (%d bytes)", remotePath, len(content))
}

// getFile downloads a file from the GitHub repo.
func (gs *GitHubSync) getFile(path string) ([]byte, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", gs.repo, path)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+gs.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("not found")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if result.Encoding != "base64" {
		return nil, fmt.Errorf("unexpected encoding: %s", result.Encoding)
	}

	return base64.StdEncoding.DecodeString(result.Content)
}

// pushFile uploads a local file to GitHub, creating or updating it.
func (gs *GitHubSync) pushFile(localPath, remotePath, message string) {
	content, err := os.ReadFile(localPath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[github-sync] Failed to read %s: %v", localPath, err)
		}
		return
	}

	if len(content) == 0 {
		return // Don't push empty files
	}

	// Encrypt state file if encryption key is available
	pushContent := content
	if remotePath == "state.json" && gs.encKey != nil {
		encrypted, err := gs.encryptState(content)
		if err != nil {
			log.Printf("[github-sync] Failed to encrypt %s: %v", remotePath, err)
			return
		}
		pushContent = encrypted
		log.Printf("[github-sync] Encrypted %s before push (%d bytes plaintext -> %d bytes encrypted)", remotePath, len(content), len(pushContent))
	}

	// Get current file SHA (needed for updates)
	sha := gs.getFileSHA(remotePath)

	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", gs.repo, remotePath)

	body := map[string]string{
		"message": message,
		"content": base64.StdEncoding.EncodeToString(pushContent),
	}
	if sha != "" {
		body["sha"] = sha
	}

	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequest("PUT", url, bytes.NewReader(jsonBody))
	if err != nil {
		log.Printf("[github-sync] Failed to create request for %s: %v", remotePath, err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+gs.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[github-sync] Failed to push %s: %v", remotePath, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 || resp.StatusCode == 201 {
		log.Printf("[github-sync] Pushed %s (%d bytes)", remotePath, len(content))
	} else {
		respBody, _ := io.ReadAll(resp.Body)
		log.Printf("[github-sync] Push %s failed (status %d): %s", remotePath, resp.StatusCode, string(respBody))
	}
}

// getFileSHA returns the current SHA of a file in the repo (needed for updates).
func (gs *GitHubSync) getFileSHA(path string) string {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s", gs.repo, path)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+gs.token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	var result struct {
		SHA string `json:"sha"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.SHA
}

// ForcePush immediately pushes both files (used after state-changing operations).
func (gs *GitHubSync) ForcePush() {
	go gs.push()
}
