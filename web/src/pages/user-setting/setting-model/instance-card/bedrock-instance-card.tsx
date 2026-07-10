/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { ConfirmDeleteDialog } from '@/components/confirm-delete-dialog';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { MultiSelect } from '@/components/ui/multi-select';
import { Segmented } from '@/components/ui/segmented';
import { useTranslate } from '@/hooks/common-hooks';
import { useBuildModelTypeOptions } from '@/hooks/logic-hooks/use-build-options';
import {
  useAddProviderInstance,
  useDeleteProviderInstance,
  useFetchProviderInstance,
  useVerifyProviderConnection,
} from '@/hooks/use-llm-request';
import { IProviderInstance } from '@/interfaces/database/llm';
import { IAddProviderInstanceRequestBody } from '@/interfaces/request/llm';
import { zodResolver } from '@hookform/resolvers/zod';
import { ListChevronsDownUp, ListChevronsUpDown, Trash2 } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { BedrockRegionList } from '../constants';
import { VerifyResult } from '../hooks';
import { splitProviderPayload } from '../payload-utils';
import { ModelsSection } from './models-section';
import VerifyButton from './verify-button';

type AuthMode = 'access_key_secret' | 'iam_role' | 'assume_role';

type BedrockFormValues = {
  auth_mode: AuthMode;
  bedrock_ak?: string;
  bedrock_sk?: string;
  aws_role_arn?: string;
  bedrock_region: string;
  llm_name: string;
  max_tokens: number;
  model_type: ('chat' | 'embedding')[];
};

// Field names whose value commits via click (Segmented, Select,
// MultiSelect) rather than blur. Their popovers render in Radix
// portals outside the card's blur container, so blur-driven saves
// don't catch them — a form.watch watcher is used instead.
const BEDROCK_WATCHED_FIELDS = new Set([
  'auth_mode',
  'bedrock_region',
  'model_type',
]);

interface BedrockInstanceCardProps {
  providerName: string;
  instance: IProviderInstance;
  isDraft?: boolean;
  onSaved?: (values: Record<string, any>) => void | Promise<void>;
  onNameSaved?: () => void;
  onDelete?: () => void;
  /**
   * When true, this card starts expanded and fetches its instance
   * details on mount. Default `false` so non-first cards stay
   * collapsed until the user opens them.
   */
  defaultOpen?: boolean;
}

/**
 * Inline instance card for AWS Bedrock. Mirrors the two-stage UX of
 * `ProviderInstanceCard` (save name first, then edit fields) but renders
 * Bedrock-specific fields (auth_mode segmented, ak/sk/arn, region, model
 * name, max tokens, model_type) directly instead of going through the
 * generic DynamicForm path.
 */
