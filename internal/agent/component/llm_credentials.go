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
func resolveTenantLLMConfig(ctx context.Context, db *gorm.DB, driver, modelID, apiKey, baseURL, originalModelID string) (string, string) {
	if apiKey != "" || driver == "" || modelID == "" {
		return apiKey, baseURL
	}
	state, err := runtime.GetStateFromContext(ctx)
	if err != nil || state == nil {
		common.Debug("llm credentials: no canvas state in ctx")
		return apiKey, baseURL
	}
	tid, _ := state.GetSysString("tenant_id")
	if tid == "" {
		common.Debug("llm credentials: state.Sys has no tenant_id")
		return apiKey, baseURL
	}

	if originalModelID == "" {
		return apiKey, baseURL
	}
	if resolvedKey, resolvedBaseURL, ok := resolveTenantModelInstanceCredentials(ctx, db, tid, originalModelID, baseURL); ok {
		return resolvedKey, resolvedBaseURL
	}
	return apiKey, baseURL
}

func resolveChatModelRef(ctx context.Context, db *gorm.DB, modelID, driver, apiKey, baseURL string) (string, string, string, string, error) {
	return resolveModelRef(ctx, db, modelID, driver, apiKey, baseURL, entity.ModelTypeChat)
}

func resolveModelRef(ctx context.Context, db *gorm.DB, modelID, driver, apiKey, baseURL string, allowedTypes entity.ModelType) (string, string, string, string, error) {
	modelID, driver, apiKey, baseURL, _, err := resolveModelRefWithType(ctx, db, modelID, driver, apiKey, baseURL, allowedTypes)
	return modelID, driver, apiKey, baseURL, err
}

func resolveModelRefWithType(ctx context.Context, db *gorm.DB, modelID, driver, apiKey, baseURL string, allowedTypes entity.ModelType) (string, string, string, string, entity.ModelType, error) {
	originalModelID := modelID
	var modelType entity.ModelType
	if driver == "" && strings.Contains(modelID, "@") {
		resolvedModelID, resolvedDriver, resolvedAPIKey, resolvedBaseURL, resolvedType, ok, err := resolveTenantCompositeModel(ctx, db, modelID, apiKey, baseURL, allowedTypes)
		if err != nil {
			return "", "", "", "", 0, err
		}
		if ok {
			return resolvedModelID, resolvedDriver, resolvedAPIKey, resolvedBaseURL, resolvedType, nil
		}
	}
	if driver == "" && modelID != "" {
		if m, prov, ok := agentProviderLastSegmentSplit(modelID); ok {
			driver = prov
			modelID = m
		}
	}
	if driver == "" && modelID != "" {
		resolvedModelID, resolvedDriver, resolvedAPIKey, resolvedBaseURL, resolvedType, ok, err := resolveTenantModelByID(ctx, db, modelID, apiKey, baseURL, allowedTypes)
		if err != nil {
			return "", "", "", "", 0, err
		}
		if ok {
			modelID, driver, apiKey, baseURL, modelType = resolvedModelID, resolvedDriver, resolvedAPIKey, resolvedBaseURL, resolvedType
		}
	}
	if modelType == 0 && driver != "" && modelID != "" {
		if model, err := dao.GetModelProviderManager().GetModelByName(driver, modelID); err == nil {
			modelType = entity.ModelTypeFromStrings(model.ModelTypes)
		}
	}
	apiKey, baseURL = resolveTenantLLMConfig(ctx, db, driver, modelID, apiKey, baseURL, originalModelID)
	return modelID, driver, apiKey, baseURL, modelType, nil
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
	if _, images := collectSysFiles(state); len(images) > 0 {
		if supports, known := modelImageCapability(ctx, db, modelRef, modelName); known && !supports {
			return runtime.NewUserFacingError(fmt.Sprintf(`Image input is not supported by the selected model %q. Please select a vision-capable model.`, modelRef))
		}
	}
	return nil
}

