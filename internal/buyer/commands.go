package buyer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/control"
	"github.com/spf13/cobra"
)

// NewRegisterCommand registers a buyer against the control pane HTTP API (PostgreSQL-backed).
func NewRegisterCommand() *cobra.Command {
	var (
		controlURL string
		name       string
		email      string
		password   string
	)

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register a buyer account on the control pane",
		RunE: func(cmd *cobra.Command, args []string) error {
			var usedDef bool
			controlURL, usedDef = control.ResolveControlURL(controlURL)
			control.WarnIfDefaultControlURL(usedDef, controlURL)
			if strings.TrimSpace(name) == "" || strings.TrimSpace(email) == "" || password == "" {
				return fmt.Errorf("required: --name, --email, and --password")
			}
			cc := control.NewClient(controlURL)
			id, err := cc.RegisterBuyer(name, email, password)
			if err != nil {
				return err
			}
			fmt.Printf("registered buyer id: %s\n", id)
			return nil
		},
	}

	cmd.Flags().StringVar(&controlURL, "control-url", "", "Control pane base URL (optional; default "+control.DefaultControlURL+")")
	cmd.Flags().StringVar(&name, "name", "", "Display name")
	cmd.Flags().StringVar(&email, "email", "", "Email")
	cmd.Flags().StringVar(&password, "password", "", "Password")
	return cmd
}

// NewPromptCommand logs in to the control pane and sends one OpenAI-style completion to the buyer mesh API.
func NewPromptCommand() *cobra.Command {
	var (
		controlURL string
		apiURL     string
		email      string
		password   string
		model      string
		prompt     string
	)

	cmd := &cobra.Command{
		Use:   "prompt",
		Short: "Log in to the control pane and send one chat completion to the buyer HTTP API",
		RunE: func(cmd *cobra.Command, args []string) error {
			var usedDef bool
			controlURL, usedDef = control.ResolveControlURL(controlURL)
			control.WarnIfDefaultControlURL(usedDef, controlURL)
			apiURL = strings.TrimRight(strings.TrimSpace(apiURL), "/")
			if email == "" || password == "" || prompt == "" {
				return fmt.Errorf("required: --email, --password, and --prompt")
			}
			if apiURL == "" {
				apiURL = "http://127.0.0.1:8080"
			}
			if model == "" {
				model = "kmg-mock-1"
			}

			cc := control.NewClient(controlURL)
			tok, _, _, _, err := cc.LoginBuyer(email, password)
			if err != nil {
				return fmt.Errorf("control login: %w", err)
			}

			body := map[string]any{
				"model": model,
				"messages": []map[string]string{
					{"role": "user", "content": prompt},
				},
			}
			b, err := json.Marshal(body)
			if err != nil {
				return err
			}
			req, err := http.NewRequest(http.MethodPost, apiURL+"/v1/chat/completions", bytes.NewReader(b))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+tok)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			out, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				return fmt.Errorf("buyer API %s: %s", resp.Status, string(out))
			}
			fmt.Println(string(out))
			return nil
		},
	}

	cmd.Flags().StringVar(&controlURL, "control-url", "", "Control pane base URL (optional; default "+control.DefaultControlURL+")")
	cmd.Flags().StringVar(&apiURL, "api-url", "http://127.0.0.1:8080", "Buyer mesh HTTP API base URL")
	cmd.Flags().StringVar(&email, "email", "", "Buyer email")
	cmd.Flags().StringVar(&password, "password", "", "Buyer password")
	cmd.Flags().StringVar(&model, "model", "kmg-mock-1", "Model id for OpenAI-style request")
	cmd.Flags().StringVar(&prompt, "prompt", "", "User prompt text")
	return cmd
}
