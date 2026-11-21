package models

// AIGROWModel drives AIGROW's OpenAI-compatible endpoint. Every model it
// lists answers on /chat/completions, so the generic driver serves it and
// ListModels discovers the catalog for the tenant's key.
type AIGROWModel struct {
	*OpenAIAPICompatibleModel
}

func NewAIGROWModel(baseURL map[string]string, urlSuffix URLSuffix) *AIGROWModel {
	return &AIGROWModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

func (m *AIGROWModel) Name() string {
	return "aigrow"
}

func (m *AIGROWModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewAIGROWModel(baseURL, m.baseModel.URLSuffix)
}
