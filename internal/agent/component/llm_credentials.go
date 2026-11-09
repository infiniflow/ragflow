package component

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// resolveTenantLLMConfig fills tenant-scoped API credentials for the supplied
// driver/model pair when the canvas DSL omitted them. It resolves them through
// tenant_model_provider + tenant_model_instance using the composite llm_id.
func resolveTenantLLMConfig(ctx context.Context, db *gorm.DB, driver, modelID, apiKey, baseURL, originalModelID string) (string, string, string, *models.ModelInfo) {
	if apiKey != "" || originalModelID == "" || driver == "" || modelID == "" {
		return apiKey, baseURL, "", nil
	}
	state, err := runtime.GetStateFromContext(ctx)
	if err != nil || state == nil {
		common.Debug("llm credentials: no canvas state in ctx")
		return apiKey, baseURL, "", nil
	}
	tid, _ := state.GetSysString("tenant_id")
	if tid == "" {
		common.Debug("llm credentials: state.Sys has no tenant_id")
		return apiKey, baseURL, "", nil
	}

	if resolvedKey, resolvedBaseURL, resolvedInst, info, ok := resolveTenantModelInstanceCredentials(ctx, db, tid, originalModelID, baseURL); ok {
		if apiKey == "" {
			apiKey = resolvedKey
		}
		if baseURL == "" {
			baseURL = resolvedBaseURL
		}
		return apiKey, baseURL, resolvedInst, info
	}
	return apiKey, baseURL, "", nil
}

type resolvedModelRef struct {
	ModelName  string
	Driver     string
	APIKey     string
	BaseURL    string
	InstanceID string
	ModelType  entity.ModelType
	ModelInfo  *models.ModelInfo
}

func resolveChatModelRef(ctx context.Context, db *gorm.DB, modelID, driver, apiKey, baseURL string) (string, string, string, string, error) {
	ref, err := resolveModelRef(ctx, db, modelID, driver, apiKey, baseURL, entity.ModelTypeChat)
	if err != nil {
		return "", "", "", "", err
	}
	return ref.ModelName, ref.Driver, ref.APIKey, ref.BaseURL, nil
}

func resolveChatModelRefWithInfo(ctx context.Context, db *gorm.DB, modelID, driver, apiKey, baseURL string) (string, string, string, string, *models.ModelInfo, error) {
	ref, err := resolveModelRef(ctx, db, modelID, driver, apiKey, baseURL, entity.ModelTypeChat)
	if err != nil {
		return "", "", "", "", nil, err
	}
	return ref.ModelName, ref.Driver, ref.APIKey, ref.BaseURL, ref.ModelInfo, nil
}

func resolveModelRefWithType(ctx context.Context, db *gorm.DB, modelID, driver, apiKey, baseURL string, allowedTypes entity.ModelType) (string, string, string, string, entity.ModelType, error) {
	ref, err := resolveModelRef(ctx, db, modelID, driver, apiKey, baseURL, allowedTypes)
	if err != nil {
		return "", "", "", "", 0, err
	}
	return ref.ModelName, ref.Driver, ref.APIKey, ref.BaseURL, ref.ModelType, nil
}

func resolveModelRefWithInstance(ctx context.Context, db *gorm.DB, modelID, driver, apiKey, baseURL string, allowedTypes entity.ModelType) (string, string, string, string, string, entity.ModelType, error) {
	ref, err := resolveModelRef(ctx, db, modelID, driver, apiKey, baseURL, allowedTypes)
	if err != nil {
		return "", "", "", "", "", 0, err
	}
	return ref.ModelName, ref.Driver, ref.APIKey, ref.BaseURL, ref.InstanceID, ref.ModelType, nil
}

