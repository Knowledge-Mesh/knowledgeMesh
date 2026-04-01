package matchmaker

import (
	"errors"
	"sort"
	"strings"

	"github.com/knowledgemeshgrid/knowledgemesh/pkg/types"
)

var ErrNoSellerMatch = errors.New("no matching seller found")

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Match(req types.InferenceRequest, sellers []types.SellerNode) (types.SellerNode, error) {
	candidates := make([]types.SellerNode, 0, len(sellers))
	for _, seller := range sellers {
		if !seller.OnDuty {
			continue
		}
		if req.MaxPrice > 0 && seller.Price > req.MaxPrice {
			continue
		}
		if !hasSkill(seller, req.Skill.Name) {
			continue
		}
		candidates = append(candidates, seller)
	}

	if len(candidates) == 0 {
		return types.SellerNode{}, ErrNoSellerMatch
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Price != candidates[j].Price {
			return candidates[i].Price < candidates[j].Price
		}
		return candidates[i].Reputation > candidates[j].Reputation
	})

	return candidates[0], nil
}

func hasSkill(seller types.SellerNode, wanted string) bool {
	for _, skill := range seller.Skills {
		if strings.EqualFold(strings.TrimSpace(skill.Name), strings.TrimSpace(wanted)) {
			return true
		}
	}
	return false
}
