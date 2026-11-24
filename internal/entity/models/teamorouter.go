package models

type TeamoRouterModel struct {
	*OpenAIAPICompatibleModel
}

func NewTeamoRouterModel(baseURL map[string]string, urlSuffix URLSuffix) *TeamoRouterModel {
	return &TeamoRouterModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

func (m *TeamoRouterModel) Name() string {
	return "teamorouter"
}

func (m *TeamoRouterModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewTeamoRouterModel(baseURL, m.baseModel.URLSuffix)
}