func resolveModelRef(ctx context.Context, db *gorm.DB, modelID, driver, apiKey, baseURL string, allowedTypes entity.ModelType) (*resolvedModelRef, error) {
	originalModelID := modelID
	var res *resolvedModelRef
	if driver == "" && strings.Contains(modelID, "@") {
		resolved, ok, err := resolveTenantCompositeModel(ctx, db, modelID, apiKey, baseURL, allowedTypes)
		if err != nil {
			return nil, err
		}
		if ok {
			return resolved, nil
		}
	}
	if driver == "" && modelID != "" {
		if m, prov, ok := agentProviderLastSegmentSplit(modelID); ok {
			driver = prov
			modelID = m
		}
	}
	if driver == "" && modelID != "" {
		resolved, ok, err := resolveTenantModelByID(ctx, db, modelID, apiKey, baseURL, allowedTypes)
		if err != nil {
			return nil, err
		}
		if ok {
			res = resolved
			modelID = res.ModelName
			driver = res.Driver
			apiKey = res.APIKey
			baseURL = res.BaseURL
		}
	}
	if res == nil {
		res = &resolvedModelRef{
			ModelName: modelID,
			Driver:    driver,
			APIKey:    apiKey,
			BaseURL:   baseURL,
		}
	}
	if res.ModelType == 0 && driver != "" && modelID != "" {
		if model, err := dao.GetModelProviderManager().GetModelByName(driver, modelID); err == nil {
			res.ModelType = entity.ModelTypeFromStrings(model.ModelTypes)
		}
	}
	resolvedKey, resolvedBaseURL, resolvedInst, fallbackInfo := resolveTenantLLMConfig(ctx, db, driver, modelID, apiKey, baseURL, originalModelID)
	res.APIKey = resolvedKey
	res.BaseURL = resolvedBaseURL
	if res.InstanceID == "" {
		res.InstanceID = resolvedInst
	}
	if res.ModelInfo == nil {
		res.ModelInfo = fallbackInfo
	}
	return res, nil
}

// modelImageCapability reports an explicitly known image capability. The
// second result is false when the model is unknown, so local/test drivers keep
// their historical multimodal behavior.
func modelImageCapability(ctx context.Context, db *gorm.DB, modelRef, modelName string) (supports, known bool) {
	if db != nil && modelRef != "" {
		if row, err := dao.NewTenantModelDAO().GetByID(ctx, db, modelRef); err == nil && row != nil {
			return entity.ModelType(row.ModelType).Has(entity.ModelTypeImage2Text), true
		}
	}
	if modelName == "" {
		return false, false
	}
	model := dao.GetModelProviderManager().GetModelByNameOrAlias(modelName)
	if model == nil || len(model.ModelTypes) == 0 {
		return false, false
	}
	for _, typ := range model.ModelTypes {
		if strings.EqualFold(typ, "vision") || strings.EqualFold(typ, "image2text") {
			return true, true
		}
	}
	return false, true
}

func rejectUnsupportedImages(ctx context.Context, db *gorm.DB, state *runtime.CanvasState, modelRef, modelName string) error {
	if state == nil {
		return nil
	}
	_, images := collectSysFiles(state)
	return rejectUnsupportedImageFiles(ctx, db, images, modelRef, modelName)
}

func rejectUnsupportedImageFiles(ctx context.Context, db *gorm.DB, images []string, modelRef, modelName string) error {
	if len(images) == 0 {
		return nil
	}
	if supports, known := modelImageCapability(ctx, db, modelRef, modelName); known && !supports {
		return runtime.NewUserFacingError(fmt.Sprintf(`Image input is not supported by the selected model %q. Please select a vision-capable model.`, modelRef))
	}
	return nil
}

