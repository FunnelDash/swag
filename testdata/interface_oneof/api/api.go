package api

// RewardRule is one way a customer earns a reward.
// @discriminator rewardsType
type RewardRule interface {
	isRewardRule()
}

// QualifiedSpendRule pays a percentage of qualified spend.
// @oneOfMember RewardRule
// @discriminatorValue qualified-spend-percentage
type QualifiedSpendRule struct {
	RewardsType string  `json:"rewardsType"`
	Percentage  float64 `json:"qualifiedSpendPercentage"`
}

func (QualifiedSpendRule) isRewardRule() {}

// FlatAmountRule pays a one-off amount at a threshold.
// @oneOfMember RewardRule
// @discriminatorValue flat-amount
type FlatAmountRule struct {
	RewardsType string  `json:"rewardsType"`
	Amount      float64 `json:"flatAmount"`
	WindowDays  int     `json:"flatAmountWindowDays"`
}

func (FlatAmountRule) isRewardRule() {}

// Untagged has no oneOf declaration, so it must stay an empty schema.
type Untagged interface {
	isUntagged()
}

type Contract struct {
	LegalName string       `json:"legalName"`
	Rules     []RewardRule `json:"rules"`
	Anything  Untagged     `json:"anything"`
}

// GetContract godoc
// @Summary Get a contract
// @Produce json
// @Success 200 {object} api.Contract
// @Router /contract [get]
func GetContract() {}
