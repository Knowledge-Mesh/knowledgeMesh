package seller

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
	"github.com/spf13/cobra"
)

func NewCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "start",
		Short: "Start seller module",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("seller module started")
		},
	}

	root.AddCommand(newRegisterCommand(), newLoginCommand(), newOnDutyCommand())
	return root
}

func newRegisterCommand() *cobra.Command {
	var (
		username   string
		email      string
		password   string
		peerID     string
		skillsRaw  string
		modelName  string
		modelType  string
		tuningTier string
		price      float64
		cpuCores   int
		memoryMB   int64
		gpus       int
	)

	cmd := &cobra.Command{
		Use:   "register",
		Short: "Register seller in local registry",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := DefaultRegistryPath()
			if err != nil {
				return err
			}
			reg := NewRegistry(path)
			node, err := reg.Register(RegisterInput{
				Username:   username,
				Email:      email,
				Password:   password,
				PeerID:     peerID,
				Skills:     parseSkills(skillsRaw, modelName, modelType, tuningTier, price),
				ModelName:  modelName,
				ModelType:  modelType,
				TuningTier: tuningTier,
				Price:      price,
				ResourceHints: types.ResourceHints{
					CPUCores: cpuCores,
					MemoryMB: memoryMB,
					GPUs:     gpus,
				},
			})
			if err != nil {
				return err
			}
			return printJSON(node)
		},
	}

	cmd.Flags().StringVar(&username, "username", "", "Username")
	cmd.Flags().StringVar(&email, "email", "", "Email")
	cmd.Flags().StringVar(&password, "password", "", "Password")
	cmd.Flags().StringVar(&peerID, "peer-id", "", "Peer ID")
	cmd.Flags().StringVar(&skillsRaw, "skills", "", "Comma separated skills")
	cmd.Flags().StringVar(&modelName, "model-name", "", "Model name")
	cmd.Flags().StringVar(&modelType, "model-type", "llm", "Model type")
	cmd.Flags().StringVar(&tuningTier, "tuning-tier", "base", "Tuning tier")
	cmd.Flags().Float64Var(&price, "price", 0, "Price")
	cmd.Flags().IntVar(&cpuCores, "cpu-cores", 0, "CPU cores")
	cmd.Flags().Int64Var(&memoryMB, "memory-mb", 0, "Memory in MB")
	cmd.Flags().IntVar(&gpus, "gpus", 0, "GPU count")

	_ = cmd.MarkFlagRequired("username")
	_ = cmd.MarkFlagRequired("email")
	_ = cmd.MarkFlagRequired("password")
	return cmd
}

func newOnDutyCommand() *cobra.Command {
	var (
		peerID           string
		anthropicCfgPath string
		openaiCfgPath    string
		ollamaCfgPath    string
	)
	cmd := &cobra.Command{
		Use:   "on-duty",
		Short: "Turn seller on-duty with optional Anthropic, OpenAI, or Ollama config",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := DefaultRegistryPath()
			if err != nil {
				return err
			}
			reg := NewRegistry(path)
			mgr := NewSellerStateManager(reg)
			n := 0
			if strings.TrimSpace(anthropicCfgPath) != "" {
				n++
			}
			if strings.TrimSpace(openaiCfgPath) != "" {
				n++
			}
			if strings.TrimSpace(ollamaCfgPath) != "" {
				n++
			}
			if n > 1 {
				return fmt.Errorf("use only one of --anthropic-config, --openai-config, or --ollama-config")
			}
			if strings.TrimSpace(ollamaCfgPath) != "" {
				b, err := os.ReadFile(ollamaCfgPath)
				if err != nil {
					return err
				}
				var cfg types.OllamaSellerConfig
				if err := json.Unmarshal(b, &cfg); err != nil {
					return err
				}
				node, err := mgr.TurnOnDutyWithOllama(peerID, cfg)
				if err != nil {
					return err
				}
				return printJSON(node)
			}
			if strings.TrimSpace(openaiCfgPath) != "" {
				b, err := os.ReadFile(openaiCfgPath)
				if err != nil {
					return err
				}
				var cfg types.OpenAISellerConfig
				if err := json.Unmarshal(b, &cfg); err != nil {
					return err
				}
				node, err := mgr.TurnOnDutyWithOpenAI(peerID, cfg)
				if err != nil {
					return err
				}
				return printJSON(node)
			}
			if strings.TrimSpace(anthropicCfgPath) != "" {
				b, err := os.ReadFile(anthropicCfgPath)
				if err != nil {
					return err
				}
				var cfg types.AnthropicSellerConfig
				if err := json.Unmarshal(b, &cfg); err != nil {
					return err
				}
				node, err := mgr.TurnOnDutyWithAnthropic(peerID, cfg)
				if err != nil {
					return err
				}
				return printJSON(node)
			}
			node, err := mgr.TurnOnDuty(peerID)
			if err != nil {
				return err
			}
			return printJSON(node)
		},
	}
	cmd.Flags().StringVar(&peerID, "peer-id", "", "Seller peer ID")
	cmd.Flags().StringVar(&anthropicCfgPath, "anthropic-config", "", "JSON file with apiKeyEnv and models")
	cmd.Flags().StringVar(&openaiCfgPath, "openai-config", "", "JSON file with apiKeyEnv and models")
	cmd.Flags().StringVar(&ollamaCfgPath, "ollama-config", "", "JSON file with baseURL and models")
	_ = cmd.MarkFlagRequired("peer-id")
	return cmd
}

func newLoginCommand() *cobra.Command {
	var (
		usernameOrEmail string
		password        string
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Login seller and return metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := DefaultRegistryPath()
			if err != nil {
				return err
			}
			reg := NewRegistry(path)
			node, err := reg.Login(LoginInput{
				UsernameOrEmail: usernameOrEmail,
				Password:        password,
			})
			if err != nil {
				return err
			}
			return printJSON(node)
		},
	}
	cmd.Flags().StringVar(&usernameOrEmail, "user", "", "Username or email")
	cmd.Flags().StringVar(&password, "password", "", "Password")
	_ = cmd.MarkFlagRequired("user")
	_ = cmd.MarkFlagRequired("password")
	return cmd
}

func parseSkills(raw, modelName, modelType, tuningTier string, price float64) []types.Skill {
	parts := strings.Split(raw, ",")
	out := make([]types.Skill, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name == "" {
			continue
		}
		out = append(out, types.Skill{
			Name:       name,
			ModelName:  modelName,
			ModelType:  modelType,
			TuningTier: tuningTier,
			Price:      price,
		})
	}
	return out
}

func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