// resolveTenantModelInstanceCredentials attempts to resolve llm credentials
// through tenant_model_provider + tenant_model_instance using the original
// composite llm_id (which still carries the instance name).
func resolveTenantModelInstanceCredentials(ctx context.Context, db *gorm.DB, tid, compositeLLMID, baseURL string) (string, string, bool) {
	modelName, instanceName, providerName := parseLLMIDParts(compositeLLMID)
	if instanceName == "" {
		common.Debug("llm credentials: new-table fallback skipped: no instance name", zap.String("composite_llm_id", compositeLLMID))
		return "", baseURL, false
	}

	common.Debug("llm credentials: new-table fallback",
		zap.String("tid", tid),
		zap.String("provider", providerName),
		zap.String("model", modelName),
		zap.String("instance", instanceName))

	provider, err := dao.NewTenantModelProviderDAO().GetByTenantIDAndProviderName(ctx, db, tid, providerName)
	if err != nil || provider == nil {
		common.Debug("llm credentials: new-table fallback: provider not found", zap.String("provider", providerName), zap.Error(err))
		return "", baseURL, false
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
		return "", baseURL, false
	}

	apiKey := instance.APIKey
	if !savedKeyMayGoTo(provider.ProviderName, instance, baseURL) {
		common.Debug("llm credentials: saved key not used with a base_url override of another origin", zap.String("provider", providerName))
		return "", baseURL, false
	}
	if instance.Extra != "" && baseURL == "" {
		var extra map[string]string
		if err := json.Unmarshal([]byte(instance.Extra), &extra); err == nil {
			if u := extra["base_url"]; u != "" {
				baseURL = u
			}
		}
	}

	common.Debug("llm credentials: new-table OK",
		zap.String("provider", providerName),
		zap.String("instance", instance.InstanceName),
		zap.Bool("api_key_present", apiKey != ""),
		zap.Bool("base_url_present", baseURL != ""))
	return apiKey, baseURL, apiKey != ""
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
func resolveTenantCompositeModel(ctx context.Context, db *gorm.DB, modelRef, apiKey, baseURL string, allowedTypes entity.ModelType) (string, string, string, string, entity.ModelType, bool, error) {
	unresolved := func() (string, string, string, string, entity.ModelType, bool, error) {
		return "", "", apiKey, baseURL, 0, false, nil
	}
	if db == nil || !allowedTypes.Has(entity.ModelTypeDecision) {
		return unresolved()
	}
	state, err := runtime.GetStateFromContext(ctx)
	if err != nil || state == nil {
		return unresolved()
	}
	tid, _ := state.GetSysString("tenant_id")
	modelName, instanceName, providerName := parseLLMIDParts(modelRef)
	if tid == "" || instanceName == "" {
		return unresolved()
	}
	explicitInstance := len(strings.Split(strings.TrimSpace(modelRef), "@")) >= 3
	// A three-part reference to a decision-only provider names an instance
	// explicitly. When it cannot be resolved, failing is safer than letting
	// the legacy model@provider split drop the instance and its credentials.
	notFound := func() (string, string, string, string, entity.ModelType, bool, error) {
		if explicitInstance && isDecisionOnlyDriver(providerName) {
			return "", "", apiKey, baseURL, 0, false, fmt.Errorf("tenant composite model %q not found", modelRef)
		}
		return unresolved()
	}
	provider, err := dao.NewTenantModelProviderDAO().GetByTenantIDAndProviderName(ctx, db, tid, providerName)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return notFound()
	}
	if err != nil {
		return "", "", apiKey, baseURL, 0, false, fmt.Errorf("resolve provider %q: %w", providerName, err)
	}
	instance, err := dao.NewTenantModelInstanceDAO().GetByProviderIDAndInstanceName(ctx, db, provider.ID, instanceName)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", apiKey, baseURL, 0, false, fmt.Errorf("resolve instance %q: %w", instanceName, err)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) && instanceName == "default" {
		instance = findSoleActiveProviderInstance(ctx, db, provider.ID)
	}
	if instance == nil {
		return notFound()
	}
	rows, err := dao.NewTenantModelDAO().GetModelsByProviderIDAndInstanceIDAndModelName(ctx, db, provider.ID, instance.ID, modelName)
	if err != nil {
		return "", "", apiKey, baseURL, 0, false, err
	}
	if len(rows) == 0 {
		// The model row is gone, but the caller falls back to the instance's
		// credentials. An explicit reference to a decision-only provider must
		// not reach an inactive instance through that fallback.
		if explicitInstance && isDecisionOnlyDriver(provider.ProviderName) && !strings.EqualFold(strings.TrimSpace(instance.Status), "active") {
			return "", "", apiKey, baseURL, 0, false, fmt.Errorf("tenant composite model %q: instance %q is not active", modelRef, instanceName)
		}
		return unresolved()
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
			return "", "", apiKey, baseURL, 0, false, fmt.Errorf("model %q has multiple active decision entries; use tenant_model.id", modelRef)
		}
		selected = row
	}
	if decisionRows == 0 {
		return unresolved()
	}
	if selected == nil {
		return "", "", apiKey, baseURL, 0, false, fmt.Errorf("model %q has no active decision entry", modelRef)
	}
	return resolveTenantModelByTenantModelID(ctx, db, tid, selected.ID, apiKey, baseURL, allowedTypes)
}

