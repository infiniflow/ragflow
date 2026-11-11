import { isProviderCredentialField } from '@/components/model-provider-schema/field-config/utils';
import type { ProviderConfig } from '@/components/model-provider-schema/types';

/** Form fields that belong to models; the admin page manages models apart. */
export const ModelOwnedFieldNames = new Set([
  'instance_name',
  'model_name',
  'model_type',
  'max_tokens',
  'is_tools',
  'vision',
]);

/**
 * Seed the instance form from the safe detail response. Stored credential
 * values are never returned, so credential fields always start empty and a
 * blank value keeps what is saved.
 */
export function buildInstanceFormValues(
  config: ProviderConfig,
  detail: AdminService.PlatformInstanceDetail,
): Record<string, unknown> {
  const safeConfig = detail.api_key_config ?? {};
  const values: Record<string, unknown> = { ...safeConfig };
  if (detail.base_url) {
    values.base_url = detail.base_url;
  }
  if (config.echoTransform) {
    Object.assign(
      values,
      config.echoTransform({
        ...detail,
        ...safeConfig,
        api_key_config: safeConfig,
      }),
    );
  }
  for (const [field, configured] of Object.entries(
    detail.credential_status ?? {},
  )) {
    if (configured) values[field] = '';
  }
  for (const field of Object.keys(values)) {
    if (isProviderCredentialField(field) && !safeConfig[field]) {
      values[field] = '';
    }
  }
  return values;
}

/** Form fields that hold a service address. */
const AddressFieldNames = new Set([
  'base_url',
  'mineru_apiserver',
  'mineru_server_url',
  'paddleocr_api_url',
  'opendataloader_apiserver',
]);

/**
 * The address field that the provider saves as the instance base_url, found
 * by passing a marker value through its own submit transform.
 */
export function baseUrlFieldOf(config: ProviderConfig): string | undefined {
  const marker = 'https://platform-base-url.invalid';
  return config.fields
    .map((field) => field.name)
    .find((name) => {
      if (!AddressFieldNames.has(name)) return false;
      const values = { [name]: marker };
      const baseUrl = config.submitTransform
        ? config.submitTransform(values).base_url
        : values.base_url;
      return baseUrl === marker;
    });
}

/**
 * Whether a saved address is withheld from the form because it may carry
 * credentials (user info, query or fragment). Leaving it blank keeps it.
 * `baseUrlField` is the field saved as the instance base_url; other address
 * fields live in the safe configuration.
 */
export function isHiddenSavedAddress(
  detail: AdminService.PlatformInstanceDetail,
  field: string,
  baseUrlField: string | undefined,
): boolean {
  if (!AddressFieldNames.has(field)) return false;
  if (detail.api_key_config_hidden?.includes(field)) return true;
  // A value in the configuration is shown, even when it was saved empty.
  if (
    Object.prototype.hasOwnProperty.call(detail.api_key_config ?? {}, field)
  ) {
    return false;
  }
  return (
    field === baseUrlField && detail.base_url_configured && !detail.base_url
  );
}

/**
 * Saved fields that a connection test with new credentials cannot reuse:
 * the test sends only the entered credentials, so blank saved credentials
 * and hidden addresses saved with them must be filled in. The server reuses
 * a saved instance base_url by itself. Pass only the fields currently shown.
 */
export function missingTestFields(
  detail: AdminService.PlatformInstanceDetail,
  fieldNames: string[],
  values: Record<string, unknown>,
): string[] {
  return fieldNames.filter((field) => {
    if (values[field]) return false;
    return (
      detail.credential_status?.[field] === true ||
      (AddressFieldNames.has(field) &&
        detail.api_key_config_hidden?.includes(field) === true)
    );
  });
}

/**
 * Models to check in a connection test: one active model per type is enough
 * to prove the credentials, and keeps the number of billed calls small.
 */
export function pickConnectionModels(
  models: AdminService.PlatformModel[],
): AdminService.PlatformModelInput[] {
  const covered = new Set<string>();
  const picked: AdminService.PlatformModelInput[] = [];
  for (const model of models) {
    if (model.status !== 'active') continue;
    const types = model.model_type.filter((type) => !covered.has(type));
    if (types.length === 0) continue;
    types.forEach((type) => covered.add(type));
    picked.push({
      model_name: model.model_name,
      model_type: types,
      max_tokens: model.max_tokens,
    });
  }
  return picked;
}

/** Whether any credential field carries a newly typed value. */
export function hasCredentialInput(values: Record<string, unknown>): boolean {
  return Object.entries(values).some(
    ([field, value]) =>
      isProviderCredentialField(field) &&
      typeof value === 'string' &&
      value.trim() !== '',
  );
}