// resolveTenantModelInstanceCredentials attempts to resolve llm credentials
// through tenant_model_provider + tenant_model_instance using the original
// composite llm_id (which still carries the instance name).
func resolveTenantModelInstanceCredentials(ctx context.Context, db *gorm.DB, tid, compositeLLMID, baseURL string) (string, string, string, *models.ModelInfo, bool) {
	modelName, instanceName, providerName := parseLLMIDParts(compositeLLMID)
	if instanceName == "" {
		common.Debug("llm credentials: new-table fallback skipped: no instance name", zap.String("composite_llm_id", compositeLLMID))
		return "", baseURL, "", nil, false
	}

	common.Debug("llm credentials: new-table fallback",
		zap.String("tid", tid),
		zap.String("provider", providerName),
		zap.String("model", modelName),
		zap.String("instance", instanceName))

	owner, ownerErr := dao.NewUserTenantDAO().GetOwnerByTenantID(ctx, db, tid)
	if ownerErr != nil {
		common.Debug("llm credentials: new-table fallback: tenant owner lookup failed", zap.String("tenant", tid), zap.Error(ownerErr))
		return "", baseURL, "", nil, false
	}
	ownerUserID := owner.UserID
	provider, err := dao.NewTenantModelProviderDAO().GetByTenantIDAndUserIDAndProviderName(ctx, db, tid, ownerUserID, providerName)
	if err != nil || provider == nil {
		common.Debug("llm credentials: new-table fallback: provider not found", zap.String("provider", providerName), zap.Error(err))
		return "", baseURL, "", nil, false
	}

	instance, err := dao.NewTenantModelInstanceDAO().GetByProviderIDAndInstanceName(ctx, db, provider.ID, instanceName)
	if err != nil || instance == nil {
		if instanceName == "default" {
			if fallback := findSoleActiveProviderInstance(ctx, db, provider.ID); fallback != nil {
				common.Debug("llm credentials: new-table fallback: remapped default instance to sole active instance",
					zap.String("instance", fallback.InstanceName),
					zap.String("provider", providerName))
				instance = fallback
				err = nil
			}
		}
	}
	if err != nil || instance == nil {
		common.Debug("llm credentials: new-table fallback: instance not found",
			zap.String("instance", instanceName),
			zap.String("provider", providerName),
			zap.Error(err))
		return "", baseURL, "", nil, false
	}

	apiKey := instance.APIKey
	if !savedKeyMayGoTo(provider.ProviderName, instance, baseURL) {
		common.Debug("llm credentials: saved key not used with a base_url override of another origin", zap.String("provider", providerName))
		return "", baseURL, "", nil, false
	}
	if instance.Extra != "" && baseURL == "" {
		var extra map[string]string
		if err := json.Unmarshal([]byte(instance.Extra), &extra); err == nil {
			if u := extra["base_url"]; u != "" {
				baseURL = u
			}
		}
	}

	info := &models.ModelInfo{
		Name:          modelName,
		ProviderName:  provider.ProviderName,
		InstanceID:    instance.ID,
		InstanceName:  instance.InstanceName,
		TenantID:      provider.TenantID,
		IsSavedConfig: true,
	}

	common.Debug("llm credentials: new-table OK",
		zap.String("provider", providerName),
		zap.String("instance", instance.InstanceName),
		zap.Bool("api_key_present", apiKey != ""),
		zap.Bool("base_url_present", baseURL != ""))
	return apiKey, baseURL, instance.ID, info, true
}

func findSoleActiveProviderInstance(ctx context.Context, db *gorm.DB, providerID string) *entity.TenantModelInstance {
	instances, err := dao.NewTenantModelInstanceDAO().GetAllInstancesByProviderID(ctx, db, providerID)
	if err != nil {
		common.Debug("llm credentials: list provider instances", zap.Error(err))
		return nil
	}
	active := make([]*entity.TenantModelInstance, 0, len(instances))
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(inst.Status), "inactive") {
			continue
		}
		active = append(active, inst)
	}
	if len(active) != 1 {
		return nil
	}
	return active[0]
}

// isDecisionOnlyDriver reports drivers that serve decision models only.
func isDecisionOnlyDriver(name string) bool {
	return strings.EqualFold(name, "SystemOne") || strings.EqualFold(name, "Nace")
}