export function BedrockInstanceCard({
  providerName,
  instance,
  isDraft = false,
  onSaved,
  onNameSaved,
  onDelete,
  defaultOpen = false,
}: BedrockInstanceCardProps) {
  const { t } = useTranslation();
  const { t: tSetting } = useTranslate('setting');
  const { buildModelTypeOptions } = useBuildModelTypeOptions();
  const [open, setOpen] = useState(isDraft || defaultOpen);
  const [draftName, setDraftName] = useState('');
  const [nameSaved, setNameSaved] = useState(!isDraft);
  const savingRef = useRef(false);

  useEffect(() => {
    if (isDraft) {
      setDraftName('');
      setNameSaved(false);
    } else {
      setNameSaved(true);
    }
  }, [providerName, isDraft]);

  const FormSchema = useMemo(
    () =>
      z
        .object({
          auth_mode: z
            .enum(['access_key_secret', 'iam_role', 'assume_role'])
            .default('access_key_secret'),
          bedrock_ak: z.string().optional(),
          bedrock_sk: z.string().optional(),
          aws_role_arn: z.string().optional(),
          bedrock_region: z
            .string()
            .min(1, { message: tSetting('bedrockRegionMessage') }),
          llm_name: z
            .string()
            .min(1, { message: tSetting('bedrockModelNameMessage') }),
          max_tokens: z
            .number({
              required_error: tSetting('maxTokensMessage'),
              invalid_type_error: tSetting('maxTokensInvalidMessage'),
            })
            .nonnegative({ message: tSetting('maxTokensMinMessage') }),
          model_type: z
            .array(z.enum(['chat', 'embedding']))
            .min(1, { message: tSetting('modelTypeMessage') }),
        })
        .superRefine((data, ctx) => {
          if (data.auth_mode === 'access_key_secret') {
            if (!data.bedrock_ak || !data.bedrock_ak.trim()) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: tSetting('bedrockAKMessage'),
                path: ['bedrock_ak'],
              });
            }
            if (!data.bedrock_sk || !data.bedrock_sk.trim()) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: tSetting('bedrockSKMessage'),
                path: ['bedrock_sk'],
              });
            }
          }
          if (data.auth_mode === 'iam_role') {
            if (!data.aws_role_arn || !data.aws_role_arn.trim()) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: tSetting('awsRoleArnMessage'),
                path: ['aws_role_arn'],
              });
            }
          }
        }),
    [tSetting],
  );

  const { data: instanceDetails, refetch: refetchInstanceDetails } =
    useFetchProviderInstance(
      isDraft ? '' : providerName,
      isDraft ? '' : instance.instance_name,
    );

  // Lazily fetch full instance details only when the card is open.
  // Mirrors the generic ProviderInstanceCard: collapsed cards never
  // hit /providers/<name>/instances/<instance_name>; expanding one
  // triggers a fresh refetch.
  useEffect(() => {
    if (!isDraft && open && providerName && instance.instance_name) {
      refetchInstanceDetails();
    }
  }, [
    isDraft,
    open,
    providerName,
    instance.instance_name,
    refetchInstanceDetails,
  ]);

  const initialValues = useMemo<BedrockFormValues>(() => {
    const merged = { ...instance, ...(instanceDetails ?? {}) } as any;
    const apiKey =
      merged.api_key && typeof merged.api_key === 'object'
        ? merged.api_key
        : {};
    return {
      auth_mode: (apiKey.auth_mode as AuthMode) ?? 'access_key_secret',
      bedrock_ak: apiKey.bedrock_ak ?? '',
      bedrock_sk: apiKey.bedrock_sk ?? '',
      aws_role_arn: apiKey.aws_role_arn ?? '',
      bedrock_region:
        merged.region && merged.region !== 'default' ? merged.region : '',
      llm_name: '',
      max_tokens: 8192,
      model_type: ['chat'],
    };
  }, [instance, instanceDetails]);

  const form = useForm<BedrockFormValues>({
    resolver: zodResolver(FormSchema),
    defaultValues: initialValues,
  });

  useEffect(() => {
    // Reset form when initial values change (e.g. instance details load).
    form.reset(initialValues);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialValues]);

  const authMode = useWatch({ control: form.control, name: 'auth_mode' });

  const regionOptions = useMemo(
    () => BedrockRegionList.map((x) => ({ value: x, label: tSetting(x) })),
    [tSetting],
  );

  // Build a Bedrock-shaped payload for both submit and verify flows.
  const buildPayload = useCallback(
    (values: BedrockFormValues, instanceName: string) => {
      const cleaned: Record<string, any> = { ...values };
      const fieldsByMode: Record<AuthMode, string[]> = {
        access_key_secret: ['bedrock_ak', 'bedrock_sk'],
        iam_role: ['aws_role_arn'],
        assume_role: [],
      };
      (Object.keys(fieldsByMode) as AuthMode[]).forEach((mode) => {
        if (mode !== values.auth_mode) {
          fieldsByMode[mode].forEach((f) => {
            delete cleaned[f];
          });
        }
      });

      const flat = {
        ...cleaned,
        instance_name: instanceName,
        llm_factory: providerName,
        max_tokens: values.max_tokens,
        model_type: values.model_type,
      };
      const { instancePayload, modelPayload } = splitProviderPayload(flat);
      return {
        ...instancePayload,
        max_tokens: modelPayload.max_tokens,
        model_info: [modelPayload],
      } as IAddProviderInstanceRequestBody;
    },
    [providerName],
  );

  const { verifyProviderConnection } = useVerifyProviderConnection();
  const handleVerify = useCallback(
    async (params: any) => {
      const isValid = await form.trigger();
      if (!isValid) {
        return {
          isValid: false,
          logs: tSetting('bedrockRegionMessage'),
        } as VerifyResult;
      }
      const values = form.getValues();
      const payload = buildPayload(
        values,
        draftName.trim() || instance.instance_name,
      );
      const { instancePayload, modelPayload } = splitProviderPayload({
        ...payload,
        ...values,
        llm_factory: providerName,
        instance_name: draftName.trim() || instance.instance_name,
      });
      const ret = await verifyProviderConnection({
        provider_name: providerName,
        api_key: JSON.stringify(instancePayload.api_key),
        base_url: instancePayload.base_url,
        region: instancePayload.region,
        model_info: [modelPayload],
        ...params,
      });
      return {
        isValid: ret.code === 0,
        logs: ret.message,
      } as VerifyResult;
    },
    [
      form,
      providerName,
      buildPayload,
      draftName,
      instance.instance_name,
      verifyProviderConnection,
      tSetting,
    ],
  );

  const { addProviderInstance } = useAddProviderInstance();

  const handleSaveName = useCallback(async () => {
    const trimmed = draftName.trim();
    if (!trimmed) return;
    const ret = await addProviderInstance({
      llm_factory: providerName,
      instance_name: trimmed,
    } as any);
    if (ret?.code === 0) {
      onNameSaved?.();
    }
  }, [draftName, addProviderInstance, providerName, onNameSaved]);

  // Auto-save in draft mode after the name is locked. Debounced on form
  // value changes; refuses to fire until validation passes.
  useEffect(() => {
    if (!isDraft) return;
    if (!nameSaved) return;
    let saveTimeout: ReturnType<typeof setTimeout> | null = null;
    let cancelled = false;
    const sub = form.watch(() => {
      if (saveTimeout) clearTimeout(saveTimeout);
      saveTimeout = setTimeout(async () => {
        if (cancelled || savingRef.current) return;
        const isValid = await form.trigger();
        if (cancelled || savingRef.current) return;
        if (!isValid) return;
        const trimmed = draftName.trim();
        if (!trimmed) return;
        savingRef.current = true;
        try {
          const values = form.getValues();
          const payload = buildPayload(values, trimmed);
          await onSaved?.(payload as unknown as Record<string, any>);
        } finally {
          savingRef.current = false;
        }
      }, 200);
    });
    return () => {
      cancelled = true;
      if (saveTimeout) clearTimeout(saveTimeout);
      try {
        sub?.unsubscribe?.();
      } catch {
        // ignore
      }
    };
  }, [isDraft, nameSaved, form, draftName, buildPayload, onSaved]);

  // Saved-mode auto-save. Both blur-driven (text inputs) and
  // change-driven (Segmented / Select / MultiSelect) edits are
  // coalesced through a shared debounced `scheduleSave`. Selects render
  // in Radix portals outside the card's blur container, so blur-driven
  // saves don't catch them — a form.watch watcher is used instead.
  const blurSavingRef = useRef(false);
  // Flipped to true while a child (e.g. ModelsSection's
  // AddCustomModelDialog) opens a Portal-based dialog. Suppresses the
  // spurious blur-save fired when focus moves into the Portal.
  const blurSuppressRef = useRef(false);
  const lastSavedSigRef = useRef('');
  const autoSaveTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const AUTO_SAVE_DEBOUNCE_MS = 500;

  const performSave = useCallback(async () => {
    if (isDraft) return;
    if (blurSavingRef.current) return;
    if (blurSuppressRef.current) return;
    const isValid = await form.trigger();
    if (!isValid) return;
    const values = form.getValues();
    const payload = buildPayload(values, instance.instance_name);
    const finalPayload = {
      ...payload,
      id: instanceDetails?.id || instance.id,
    };
    const sig = JSON.stringify(finalPayload);
    if (sig === lastSavedSigRef.current) return;
    blurSavingRef.current = true;
    try {
      const ret = await addProviderInstance(finalPayload as any);
      if (ret?.code === 0) {
        lastSavedSigRef.current = sig;
      }
    } finally {
      blurSavingRef.current = false;
    }
  }, [
    isDraft,
    form,
    buildPayload,
    instance.instance_name,
    instance.id,
    instanceDetails?.id,
    addProviderInstance,
  ]);

  const scheduleSave = useCallback(() => {
    if (isDraft) return;
    if (autoSaveTimeoutRef.current) {
      clearTimeout(autoSaveTimeoutRef.current);
    }
    autoSaveTimeoutRef.current = setTimeout(() => {
      autoSaveTimeoutRef.current = null;
      void performSave();
    }, AUTO_SAVE_DEBOUNCE_MS);
  }, [isDraft, performSave]);

  const handleFieldsBlur = useCallback(
    (e: React.FocusEvent<HTMLDivElement>) => {
      if (isDraft) return;
      if (
        e.currentTarget.contains(e.relatedTarget as Node | null) &&
        e.relatedTarget !== null
      ) {
        return;
      }
      scheduleSave();
    },
    [isDraft, scheduleSave],
  );

  // Segmented / Select / MultiSelect change-driven save (saved mode
  // only). These commit via click and their popovers render in portals,
  // so blur-driven saves don't catch them. Watch the form directly.
  // Only react to user-driven changes (type === 'change'); ignore
  // programmatic resets (form.reset when instanceDetails loads).
  useEffect(() => {
    if (isDraft) return;
    if (!instanceDetails) return;
    let cancelled = false;
    const subscription = form.watch(
      (_values: any, meta: { name?: string; type?: string }) => {
        if (cancelled) return;
        if (meta?.type !== 'change') return;
        if (!meta?.name || !BEDROCK_WATCHED_FIELDS.has(meta.name)) return;
        scheduleSave();
      },
    );
    return () => {
      cancelled = true;
      try {
        subscription?.unsubscribe?.();
      } catch {
        // ignore
      }
    };
  }, [isDraft, instanceDetails, form, scheduleSave]);

  // Clear pending save on unmount.
  useEffect(() => {
    return () => {
      if (autoSaveTimeoutRef.current) {
        clearTimeout(autoSaveTimeoutRef.current);
        autoSaveTimeoutRef.current = null;
      }
    };
  }, []);

  const { deleteProviderInstance } = useDeleteProviderInstance();
  const handleDelete = useCallback(async () => {
    if (isDraft) {
      onDelete?.();
    } else {
      await deleteProviderInstance({
        provider_name: providerName,
        instances: [instance.instance_name],
      });
    }
  }, [
    isDraft,
    providerName,
    instance.instance_name,
    deleteProviderInstance,
    onDelete,
  ]);

  // ──────────────── Field group rendered in both modes ────────────────
  const renderFields = () => (
    <Form {...form}>
      <form className="space-y-6" onSubmit={(e) => e.preventDefault()}>
        <RAGFlowFormItem
          name="model_type"
          label={tSetting('modelType')}
          required
        >
          {(field) => (
            <MultiSelect
              options={buildModelTypeOptions(['chat', 'embedding'])}
              placeholder={tSetting('modelTypeMessage')}
              onValueChange={field.onChange}
              defaultValue={field.value}
              variant="inverted"
              maxCount={100}
            />
          )}
        </RAGFlowFormItem>

        <RAGFlowFormItem name="llm_name" label={tSetting('modelName')} required>
          <Input placeholder={tSetting('bedrockModelNameMessage')} />
        </RAGFlowFormItem>

        <div>
          <RAGFlowFormItem name="auth_mode">
            {(field) => (
              <Segmented
                value={field.value}
                onChange={(value) => {
                  if (value !== 'access_key_secret') {
                    form.setValue('bedrock_ak', '');
                    form.setValue('bedrock_sk', '');
                  }
                  if (value !== 'iam_role') {
                    form.setValue('aws_role_arn', '');
                  }
                  field.onChange(value);
                }}
                options={[
                  {
                    label: tSetting('awsAuthModeAccessKeySecret'),
                    value: 'access_key_secret',
                  },
                  { label: tSetting('awsAuthModeIamRole'), value: 'iam_role' },
                  {
                    label: tSetting('awsAuthModeAssumeRole'),
                    value: 'assume_role',
                  },
                ]}
              />
            )}
          </RAGFlowFormItem>
        </div>

        {authMode === 'access_key_secret' && (
          <>
            <RAGFlowFormItem
              name="bedrock_ak"
              label={tSetting('awsAccessKeyId')}
              required
            >
              <Input placeholder={tSetting('bedrockAKMessage')} />
            </RAGFlowFormItem>
            <RAGFlowFormItem
              name="bedrock_sk"
              label={tSetting('awsSecretAccessKey')}
              required
            >
              <Input placeholder={tSetting('bedrockSKMessage')} />
            </RAGFlowFormItem>
          </>
        )}

        {authMode === 'iam_role' && (
          <RAGFlowFormItem
            name="aws_role_arn"
            label={tSetting('awsRoleArn')}
            required
          >
            <Input placeholder={tSetting('awsRoleArnMessage')} />
          </RAGFlowFormItem>
        )}

        {authMode === 'assume_role' && (
          <div className="text-sm text-text-secondary">
            {tSetting('awsAssumeRoleTip')}
          </div>
        )}

        <RAGFlowFormItem
          name="bedrock_region"
          label={tSetting('bedrockRegion')}
          required
        >
          {(field) => (
            <SelectWithSearch
              value={field.value}
              onChange={field.onChange}
              options={regionOptions}
              placeholder={tSetting('bedrockRegionMessage')}
              allowClear
            />
          )}
        </RAGFlowFormItem>

        <RAGFlowFormItem
          name="max_tokens"
          label={tSetting('maxTokens')}
          required
        >
          {(field) => (
            <Input
              type="number"
              placeholder={tSetting('maxTokensTip')}
              value={field.value}
              onChange={(e) => field.onChange(Number(e.target.value))}
            />
          )}
        </RAGFlowFormItem>
      </form>

      {/* VerifyButton lives inside <Form> (FormProvider) so its
          internal useFormContext() resolves the form instance.
          Rendered outside <form> so it never triggers submission. */}
      <div className="pt-3">
        <VerifyButton onVerify={handleVerify} isAbsolute={false} />
      </div>
    </Form>
  );

  return (
    <div
      className="border-b border-border-button mb-5 pb-5"
      data-testid={`instance-card-${instance.instance_name || 'draft'}`}
    >
      {nameSaved ? (
        <Collapsible open={open} onOpenChange={setOpen}>
          <CollapsibleTrigger asChild>
            <div className="flex items-center gap-1 w-full mb-5">
              <div
                className="group flex items-center flex-1 gap-2 px-2 mx-2 py-1 cursor-pointer bg-bg-input rounded-md"
                data-testid="instance-name-row"
              >
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={
                    open ? t('setting.hideModels') : t('setting.showMoreModels')
                  }
                  data-testid="instance-collapse"
                >
                  {open ? (
                    <ListChevronsDownUp className="size-4" />
                  ) : (
                    <ListChevronsUpDown className="size-4" />
                  )}
                </Button>
                <span
                  className="text-sm font-medium"
                  data-testid="instance-name-static"
                >
                  {draftName || instance.instance_name}
                </span>
              </div>
              <ConfirmDeleteDialog onOk={handleDelete}>
                <Button
                  variant="delete"
                  size="icon-sm"
                  aria-label={tSetting('deleteInstance')}
                  data-testid="instance-delete"
                  onClick={(e: React.MouseEvent) => e.stopPropagation()}
                >
                  <Trash2 className="size-4" />
                </Button>
              </ConfirmDeleteDialog>
            </div>
          </CollapsibleTrigger>
          <CollapsibleContent
            forceMount
            className="data-[state=closed]:hidden overflow-hidden"
          >
            <div
              className="px-2 pb-4 flex flex-col gap-4"
              onBlurCapture={handleFieldsBlur}
            >
              {renderFields()}

              <div className="pt-3">
                <ModelsSection
                  providerName={providerName}
                  instanceName={instance.instance_name || '__draft__'}
                  instance={instance}
                  hideActions={false}
                  hideIfEmpty={false}
                  getFormValues={() => form.getValues()}
                  onBlurSuppressChange={(s) => {
                    blurSuppressRef.current = s;
                  }}
                />
              </div>
            </div>
          </CollapsibleContent>
        </Collapsible>
      ) : (
        <div className="px-2 py-3 flex flex-col gap-4">
          <div
            className="flex flex-col gap-1.5"
            data-testid="instance-name-section"
          >
            <label
              htmlFor="instance-name-input"
              className="text-sm font-medium text-text-primary"
            >
              <span className="text-destructive mr-0.5">*</span>
              {tSetting('instanceName')}
            </label>
            <div className="flex items-center">
              <Input
                id="instance-name-input"
                value={draftName}
                onChange={(e) => setDraftName(e.target.value)}
                placeholder={tSetting('instanceNamePlaceholder')}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') {
                    e.preventDefault();
                    handleSaveName();
                  }
                }}
                className="flex-1 rounded-r-none"
                data-testid="instance-name-input"
              />
              <Button
                onClick={handleSaveName}
                disabled={!draftName.trim()}
                data-testid="instance-name-save"
                variant="outline"
                className="rounded-l-none bg-bg-input shrink-0"
              >
                {tSetting('save')}
              </Button>
              <ConfirmDeleteDialog onOk={handleDelete}>
                <Button
                  variant="delete"
                  size="icon-sm"
                  className="ml-2 shrink-0"
                  aria-label={tSetting('deleteInstance')}
                  data-testid="draft-delete"
                >
                  <Trash2 className="size-4" />
                </Button>
              </ConfirmDeleteDialog>
            </div>
            <p
              className="text-xs text-text-secondary"
              data-testid="instance-name-helper"
            >
              {tSetting('instanceNameSaveTip')}
            </p>
          </div>

          <fieldset
            disabled={!nameSaved}
            className="contents disabled:[&_*]:pointer-events-none disabled:opacity-60"
            data-testid="instance-locked-fields"
          >
            {renderFields()}

            <div className="pt-3">
              <ModelsSection
                providerName={providerName}
                instanceName={instance.instance_name || '__draft__'}
                instance={instance}
                hideActions={false}
                hideIfEmpty={false}
                getFormValues={() => form.getValues()}
              />
            </div>
          </fieldset>
        </div>
      )}
    </div>
  );
}

export default BedrockInstanceCard;
