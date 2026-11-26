package models

type StraitlyModel struct {
	*OpenAIAPICompatibleModel
}

func NewStraitlyModel(baseURL map[string]string, urlSuffix URLSuffix) *StraitlyModel {
	return &StraitlyModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

func (m *StraitlyModel) Name() string {
	return "straitly"
}

func (m *StraitlyModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewStraitlyModel(baseURL, m.baseModel.URLSuffix)
}
