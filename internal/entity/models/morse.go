package models

type MorseModel struct {
	*OpenAIAPICompatibleModel
}

func NewMorseModel(baseURL map[string]string, urlSuffix URLSuffix) *MorseModel {
	return &MorseModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

func (m *MorseModel) Name() string {
	return "morse"
}

func (m *MorseModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewMorseModel(baseURL, m.baseModel.URLSuffix)
}
