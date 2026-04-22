package seller

import (
	"fmt"
	"sort"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/control"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
	"github.com/spf13/cobra"
)

// NewControlSetupCommand logs in and applies seller models/backend/duty in one step.
func NewControlSetupCommand() *cobra.Command {
	var (
		controlURL    string
		email         string
		password      string
		modelID       string
		modelName     string
		ratePerToken  float64
		hourlyTokens  int
		dailyTokens   int
		totalTokens   int
		onDuty        bool
		ollamaBaseURL string
		ollamaMaps    []string
	)

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "One-step seller setup (models + optional Ollama + duty)",
		Long:  "Logs in to the control pane, replaces seller models, optionally stores Ollama config, and sets on-duty.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var usedDef bool
			controlURL, usedDef = control.ResolveControlURL(controlURL)
			control.WarnIfDefaultControlURL(usedDef, controlURL)
			if strings.TrimSpace(email) == "" || strings.TrimSpace(password) == "" {
				return fmt.Errorf("required: --email and --password")
			}
			modelID = strings.TrimSpace(modelID)
			if modelID == "" {
				return fmt.Errorf("required: --model-id")
			}
			if ratePerToken <= 0 {
				return fmt.Errorf("required: --rate-per-token > 0")
			}
			if strings.TrimSpace(modelName) == "" {
				modelName = modelID
			}

			cc := control.NewClient(controlURL)
			tok, _, err := cc.LoginSeller(email, password)
			if err != nil {
				return fmt.Errorf("login seller: %w", err)
			}

			model := control.SellerModelRecord{
				ID:           modelID,
				Name:         modelName,
				SkillName:    modelID,
				ModelName:    modelName,
				ModelType:    "llm",
				TuningTier:   "base",
				HourlyTokens: hourlyTokens,
				DailyTokens:  dailyTokens,
				TotalTokens:  totalTokens,
				RatePerToken: ratePerToken,
				Active:       true,
			}
			prof, err := cc.PutSellerModels(tok, []control.SellerModelRecord{model})
			if err != nil {
				return fmt.Errorf("put seller models: %w", err)
			}

			ollamaCfg, err := parseOllamaConfig(ollamaBaseURL, ollamaMaps)
			if err != nil {
				return err
			}
			if ollamaCfg != nil {
				prof, err = cc.PutSellerOllama(tok, ollamaCfg)
				if err != nil {
					return fmt.Errorf("put seller ollama: %w", err)
				}
			}

			prof, err = cc.PutSellerDuty(tok, onDuty)
			if err != nil {
				return fmt.Errorf("put seller duty: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "seller setup complete: id=%s onDuty=%v models=%d ollamaConfigured=%v\n",
				prof.SellerID, prof.OnDuty, len(prof.Models), prof.Ollama != nil)
			return nil
		},
	}

	cmd.Flags().StringVar(&controlURL, "control-url", "", "Control pane base URL (optional; default "+control.DefaultControlURL+")")
	cmd.Flags().StringVar(&email, "email", "", "Seller email for control login (required)")
	cmd.Flags().StringVar(&password, "password", "", "Seller password for control login (required)")
	cmd.Flags().StringVar(&modelID, "model-id", "", "Model id / skill id (required)")
	cmd.Flags().StringVar(&modelName, "model-name", "", "Display model name (default: --model-id)")
	cmd.Flags().Float64Var(&ratePerToken, "rate-per-token", 0, "Rate per token (required, > 0)")
	cmd.Flags().IntVar(&hourlyTokens, "hourly-tokens", 0, "Hourly token limit (0 = unlimited)")
	cmd.Flags().IntVar(&dailyTokens, "daily-tokens", 0, "Daily token limit (0 = unlimited)")
	cmd.Flags().IntVar(&totalTokens, "total-tokens", 0, "Total token limit (0 = unlimited)")
	cmd.Flags().BoolVar(&onDuty, "on-duty", true, "Set seller on duty after setup")
	cmd.Flags().StringVar(&ollamaBaseURL, "ollama-base-url", "", "Optional Ollama base URL (example: http://127.0.0.1:11434)")
	cmd.Flags().StringArrayVar(&ollamaMaps, "ollama-map", nil, "Optional model mapping pair '<model-id>=<ollama-tag>' (repeatable)")
	return cmd
}

