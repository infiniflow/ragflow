import { FormFieldConfig, FormFieldType } from '@/components/dynamic-form';
import { Input } from '@/components/ui/input';
import { InputSelect } from '@/components/ui/input-select';
import { useTranslate } from '@/hooks/common-hooks';
import { useMemo } from 'react';
import { ControllerRenderProps, FieldValues } from 'react-hook-form';
import { LIST_MODEL_FIELD_NAMES, LIST_MODEL_PROVIDERS } from '../constants';
import { FACTORIES_WITH_BASE_URL, getProviderConfig } from '../field-config';
import type { FieldConfig, SelectOption } from '../types';

interface UseProviderFieldsParams {
  llmFactory: string;
  editMode?: boolean;
  viewMode?: boolean;
  initialValues?: Record<string, any>;
  baseUrlOptions?: SelectOption[];
  hideWhenInstanceExists: (values: Record<string, any>) => boolean;
}

/**
 * Resolve a text value that may be a static i18n key or a factory-aware
 * resolver, then translate it via `t`. Used for `placeholder` and `tooltip`
 * so that a single FieldConfig entry can render different text per provider
 * (e.g. the generic `base_url` field has a Minimax-specific tooltip).
 */
const resolveText = (
  val: string | ((factory: string) => string) | undefined,
  factory: string,
  t: (key: string) => string,
): string | undefined => {
  if (!val) return undefined;
  const key = typeof val === 'function' ? val(factory) : val;
  return t(key);
};

/** Set value by nested path (supports paths like 'model_info.model_type'). */
const setNestedValue = (obj: any, path: string, value: any) => {
  const keys = path.split('.');
  let current = obj;
  for (let i = 0; i < keys.length - 1; i++) {
    const key = keys[i];
    if (!current[key]) {
      current[key] = {};
    }
    current = current[key];
  }
  current[keys[keys.length - 1]] = value;
};

/**
 * Builds the form field config, default values, and doc-link text for the
 * Provider modal. Handles:
 * - Per-provider text resolution (placeholder / tooltip).
 * - shouldRender token → predicate resolution.
 * - Hiding the 4 model_* fields when the list-models picker is active.
 * - Disabling non-model fields in viewMode.
 * - Custom inputSelect rendering (Input + dropdown of suggestions).
 */
