package seller

import (
	"github.com/knowledgemeshgrid/knowledgemesh/internal/control"
	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

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
		Ollama:      prof.Ollama,
	}
}
