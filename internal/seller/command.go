package seller

import (
	"encoding/json"
	"fmt"
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

	root.AddCommand(newRegisterCommand(), newLoginCommand())
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