// resolveTenantCompositeModel resolves a model@instance@provider (or
// model@provider) reference to a tenant decision model. It only acts when the
// caller accepts decision models and the reference names one; everything else
// (chat, vision, disabled rows, unknown providers) returns ok=false so the
// pre-existing resolution runs unchanged.
func resolveTenantCompositeModel(ctx context.Context, db *gorm.DB, modelRef, apiKey, baseURL string, allowedTypes entity.ModelType) (*resolvedModelRef, bool, error) {
	fallback := &resolvedModelRef{APIKey: apiKey, BaseURL: baseURL}
	if db == nil || !allowedTypes.Has(entity.ModelTypeDecision) {
		return fallback, false, nil
	}
	state, err := runtime.GetStateFromContext(ctx)
	if err != nil || state == nil {
		return fallback, false, nil
	}
	tid, _ := state.GetSysString("tenant_id")
	modelName, instanceName, providerName := parseLLMIDParts(modelRef)
	if tid == "" || instanceName == "" {
		return fallback, false, nil
	}
	explicitInstance := len(strings.Split(strings.TrimSpace(modelRef), "@")) >= 3
	// A three-part reference to a decision-only provider names an instance
	// explicitly. When it cannot be resolved, failing is safer than letting
	// the legacy model@provider split drop the instance and its credentials.
	notFound := func() (*resolvedModelRef, bool, error) {
		if explicitInstance && isDecisionOnlyDriver(providerName) {
			return nil, false, fmt.Errorf("tenant composite model %q not found", modelRef)
		}
		return fallback, false, nil
	}
	owner, err := dao.NewUserTenantDAO().GetOwnerByTenantID(ctx, db, tid)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound()
	}
	if err != nil {
		return nil, false, fmt.Errorf("resolve tenant owner %q: %w", tid, err)
	}
	provider, err := dao.NewTenantModelProviderDAO().GetByTenantIDAndUserIDAndProviderName(ctx, db, tid, owner.UserID, providerName)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound()
	}
	if err != nil {
		return nil, false, fmt.Errorf("resolve provider %q: %w", providerName, err)
	}
	instance, err := dao.NewTenantModelInstanceDAO().GetByProviderIDAndInstanceName(ctx, db, provider.ID, instanceName)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, fmt.Errorf("resolve instance %q: %w", instanceName, err)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) && instanceName == "default" {
		instance = findSoleActiveProviderInstance(ctx, db, provider.ID)
	}
	if instance == nil {
		return notFound()
	}
	rows, err := dao.NewTenantModelDAO().GetModelsByProviderIDAndInstanceIDAndModelName(ctx, db, provider.ID, instance.ID, modelName)
	if err != nil {
		return nil, false, err
	}
	if len(rows) == 0 {
		// The model row is gone, but the caller falls back to the instance's
		// credentials. An explicit reference to a decision-only provider must
		// not reach an inactive instance through that fallback.
		if explicitInstance && isDecisionOnlyDriver(provider.ProviderName) && !strings.EqualFold(strings.TrimSpace(instance.Status), "active") {
			return nil, false, fmt.Errorf("tenant composite model %q: instance %q is not active", modelRef, instanceName)
		}
		return fallback, false, nil
	}
	var selected *entity.TenantModel
	decisionRows := 0
	for _, row := range rows {
		if !entity.ModelType(row.ModelType).Has(entity.ModelTypeDecision) {
			continue
		}
		decisionRows++
		if !strings.EqualFold(strings.TrimSpace(row.Status), "active") {
			continue
		}
		if selected != nil {
			return nil, false, fmt.Errorf("model %q has multiple active decision entries; use tenant_model.id", modelRef)
		}
		selected = row
	}
	if decisionRows == 0 {
		return fallback, false, nil
	}
	if selected == nil {
		return nil, false, fmt.Errorf("model %q has no active decision entry", modelRef)
	}
	return resolveTenantModelByTenantModelID(ctx, db, tid, selected.ID, apiKey, baseURL, allowedTypes)
}

func resolveTenantModelByID(ctx context.Context, db *gorm.DB, modelRef, apiKey, baseURL string, allowedTypes entity.ModelType) (*resolvedModelRef, bool, error) {
	if !isBareTenantModelID(modelRef) {
		return nil, false, nil
	}
	state, err := runtime.GetStateFromContext(ctx)
	if err != nil || state == nil {
		return nil, false, nil
	}
	tid, _ := state.GetSysString("tenant_id")
	if tid == "" {
		return nil, false, nil
	}

	res, ok, err := resolveTenantModelByTenantModelID(ctx, db, tid, modelRef, apiKey, baseURL, allowedTypes)
	if err != nil || ok {
		return res, ok, err
	}

	res, ok, err = resolveTenantModelByInstanceID(ctx, db, tid, modelRef, apiKey, baseURL, allowedTypes)
	if err != nil || ok {
		return res, ok, err
	}
	return nil, false, fmt.Errorf("tenant %s model id %q not found", allowedTypes, modelRef)
}

