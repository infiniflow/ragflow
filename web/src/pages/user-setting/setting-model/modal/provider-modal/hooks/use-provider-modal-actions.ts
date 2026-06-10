import { DynamicFormRef } from '@/components/dynamic-form';
import message from '@/components/ui/message';
import { useTranslate } from '@/hooks/common-hooks';
import { IModelInfo } from '@/interfaces/request/llm';
import { VerifyResult } from '@/pages/user-setting/setting-model/hooks';
import { RefObject, useCallback } from 'react';
import { FieldValues } from 'react-hook-form';
import type {
  IViewModeOkPayload,
  ProviderConfig,
  ProviderModalProps,
} from '../types';

type ActionParams = {
  config: ProviderConfig;
  viewMode?: boolean;
  hasModelNameField: boolean;
  llmFactory: string;
  initialValues?: Record<string, any>;
  modelInfoList: IModelInfo[];
  formRef: RefObject<DynamicFormRef>;
  /**
   * URL → regionKey map for each inputSelect field (built in
   * `useProviderFields`). Used to derive the `region` submit field
   * from the user's currently selected base URL.
   */
  baseUrlRegionMaps?: Record<string, Map<string, string>>;
  onOk: ProviderModalProps['onOk'];
  onVerify: ProviderModalProps['onVerify'];
  onViewModeOk: ProviderModalProps['onViewModeOk'];
};

/**
 * Look up the `region` key (e.g. 'default', 'intl', 'cn') for the
 * currently selected base URL of any inputSelect field. Returns
 * `undefined` when no inputSelect field has a value, or when that
 * value does not match any option's URL — in those cases the caller
 * should leave `region` unset.
 */
const resolveRegionFromValues = (
  values: Record<string, any> | undefined,
  baseUrlRegionMaps?: Record<string, Map<string, string>>,
): string | undefined => {
  if (!values || !baseUrlRegionMaps) return undefined;
  for (const fieldName of Object.keys(baseUrlRegionMaps)) {
    const url = values[fieldName];
    if (typeof url !== 'string' || url === '') continue;
    const regionKey = baseUrlRegionMaps[fieldName].get(url);
    if (regionKey !== undefined) {
      return regionKey;
    }
  }
  return undefined;
};

/**
 * Build the two outbound handlers for the Provider modal:
 *
 * - `handleVerify` reads current form values, runs them through the
 *   provider's `verifyTransform`, and forwards the result to `onVerify`.
 *   Returns a `VerifyResult` (the VerifyButton consumes the shape).
 *
 * - `handleSubmit` has two paths:
 *   1. viewMode → invoke `onViewModeOk` with either the picker's selected
 *      models (LIST_MODEL_PROVIDERS) or the editable form values
 *      (non-LIST_MODEL_PROVIDERS). The instance itself is not re-saved.
 *   2. normal mode → run values through `submitTransform` (when present)
 *      and forward to `onOk`.
 *
 * Both paths inject a `region` field derived from the currently selected
 * base URL whenever the field is an inputSelect (see `baseUrlRegionMaps`).
 */
export const useProviderModalActions = ({
  config,
  viewMode,
  hasModelNameField,
  llmFactory,
  initialValues,
  modelInfoList,
  formRef,
  baseUrlRegionMaps,
  onOk,
  onVerify,
  onViewModeOk,
}: ActionParams) => {
  const { t } = useTranslate('setting');

  const handleVerify = useCallback(
    async (params: any) => {
      const values = formRef.current?.getValues() || params;
      if (!config.verifyTransform) {
        return { isValid: null, logs: '' } as VerifyResult;
      }
      if (hasModelNameField && modelInfoList.length === 0) {
        message.error(t('selectModelBeforeVerify'));
        return { isValid: null, logs: '' } as VerifyResult;
      }
      const verifyArgs = config.verifyTransform({
        ...values,
        model_info: modelInfoList,
      });
      const region = resolveRegionFromValues(values, baseUrlRegionMaps);
      if (region !== undefined) {
        verifyArgs.region = region;
      }
      const res = await onVerify({ ...params, ...verifyArgs });
      return (res || { isValid: null, logs: '' }) as VerifyResult;
    },
    [
      config,
      onVerify,
      modelInfoList,
      formRef,
      baseUrlRegionMaps,
      hasModelNameField,
      t,
    ],
  );

  const handleSubmit = useCallback(
    async (values?: FieldValues) => {
      if (!values) return;

      // viewMode: only add/update models. The instance itself is not
      // re-saved because all instance-level fields are disabled. The
      // parent receives the selected models (or the model-related form
      // values for non-list-model providers) via `onViewModeOk`.
      if (viewMode) {
        if (!onViewModeOk) {
          // No viewMode handler provided — nothing to save, just close
          // (the modal's own hideModal flow handles closing).
          return;
        }
        const instanceName = String(
          (initialValues as any)?.instance_name ?? '',
        );
        const payload: IViewModeOkPayload = hasModelNameField
          ? {
              instanceName,
              llmFactory,
              modelInfos: modelInfoList,
            }
          : {
              instanceName,
              llmFactory,
              modelInfos: [],
              formValues: values as Record<string, any>,
            };
        await onViewModeOk(payload);
        return;
      }

      const transformed = (
        config.submitTransform
          ? config.submitTransform({
              ...(values as Record<string, any>),
              model_info: modelInfoList,
            })
          : values
      ) as Record<string, any>;
      const region = resolveRegionFromValues(
        values as Record<string, any>,
        baseUrlRegionMaps,
      );
      if (region !== undefined) {
        transformed.region = region;
      }
      // Always include `llm_factory` in the submitted payload. Some
      // providers' submitTransforms (e.g. GenericApiKeyConfig) omit it,
      // but the parent uses it to build the request URL
      // (`/api/v1/providers/${llm_factory}/instances`); without it the
      // URL becomes `/providers/undefined/instances`.
      if (!transformed.llm_factory) {
        transformed.llm_factory = llmFactory;
      }
      await onOk?.(transformed, false);
    },
    [
      config,
      onOk,
      onViewModeOk,
      modelInfoList,
      viewMode,
      hasModelNameField,
      llmFactory,
      initialValues,
      baseUrlRegionMaps,
    ],
  );

  return { handleVerify, handleSubmit };
};
