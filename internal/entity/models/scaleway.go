package models

type ScalewayModel struct {
	*OpenAIAPICompatibleModel
}

func NewScalewayModel(baseURL map[string]string, urlSuffix URLSuffix) *ScalewayModel {
	return &ScalewayModel{
		OpenAIAPICompatibleModel: NewOpenAIAPICompatibleModel(baseURL, urlSuffix),
	}
}

func (m *ScalewayModel) Name() string {
	return "scaleway"
}

func (m *ScalewayModel) NewInstance(baseURL map[string]string) ModelDriver {
	return NewScalewayModel(baseURL, m.baseModel.URLSuffix)
}