func resolveTenantModelByTenantModelID(ctx context.Context, db *gorm.DB, tid, modelID, apiKey, baseURL string, allowedTypes entity.ModelType) (*resolvedModelRef, bool, error) {
	model, err := dao.NewTenantModelDAO().GetByID(ctx, db, modelID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("resolve tenant model id %q: %w", modelID, err)
	}
	if !strings.EqualFold(strings.TrimSpace(model.Status), "active") {
		return nil, false, fmt.Errorf("tenant model id %s is disabled", modelID)
	}
	if !entity.ModelType(model.ModelType).Has(allowedTypes) {
		return nil, false, fmt.Errorf("tenant model id %s cannot be used as %s model", modelID, allowedTypes)
	}

	provider, err := dao.NewTenantModelProviderDAO().GetByID(ctx, db, model.ProviderID)
	if err != nil {
		return nil, false, fmt.Errorf("resolve provider for tenant model id %q: %w", modelID, err)
	}
	if provider.TenantID != tid {
		return nil, false, fmt.Errorf("tenant %s has no access to model id %s", tid, modelID)
	}

	instance, err := dao.NewTenantModelInstanceDAO().GetByID(ctx, db, model.InstanceID)
	if err != nil {
		return nil, false, fmt.Errorf("resolve instance for tenant model id %q: %w", modelID, err)
	}
	if entity.ModelType(model.ModelType).Has(entity.ModelTypeDecision) && allowedTypes.Has(entity.ModelTypeDecision) {
		if instance.ProviderID != provider.ID || !strings.EqualFold(strings.TrimSpace(instance.Status), "active") {
			return nil, false, fmt.Errorf("tenant model id %s has an inactive or mismatched instance", modelID)
		}
	}
	rawAPIKey := apiKey
	apiKey, baseURL = fillInstanceCredentials(provider.ProviderName, instance, apiKey, baseURL)
	var info *models.ModelInfo
	var resolvedInstanceID string
	if rawAPIKey == "" || rawAPIKey == instance.APIKey {
		info = &models.ModelInfo{
			ID:            model.ID,
			Name:          model.ModelName,
			ProviderName:  provider.ProviderName,
			InstanceID:    instance.ID,
			InstanceName:  instance.InstanceName,
			TenantID:      provider.TenantID,
			IsSavedConfig: true,
		}
		resolvedInstanceID = instance.ID
	}
	return &resolvedModelRef{
		ModelName:  model.ModelName,
		Driver:     provider.ProviderName,
		APIKey:     apiKey,
		BaseURL:    baseURL,
		InstanceID: resolvedInstanceID,
		ModelType:  entity.ModelType(model.ModelType),
		ModelInfo:  info,
	}, true, nil
}

func resolveTenantModelByInstanceID(ctx context.Context, db *gorm.DB, tid, instanceID, apiKey, baseURL string, allowedTypes entity.ModelType) (*resolvedModelRef, bool, error) {
	instance, err := dao.NewTenantModelInstanceDAO().GetByID(ctx, db, instanceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("resolve tenant model instance id %q: %w", instanceID, err)
	}
	provider, err := dao.NewTenantModelProviderDAO().GetByID(ctx, db, instance.ProviderID)
	if err != nil {
		return nil, false, fmt.Errorf("resolve provider for tenant model instance id %q: %w", instanceID, err)
	}
	if provider.TenantID != tid {
		return nil, false, fmt.Errorf("tenant %s has no access to model instance id %s", tid, instanceID)
	}

	modelList, err := dao.NewTenantModelDAO().GetModelsByInstanceID(ctx, db, instance.ID)
	if err != nil {
		return nil, false, fmt.Errorf("resolve models for tenant model instance id %q: %w", instanceID, err)
	}
	candidates := make([]*entity.TenantModel, 0, len(modelList))
	for _, model := range modelList {
		if model == nil || model.ProviderID != provider.ID {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(model.Status), "active") {
			continue
		}
		if !entity.ModelType(model.ModelType).Has(allowedTypes) {
			continue
		}
		candidates = append(candidates, model)
	}
	if len(candidates) == 0 {
		return nil, false, fmt.Errorf("tenant model instance id %s has no active %s model", instanceID, allowedTypes)
	}
	if len(candidates) > 1 {
		return nil, false, fmt.Errorf("tenant model instance id %s has %d active %s models; use tenant_model.id or model@instance@provider", instanceID, len(candidates), allowedTypes)
	}
	if entity.ModelType(candidates[0].ModelType).Has(entity.ModelTypeDecision) {
		if !strings.EqualFold(strings.TrimSpace(instance.Status), "active") {
			return nil, false, fmt.Errorf("decision model instance id %s requires an active instance", instanceID)
		}
	}
	rawAPIKey := apiKey
	apiKey, baseURL = fillInstanceCredentials(provider.ProviderName, instance, apiKey, baseURL)
	var info *models.ModelInfo
	var resolvedInstanceID string
	if rawAPIKey == "" || rawAPIKey == instance.APIKey {
		info = &models.ModelInfo{
			ID:            candidates[0].ID,
			Name:          candidates[0].ModelName,
			ProviderName:  provider.ProviderName,
			InstanceID:    instance.ID,
			InstanceName:  instance.InstanceName,
			TenantID:      provider.TenantID,
			IsSavedConfig: true,
		}
		resolvedInstanceID = instance.ID
	}
	return &resolvedModelRef{
		ModelName:  candidates[0].ModelName,
		Driver:     provider.ProviderName,
		APIKey:     apiKey,
		BaseURL:    baseURL,
		InstanceID: resolvedInstanceID,
		ModelType:  entity.ModelType(candidates[0].ModelType),
		ModelInfo:  info,
	}, true, nil
}