export const useProviderFields = ({
  llmFactory,
  editMode,
  viewMode,
  initialValues,
  baseUrlOptions,
  hideWhenInstanceExists,
}: UseProviderFieldsParams) => {
  const { t } = useTranslate('setting');

  const config = useMemo(() => getProviderConfig(llmFactory), [llmFactory]);

  // Whether this factory should render the "List Models" picker. Only the
  // providers listed in `LIST_MODEL_PROVIDERS` opt into the picker; all
  // others keep the traditional model_name / model_type / max_tokens /
  // is_tools form fields.
  const hasModelNameField = useMemo(
    () => LIST_MODEL_PROVIDERS.has(llmFactory),
    [llmFactory],
  );

  // Resolve the shouldRender string token to an actual predicate.
  const resolveShouldRender = useMemo(() => {
    return (sr: FieldConfig['shouldRender']) => {
      if (!sr) return undefined;
      if (typeof sr === 'function') return sr;

      switch (sr) {
        case 'hideWhenInstanceExists':
          return hideWhenInstanceExists;
        case 'modelTypeIncludesChat':
          return (values: any) => {
            const mt = values?.model_type;
            if (Array.isArray(mt)) return mt.includes('chat');
            return mt === 'chat';
          };
        case 'modelTypeSupportsToolCall':
          return (values: any) => {
            const mt = values?.model_type;
            if (Array.isArray(mt)) {
              return mt.includes('chat') || mt.includes('image2text');
            }
            return mt === 'chat' || mt === 'image2text';
          };
        case 'modelTypeIncludesTtsAndNotExists':
          return (values: any) => {
            if (!hideWhenInstanceExists(values)) return false;
            const mt = values?.model_type;
            if (Array.isArray(mt)) return mt.includes('tts');
            return mt === 'tts';
          };
        case 'showBaseUrl':
          return () =>
            FACTORIES_WITH_BASE_URL.some((x) => x === llmFactory) ||
            llmFactory?.toLowerCase() === 'Anthropic'.toLowerCase();
        case 'showGroupId':
          return () => llmFactory?.toLowerCase() === 'Minimax'.toLowerCase();
        default:
          return undefined;
      }
    };
  }, [hideWhenInstanceExists, llmFactory]);

  // For each inputSelect field, build a URL → regionKey map from its
  // options (either inline `field.options` or the shared `baseUrlOptions`).
  // The map is used both to pick the "default" key's URL as the form's
  // initial value and (downstream, in useProviderModalActions) to derive
  // the `region` submit field from the user's currently selected URL.
  const baseUrlRegionMaps = useMemo(() => {
    const maps: Record<string, Map<string, string>> = {};
    config.fields.forEach((field) => {
      if (field.type !== 'inputSelect') return;
      const options =
        field.options && field.options.length > 0
          ? field.options
          : (baseUrlOptions ?? []);
      const urlMap = new Map<string, string>();
      options.forEach((opt) => {
        if (opt.regionKey) {
          urlMap.set(opt.value, opt.regionKey);
        }
      });
      if (urlMap.size > 0) {
        maps[field.name] = urlMap;
      }
    });
    return maps;
  }, [config.fields, baseUrlOptions]);

  // Convert FieldConfig to FormFieldConfig (for use by DynamicForm)
  const fields: FormFieldConfig[] = useMemo(() => {
    const res = config.fields
      .filter(
        // When the list-models picker is active, the 4 model_* fields are
        // owned by the picker (component state) and must not be registered
        // in the dynamic form.
        (field) =>
          !hasModelNameField || !LIST_MODEL_FIELD_NAMES.has(field.name),
      )
      .map((field) => {
        const placeholderText = resolveText(field.placeholder, llmFactory, t);
        const tooltipText = resolveText(field.tooltip, llmFactory, t);
        const validation = field.validation
          ? {
              ...field.validation,
              message: field.validation.message
                ? t(field.validation.message)
                : undefined,
            }
          : undefined;
        const baseField: Omit<FormFieldConfig, 'type'> = {
          name: field.name,
          label: t(field.label),
          required: field.required,
          hidden: false,
          placeholder: placeholderText,
          tooltip: tooltipText,
          options: field.options?.map((o) => ({
            label: o.label as string,
            value: o.value,
          })) as any,
          defaultValue: field.defaultValue,
          validation,
          shouldRender: resolveShouldRender(field.shouldRender),
          // In viewMode, only the model-related fields are editable.
          // All other fields (instance_name, api_key, base_url, etc.)
          // are rendered as disabled.
          disabled: !!viewMode && !LIST_MODEL_FIELD_NAMES.has(field.name),
          dependencies:
            field.shouldRender === 'modelTypeIncludesChat' ||
            field.shouldRender === 'modelTypeSupportsToolCall' ||
            field.shouldRender === 'modelTypeIncludesTtsAndNotExists'
              ? ['model_type']
              : ['model_type', 'instance_name'].includes(field.name)
                ? ['model_type', 'instance_name']
                : undefined,
        };

        // inputSelect type: use the InputSelect component, options come from baseUrlOptions
        if (field.type === 'inputSelect') {
          const inputSelectOptions: SelectOption[] =
            field.options && field.options.length > 0
              ? field.options
              : (baseUrlOptions ?? []);
          return {
            ...baseField,
            type: FormFieldType.Custom,
            options: inputSelectOptions as any,
            render: (fieldProps: ControllerRenderProps) => {
              return inputSelectOptions.length > 0 ? (
                <InputSelect
                  {...fieldProps}
                  value={(fieldProps.value ?? '') as string}
                  onChange={(value) => fieldProps.onChange(value)}
                  options={inputSelectOptions as any}
                  placeholder={placeholderText}
                />
              ) : (
                <Input {...fieldProps} placeholder={placeholderText} />
              );
            },
          };
        }

        // Other types use the enum value directly from the field config
        return { ...baseField, type: field.type };
      });
    return res;
  }, [
    config.fields,
    resolveShouldRender,
    t,
    baseUrlOptions,
    llmFactory,
    hasModelNameField,
    viewMode,
  ]);

  const defaultValues: FieldValues = useMemo(() => {
    // In editMode or viewMode, seed the form with the supplied
    // `initialValues` so the user sees the existing instance/model data.
    if ((editMode || viewMode) && initialValues) {
      return initialValues as FieldValues;
    }
    const result: FieldValues = {};
    config.fields.forEach((f) => {
      if (f.defaultValue !== undefined) {
        setNestedValue(result, f.name, f.defaultValue);
        return;
      }
      // For inputSelect fields, default the form to the option whose
      // original key in the URL object is 'default' (e.g.
      // `availableProviders.url.default`). If no such option exists,
      // leave the field empty so the user picks one explicitly.
      if (f.type === 'inputSelect') {
        const urlMap = baseUrlRegionMaps[f.name];
        if (urlMap) {
          for (const [url, regionKey] of urlMap.entries()) {
            if (regionKey === 'default') {
              setNestedValue(result, f.name, url);
              return;
            }
          }
        }
      }
    });
    // For configurations without a default model_type, assign an empty array (multi-select field)
    const mtField = config.fields.find((f) => f.name === 'model_type');
    if (mtField && mtField.defaultValue === undefined) {
      setNestedValue(result, 'model_type', []);
    }
    return result;
  }, [
    editMode,
    viewMode,
    initialValues,
    config.fields,
    llmFactory,
    baseUrlRegionMaps,
  ]);

  // Documentation link text (rendered at the bottom of the modal)
  const docLinkText = useMemo(() => {
    if (config.docLinkText) return config.docLinkText;
    if (config.docLinkI18nKey) {
      return t(config.docLinkI18nKey, { name: llmFactory });
    }
    return null;
  }, [config, llmFactory, t]);

  return {
    config,
    fields,
    defaultValues,
    docLinkText,
    hasModelNameField,
    baseUrlRegionMaps,
  };
};
