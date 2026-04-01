package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// maskEmail masks an email address for safe logging, e.g. "m***@example.com".
func maskEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 || len(parts[0]) == 0 {
		return "***"
	}
	return string(parts[0][0]) + "***@" + parts[1]
}

// sendRecoveryEmail sends the reset token to the user's email via Resend.
// Returns true if email was sent successfully, false if not configured or failed.
func sendRecoveryEmail(toEmail, nodeName, resetToken, brokerURL string) bool {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		log.Println("[email] RESEND_API_KEY not set — skipping email")
		return false
	}

	fromEmail := os.Getenv("RESEND_FROM_EMAIL")
	if fromEmail == "" {
		fromEmail = "KnowledgeMesh <noreply@knowledgemesh.ai>"
	}

	body := map[string]interface{}{
		"from":    fromEmail,
		"to":      []string{toEmail},
		"subject": "KnowledgeMesh — Account Recovery",
		"html": fmt.Sprintf(`
			<h2>Account Recovery</h2>
			<p>You requested a secret reset for node <strong>%s</strong>.</p>
			<p>Your reset token:</p>
			<pre style="background:#f4f4f4;padding:12px;border-radius:4px;font-size:16px;">%s</pre>
			<p>Use it within 1 hour:</p>
			<pre style="background:#f4f4f4;padding:12px;border-radius:4px;">curl -X POST %s/reset-secret \
  -H "Content-Type: application/json" \
  -d '{"reset_token":"%s"}'</pre>
			<p>If you didn't request this, ignore this email.</p>
			<p style="color:#888;font-size:12px;">— KnowledgeMesh</p>
		`, nodeName, resetToken, brokerURL, resetToken),
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		log.Printf("[email] Failed to marshal email body: %v", err)
		return false
	}

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewReader(jsonBody))
	if err != nil {
		log.Printf("[email] Failed to create request: %v", err)
		return false
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[email] Failed to send email: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		var respBody bytes.Buffer
		respBody.ReadFrom(resp.Body)
		log.Printf("[email] Resend API error (%d): %s", resp.StatusCode, respBody.String())
		return false
	}

	maskedEmail := maskEmail(toEmail)
	log.Printf("[email] Recovery email sent to %s for node '%s'", maskedEmail, nodeName)
	return true
}