// naceDefaultOrigin is where a saved Nace instance points when it stores no
// base_url of its own.
const naceDefaultOrigin = "https://console.nace.ai"

// instanceBaseURL returns the base_url saved on the instance, if any.
func instanceBaseURL(instance *entity.TenantModelInstance) string {
	if instance == nil || instance.Extra == "" {
		return ""
	}
	var extra map[string]string
	if err := json.Unmarshal([]byte(instance.Extra), &extra); err != nil {
		return ""
	}
	return extra["base_url"]
}

// savedKeyMayGoTo reports whether the instance's saved key may be sent to the
// override base URL. A workflow can supply base_url without an api_key; for
// Nace the saved key is then only combined with an override that has the same
// origin as the saved instance, so a workflow cannot send it elsewhere.
func savedKeyMayGoTo(providerName string, instance *entity.TenantModelInstance, override string) bool {
	if !strings.EqualFold(providerName, "Nace") || strings.TrimSpace(override) == "" {
		return true
	}
	saved := instanceBaseURL(instance)
	if saved == "" {
		saved = naceDefaultOrigin
	}
	return models.SameOrigin(saved, override)
}

func fillInstanceCredentials(providerName string, instance *entity.TenantModelInstance, apiKey, baseURL string) (string, string) {
	if instance == nil {
		return apiKey, baseURL
	}
	if apiKey == "" && savedKeyMayGoTo(providerName, instance, baseURL) {
		apiKey = instance.APIKey
	}
	if baseURL == "" && instance.Extra != "" {
		var extra map[string]string
		if err := json.Unmarshal([]byte(instance.Extra), &extra); err == nil {
			baseURL = extra["base_url"]
		}
	}
	return apiKey, baseURL
}

func isBareTenantModelID(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) != 32 || strings.Contains(s, "@") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// parseLLMIDParts splits a composite llm_id into model, instance, and
// provider segments. The split is right-anchored — provider is the last
// segment, instance the second-to-last, and everything left of them is the
// model name, which may itself contain '@' (e.g. LM Studio quant-suffixed
// ids). Mirrors Python's split_model_name (rsplit("@", 2)).
//
//	"model@provider"          -> ("model", "default", "provider")
//	"model@instance@provider" -> ("model", "instance", "provider")
//	"model@tag@inst@provider" -> ("model@tag", "inst", "provider")
func parseLLMIDParts(s string) (modelName, instanceName, providerName string) {
	parts := strings.Split(strings.TrimSpace(s), "@")
	switch len(parts) {
	case 2:
		return parts[0], "default", parts[1]
	default:
		if len(parts) >= 3 {
			return strings.Join(parts[:len(parts)-2], "@"), parts[len(parts)-2], parts[len(parts)-1]
		}
		return s, "", ""
	}
}