func resolveTenantModelByID(ctx context.Context, db *gorm.DB, modelRef, apiKey, baseURL string, allowedTypes entity.ModelType) (string, string, string, string, entity.ModelType, bool, error) {
	if !isBareTenantModelID(modelRef) {
		return "", "", apiKey, baseURL, 0, false, nil
	}
	state, err := runtime.GetStateFromContext(ctx)
	if err != nil || state == nil {
		return "", "", apiKey, baseURL, 0, false, nil
	}
	tid, _ := state.GetSysString("tenant_id")
	if tid == "" {
		return "", "", apiKey, baseURL, 0, false, nil
	}

	modelName, provider, modelKey, modelBaseURL, modelType, ok, err := resolveTenantModelByTenantModelID(ctx, db, tid, modelRef, apiKey, baseURL, allowedTypes)
	if err != nil {
		return "", "", apiKey, baseURL, 0, false, err
	}
	if ok {
		return modelName, provider, modelKey, modelBaseURL, modelType, true, nil
	}

	modelName, provider, instanceKey, instanceBaseURL, modelType, ok, err := resolveTenantModelByInstanceID(ctx, db, tid, modelRef, apiKey, baseURL, allowedTypes)
	if err != nil {
		return "", "", apiKey, baseURL, 0, false, err
	}
	if ok {
		return modelName, provider, instanceKey, instanceBaseURL, modelType, true, nil
	}
	return "", "", apiKey, baseURL, 0, false, fmt.Errorf("tenant %s model id %q not found", allowedTypes, modelRef)
}