// NewControlStatusCommand prints seller profile state from control pane.
func NewControlStatusCommand() *cobra.Command {
	var (
		controlURL string
		email      string
		password   string
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show seller profile status from control pane",
		RunE: func(cmd *cobra.Command, args []string) error {
			var usedDef bool
			controlURL, usedDef = control.ResolveControlURL(controlURL)
			control.WarnIfDefaultControlURL(usedDef, controlURL)
			if strings.TrimSpace(email) == "" || strings.TrimSpace(password) == "" {
				return fmt.Errorf("required: --email and --password")
			}
			cc := control.NewClient(controlURL)
			tok, _, err := cc.LoginSeller(email, password)
			if err != nil {
				return fmt.Errorf("login seller: %w", err)
			}
			prof, err := cc.GetSellerMe(tok)
			if err != nil {
				return fmt.Errorf("get seller me: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "seller: id=%s email=%s onDuty=%v peerId=%s models=%d ollamaConfigured=%v\n",
				prof.SellerID, prof.Email, prof.OnDuty, prof.PeerID, len(prof.Models), prof.Ollama != nil)
			if len(prof.Models) > 0 {
				names := make([]string, 0, len(prof.Models))
				for _, m := range prof.Models {
					state := "inactive"
					if m.Active {
						state = "active"
					}
					names = append(names, fmt.Sprintf("%s(%s,rate=%g)", m.ID, state, m.RatePerToken))
				}
				sort.Strings(names)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "models: %s\n", strings.Join(names, ", "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&controlURL, "control-url", "", "Control pane base URL (optional; default "+control.DefaultControlURL+")")
	cmd.Flags().StringVar(&email, "email", "", "Seller email for control login (required)")
	cmd.Flags().StringVar(&password, "password", "", "Seller password for control login (required)")
	return cmd
}

// NewControlDutyCommand toggles seller on-duty/off-duty state.
func NewControlDutyCommand() *cobra.Command {
	var (
		controlURL string
		email      string
		password   string
	)
	root := &cobra.Command{
		Use:   "duty",
		Short: "Manage seller on-duty state",
	}
	build := func(use string, onDuty bool) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: fmt.Sprintf("Set seller onDuty=%v", onDuty),
			RunE: func(cmd *cobra.Command, args []string) error {
				var usedDef bool
				controlURL, usedDef = control.ResolveControlURL(controlURL)
				control.WarnIfDefaultControlURL(usedDef, controlURL)
				if strings.TrimSpace(email) == "" || strings.TrimSpace(password) == "" {
					return fmt.Errorf("required: --email and --password")
				}
				cc := control.NewClient(controlURL)
				tok, _, err := cc.LoginSeller(email, password)
				if err != nil {
					return fmt.Errorf("login seller: %w", err)
				}
				prof, err := cc.PutSellerDuty(tok, onDuty)
				if err != nil {
					return fmt.Errorf("put seller duty: %w", err)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "seller duty updated: onDuty=%v (sellerId=%s)\n", prof.OnDuty, prof.SellerID)
				return nil
			},
		}
	}
	onCmd := build("on", true)
	offCmd := build("off", false)
	for _, c := range []*cobra.Command{onCmd, offCmd} {
		c.Flags().StringVar(&controlURL, "control-url", "", "Control pane base URL (optional; default "+control.DefaultControlURL+")")
		c.Flags().StringVar(&email, "email", "", "Seller email for control login (required)")
		c.Flags().StringVar(&password, "password", "", "Seller password for control login (required)")
	}
	root.AddCommand(onCmd, offCmd)
	return root
}

func parseOllamaConfig(baseURL string, maps []string) (*types.OllamaSellerConfig, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" && len(maps) == 0 {
		return nil, nil
	}
	cfg := &types.OllamaSellerConfig{BaseURL: baseURL}
	for _, raw := range maps {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, fmt.Errorf("invalid --ollama-map %q (expected '<model-id>=<ollama-tag>')", raw)
		}
		cfg.Models = append(cfg.Models, types.OllamaModelDecl{
			ID:   strings.TrimSpace(parts[0]),
			Name: strings.TrimSpace(parts[1]),
		})
	}
	return cfg, nil
}
