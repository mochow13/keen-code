package config

const (
	RouterCategorySimple   = "simple"
	RouterCategoryStandard = "standard"
	RouterCategoryComplex  = "complex"
)

type RouterModel struct {
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	ThinkingEffort string `json:"thinking_effort,omitempty"`
}

type RouterConfig struct {
	Enabled  bool        `json:"enabled"`
	Simple   RouterModel `json:"simple"`
	Standard RouterModel `json:"standard"`
	Complex  RouterModel `json:"complex"`
}

func (r *RouterConfig) Configured() bool {
	return r != nil && r.Simple.Provider != "" && r.Simple.Model != "" && r.Standard.Provider != "" && r.Standard.Model != "" && r.Complex.Provider != "" && r.Complex.Model != ""
}

func (r *RouterConfig) ModelForCategory(category string) RouterModel {
	switch category {
	case RouterCategorySimple:
		return r.Simple
	case RouterCategoryComplex:
		return r.Complex
	default:
		return r.Standard
	}
}