func resolveTenantModelByTenantModelID(ctx context.Context, db *gorm.DB, tid, modelID, apiKey, baseURL string, allowedTypes entity.ModelType) (string, string, string, string, entity.ModelType, bool, error) {
	model, err := dao.NewTenantModelDAO().GetByID(ctx, db, modelID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", apiKey, baseURL, 0, false, nil
		}
		return "", "", apiKey, baseURL, 0, false, fmt.Errorf("resolve tenant model id %q: %w", modelID, err)
	}
	if !strings.EqualFold(strings.TrimSpace(model.Status), "active") {
		return "", "", apiKey, baseURL, 0, false, fmt.Errorf("tenant model id %s is disabled", modelID)
	}
	if !entity.ModelType(model.ModelType).Has(allowedTypes) {
		return "", "", apiKey, baseURL, 0, false, fmt.Errorf("tenant model id %s cannot be used as %s model", modelID, allowedTypes)
	}

	provider, err := dao.NewTenantModelProviderDAO().GetByID(ctx, db, model.ProviderID)
	if err != nil {
		return "", "", apiKey, baseURL, 0, false, fmt.Errorf("resolve provider for tenant model id %q: %w", modelID, err)
	}
	if provider.TenantID != tid {
		return "", "", apiKey, baseURL, 0, false, fmt.Errorf("tenant %s has no access to model id %s", tid, modelID)
	}

	instance, err := dao.NewTenantModelInstanceDAO().GetByID(ctx, db, model.InstanceID)
	if err != nil {
		return "", "", apiKey, baseURL, 0, false, fmt.Errorf("resolve instance for tenant model id %q: %w", modelID, err)
	}
	if entity.ModelType(model.ModelType).Has(entity.ModelTypeDecision) && allowedTypes.Has(entity.ModelTypeDecision) {
		if instance.ProviderID != provider.ID || !strings.EqualFold(strings.TrimSpace(instance.Status), "active") {
			return "", "", apiKey, baseURL, 0, false, fmt.Errorf("tenant model id %s has an inactive or mismatched instance", modelID)
		}
	}
	apiKey, baseURL = fillInstanceCredentials(provider.ProviderName, instance, apiKey, baseURL)
	return model.ModelName, provider.ProviderName, apiKey, baseURL, entity.ModelType(model.ModelType), true, nil
}

func resolveTenantModelByInstanceID(ctx context.Context, db *gorm.DB, tid, instanceID, apiKey, baseURL string, allowedTypes entity.ModelType) (string, string, string, string, entity.ModelType, bool, error) {
	instance, err := dao.NewTenantModelInstanceDAO().GetByID(ctx, db, instanceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", apiKey, baseURL, 0, false, nil
		}
		return "", "", apiKey, baseURL, 0, false, fmt.Errorf("resolve tenant model instance id %q: %w", instanceID, err)
	}
	provider, err := dao.NewTenantModelProviderDAO().GetByID(ctx, db, instance.ProviderID)
	if err != nil {
		return "", "", apiKey, baseURL, 0, false, fmt.Errorf("resolve provider for tenant model instance id %q: %w", instanceID, err)
	}
	if provider.TenantID != tid {
		return "", "", apiKey, baseURL, 0, false, fmt.Errorf("tenant %s has no access to model instance id %s", tid, instanceID)
	}

	models, err := dao.NewTenantModelDAO().GetModelsByInstanceID(ctx, db, instance.ID)
	if err != nil {
		return "", "", apiKey, baseURL, 0, false, fmt.Errorf("resolve models for tenant model instance id %q: %w", instanceID, err)
	}
	candidates := make([]*entity.TenantModel, 0, len(models))
	for _, model := range models {
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
		return "", "", apiKey, baseURL, 0, false, fmt.Errorf("tenant model instance id %s has no active %s model", instanceID, allowedTypes)
	}
	if len(candidates) > 1 {
		return "", "", apiKey, baseURL, 0, false, fmt.Errorf("tenant model instance id %s has %d active %s models; use tenant_model.id or model@instance@provider", instanceID, len(candidates), allowedTypes)
	}
	if entity.ModelType(candidates[0].ModelType).Has(entity.ModelTypeDecision) {
		if !strings.EqualFold(strings.TrimSpace(instance.Status), "active") {
			return "", "", apiKey, baseURL, 0, false, fmt.Errorf("decision model instance id %s requires an active instance", instanceID)
		}
	}
	apiKey, baseURL = fillInstanceCredentials(provider.ProviderName, instance, apiKey, baseURL)
	return candidates[0].ModelName, provider.ProviderName, apiKey, baseURL, entity.ModelType(candidates[0].ModelType), true, nil
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