/**
 * Pick the non-secret settings of a provider configuration so a connection
 * test with saved credentials can still apply the edited settings.
 */
export function pickSafeSettings(
  apiKey: unknown,
): Record<string, unknown> | undefined {
  if (!apiKey || typeof apiKey !== 'object' || Array.isArray(apiKey)) {
    return undefined;
  }
  const settings = Object.fromEntries(
    Object.entries(apiKey as Record<string, unknown>).filter(
      // A blank setting means "keep"; sending it would remove the saved one.
      // null removes a saved setting the admin emptied.
      ([field, value]) =>
        !isProviderCredentialField(field) &&
        value !== undefined &&
        value !== '',
    ),
  );
  return Object.keys(settings).length > 0 ? settings : undefined;
}

/**
 * Mark saved settings that the admin emptied as removed. A blank value
 * otherwise means "keep", so a cleared setting would stay saved. Only shown
 * non-credential fields with a saved value count.
 */
export function withClearedSettings(
  apiKey: string | Record<string, unknown>,
  detail: AdminService.PlatformInstanceDetail,
  shownFields: string[],
  values: Record<string, unknown>,
): string | Record<string, unknown> {
  const saved = detail.api_key_config ?? {};
  const cleared = shownFields.filter(
    (field) =>
      !isProviderCredentialField(field) &&
      typeof saved[field] === 'string' &&
      saved[field] !== '' &&
      (values[field] === undefined || values[field] === ''),
  );
  if (cleared.length === 0) return apiKey;
  // A typed plain key goes under api_key, as the server stores it.
  const config: Record<string, unknown> =
    typeof apiKey === 'string'
      ? apiKey
        ? { api_key: apiKey }
        : {}
      : { ...apiKey };
  cleared.forEach((field) => {
    config[field] = null;
  });
  return config;
}

/**
 * Drop settings that were never saved, that the admin did not edit, and that
 * still hold the value the form filled in (a field or provider default). Sending them would store a
 * default the admin did not choose, for example bring back a setting the
 * admin had cleared; left out, the server keeps applying its own default.
 */
export function withoutUnsavedDefaults(
  apiKey: string | Record<string, unknown>,
  detail: AdminService.PlatformInstanceDetail,
  filled: Record<string, unknown>,
  values: Record<string, unknown>,
  edited: ReadonlySet<string> = new Set(),
): string | Record<string, unknown> {
  if (typeof apiKey === 'string') return apiKey;
  const saved = detail.api_key_config ?? {};
  const config = { ...apiKey };
  for (const field of Object.keys(apiKey)) {
    if (
      !isProviderCredentialField(field) &&
      !Object.prototype.hasOwnProperty.call(saved, field) &&
      // A choice the admin made counts even when it equals the default.
      !edited.has(field) &&
      Object.prototype.hasOwnProperty.call(filled, field) &&
      values[field] === filled[field]
    ) {
      delete config[field];
    }
  }
  return config;
}

/**
 * Focus `target()` once focus is lost because the focused element went away
 * or was disabled (it then falls to the body). A closing dialog is waited
 * for; focus the user moved elsewhere stays put.
 */
export function focusWhenLost(target: () => HTMLElement | null, tries = 20) {
  window.setTimeout(() => {
    const active = document.activeElement;
    if (active?.closest('[role="alertdialog"], [role="dialog"]') && tries > 0) {
      focusWhenLost(target, tries - 1);
      return;
    }
    if (!active || active === document.body || !active.isConnected) {
      target()?.focus();
    }
  }, 50);
}

export interface InstanceSubmission {
  instanceName: string;
  apiKey: string | Record<string, unknown>;
  baseUrl: string;
  region?: string;
}

/**
 * Map the provider form to the platform instance body. The provider's own
 * `submitTransform` keeps the credential shape identical to tenant
 * instances.
 */
export function buildInstanceSubmission(
  config: ProviderConfig,
  values: Record<string, any>,
  regionByUrl: Map<string, string> | undefined,
): InstanceSubmission {
  const transformed = config.submitTransform
    ? config.submitTransform(values)
    : { api_key: values.api_key ?? '', base_url: values.base_url ?? '' };
  const baseUrl = String(transformed.base_url ?? values.base_url ?? '').trim();
  return {
    instanceName: String(values.instance_name ?? '').trim(),
    apiKey: transformed.api_key ?? '',
    baseUrl,
    region: baseUrl ? regionByUrl?.get(baseUrl) : undefined,
  };
}
