package seller

import (
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/internal/control"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

// SellerNodeFromControlWithOllama builds a SellerNode with Ollama config when ollamaURL is non-empty.
func SellerNodeFromControlWithOllama(peerID string, prof control.SellerProfile, ollamaURL string) types.SellerNode {
	node := SellerNodeFromControl(peerID, prof)
	if strings.TrimSpace(ollamaURL) != "" {
		var models []types.OllamaModelDecl
		for _, m := range prof.Models {
			if m.Active {
				models = append(models, types.OllamaModelDecl{
					ID:   m.ID,
					Name: m.ModelName,
				})
			}
		}
		node.Ollama = &types.OllamaSellerConfig{
			BaseURL: ollamaURL,
			Models:  models,
		}
	}
	return node
}

// SellerNodeFromControl builds a SellerNode for inference + matchmaking from control pane profile.
func SellerNodeFromControl(peerID string, prof control.SellerProfile) types.SellerNode {
	var skills []types.Skill
	for _, m := range prof.Models {
		if !m.Active {
			continue
		}
		skills = append(skills, types.Skill{
			Name:       m.SkillName,
			ModelName:  m.ModelName,
			ModelType:  m.ModelType,
			TuningTier: m.TuningTier,
			Price:      m.RatePerToken,
		})
	}
	modelName, modelType, tuning, price := "", "", "", 0.0
	if len(skills) > 0 {
		modelName = skills[0].ModelName
		modelType = skills[0].ModelType
		tuning = skills[0].TuningTier
		price = skills[0].Price
	}
	limits := types.RateLimits{}
	if len(prof.Models) > 0 {
		m := prof.Models[0]
		limits.HourlyTokens = m.HourlyTokens
		limits.DailyTokens = m.DailyTokens
		limits.TotalTokens = m.TotalTokens
	}
	return types.SellerNode{
		PeerID:      peerID,
		Skills:      skills,
		ModelName:   modelName,
		ModelType:   modelType,
		TuningTier:  tuning,
		Price:       price,
		Reputation:  1,
		OnDuty:      prof.OnDuty,
		TokenLimits: limits,
	}
}
