package models

type SarvamModel struct {
	*OpenAIAPICompatibleModel
}

func NewSarvamModel(baseURL map[string]string, urlSuffix URLSuffix) *SarvamModel {
	return &SarvamModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

func (m *SarvamModel) Name() string {
	return "sarvam"
}

func (m *SarvamModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewSarvamModel(baseURL, m.baseModel.URLSuffix)
}
