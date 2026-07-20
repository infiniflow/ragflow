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
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { RAGFlowSelect } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { useTranslate } from '@/hooks/common-hooks';
import {
  useDeleteProviderInstance,
  useFetchProviderInstance,
  useVerifyProviderConnection,
} from '@/hooks/use-llm-request';
import { IProviderInstance } from '@/interfaces/database/llm';
import { IAddProviderInstanceRequestBody } from '@/interfaces/request/llm';
import { zodResolver } from '@hookform/resolvers/zod';
import { ListChevronsDownUp, ListChevronsUpDown, Trash2 } from 'lucide-react';
import {
  forwardRef,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { VerifyResult } from '../hooks';
import {
  ProviderInstanceCardProps,
  ProviderInstanceCardRef,
} from './interface';
import VerifyButton from './verify-button';

const IMAGE_FORMATS = ['url', 'base64', 'none'] as const;
const FORMULA_FORMATS = ['latex', 'mathml', 'ascii'] as const;
const TABLE_FORMATS = ['html', 'markdown', 'image'] as const;
const CS_FORMATS = ['image'] as const;
const FORMAT_LABELS = {
  url: 'URL',
  base64: 'Base64',
  none: 'None',
  latex: 'LaTeX',
  mathml: 'MathML',
  ascii: 'ASCII',
  html: 'HTML',
  markdown: 'Markdown',
  image: 'Image',
} as const;

const buildFormatOptions = <T extends keyof typeof FORMAT_LABELS>(
  formats: readonly T[],
) => formats.map((value) => ({ label: FORMAT_LABELS[value], value }));

type SoMarkFormValues = {
  llm_name: string;
  somark_base_url: string;
  somark_api_key?: string;
  somark_image_format: (typeof IMAGE_FORMATS)[number];
  somark_formula_format: (typeof FORMULA_FORMATS)[number];
  somark_table_format: (typeof TABLE_FORMATS)[number];
  somark_cs_format: (typeof CS_FORMATS)[number];
  somark_enable_text_cross_page: boolean;
  somark_enable_table_cross_page: boolean;
  somark_enable_title_level_recognition: boolean;
  somark_enable_inline_image: boolean;
  somark_enable_table_image: boolean;
  somark_enable_image_understanding: boolean;
  somark_keep_header_footer: boolean;
};

interface SoMarkInstanceCardProps {
  providerName: string;
  instance: IProviderInstance;
  isDraft?: boolean;
  onDelete?: () => void;
  defaultOpen?: boolean;
}

/**
 * Inline instance card for SoMark. Renders SoMark-specific fields
 * (model name, base URL, API key, 4 element-format selects, 7 feature
 * toggles) directly. The model type is fixed to `['ocr']` (SoMark is
 * an OCR provider) and not exposed in the form.
 *
 * All fields are editable from the start (no name-first lock); the
 * parent page's top Save button drives persistence through the
 * imperative ref API.
 *
 * Payload shape (matches the legacy `useSubmitSoMark` hook so the
 * backend contract is unchanged):
 *   {
 *     instance_name, llm_factory: 'SoMark',
 *     api_key: somark_api_key || '',
 *     base_url: somark_base_url,
 *     max_tokens: 0,
 *     model_info: [{
 *       llm_name, model_type: ['ocr'], max_tokens: 0,
 *       extra: { somark_image_format, somark_formula_format, ... }
 *     }]
 *   }
 */
export const SoMarkInstanceCard = forwardRef<
  ProviderInstanceCardRef,
  SoMarkInstanceCardProps
>(function SoMarkInstanceCard(
  { providerName, instance, isDraft = false, onDelete, defaultOpen = false },
  ref,
) {
  const { t } = useTranslation();
  const { t: tSetting } = useTranslate('setting');
  const [open, setOpen] = useState(isDraft || defaultOpen);
  const [draftName, setDraftName] = useState('');

  useEffect(() => {
    if (isDraft) {
      setDraftName('');
    }
  }, [providerName, isDraft]);

  const FormSchema = useMemo(
    () =>
      z.object({
        llm_name: z.string().min(1, {
          message: tSetting('somark.modelNameMessage'),
        }),
        somark_base_url: z.string().min(1, {
          message: tSetting('somark.baseUrlMessage'),
        }),
        somark_api_key: z.string().optional(),
        somark_image_format: z.enum(IMAGE_FORMATS),
        somark_formula_format: z.enum(FORMULA_FORMATS),
        somark_table_format: z.enum(TABLE_FORMATS),
        somark_cs_format: z.enum(CS_FORMATS),
        somark_enable_text_cross_page: z.boolean(),
        somark_enable_table_cross_page: z.boolean(),
        somark_enable_title_level_recognition: z.boolean(),
        somark_enable_inline_image: z.boolean(),
        somark_enable_table_image: z.boolean(),
        somark_enable_image_understanding: z.boolean(),
        somark_keep_header_footer: z.boolean(),
      }),
    [tSetting],
  );

  const { data: instanceDetails, refetch: refetchInstanceDetails } =
    useFetchProviderInstance(
      isDraft ? '' : providerName,
      isDraft ? '' : instance.instance_name,
    );

  // Lazily fetch full instance details only when the card is open.
  // Collapsed cards never hit /providers/<name>/instances/<instance_name>;
  // expanding one triggers a fresh refetch.
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

  // Build initial values from the persisted instance + lazy-loaded details.
  // SoMark stores its provider-specific fields inside
  // `model_info[0].extra`; `api_key` and `base_url` live at the
  // instance top level. Map them back to the form's flat shape.
  const initialValues = useMemo<SoMarkFormValues>(() => {
    const merged: any = { ...instance, ...(instanceDetails ?? {}) };
    const rawApiKey = merged.api_key;
    const apiKey =
      typeof rawApiKey === 'string'
        ? rawApiKey
        : rawApiKey && typeof rawApiKey === 'object'
          ? (rawApiKey.api_key ?? '')
          : '';
    const modelInfo = Array.isArray(merged.model_info)
      ? merged.model_info[0]
      : null;
    const extra = (modelInfo?.extra ?? {}) as Record<string, any>;
    return {
      llm_name: modelInfo?.llm_name ?? modelInfo?.model_name ?? '',
      somark_base_url: (merged.base_url as string) ?? '',
      somark_api_key: apiKey,
      somark_image_format:
        (extra.somark_image_format as (typeof IMAGE_FORMATS)[number]) ?? 'url',
      somark_formula_format:
        (extra.somark_formula_format as (typeof FORMULA_FORMATS)[number]) ??
        'latex',
      somark_table_format:
        (extra.somark_table_format as (typeof TABLE_FORMATS)[number]) ?? 'html',
      somark_cs_format:
        (extra.somark_cs_format as (typeof CS_FORMATS)[number]) ?? 'image',
      somark_enable_text_cross_page:
        extra.somark_enable_text_cross_page ?? false,
      somark_enable_table_cross_page:
        extra.somark_enable_table_cross_page ?? false,
      somark_enable_title_level_recognition:
        extra.somark_enable_title_level_recognition ?? false,
      somark_enable_inline_image: extra.somark_enable_inline_image ?? false,
      somark_enable_table_image: extra.somark_enable_table_image ?? true,
      somark_enable_image_understanding:
        extra.somark_enable_image_understanding ?? true,
      somark_keep_header_footer: extra.somark_keep_header_footer ?? false,
    };
  }, [instance, instanceDetails]);

  const form = useForm<SoMarkFormValues>({
    resolver: zodResolver(FormSchema),
    defaultValues: initialValues,
  });

  useEffect(() => {
    // Reset form when initial values change (e.g. instance details load).
    form.reset(initialValues);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initialValues]);

  const imageFormatOptions = useMemo(
    () => buildFormatOptions(IMAGE_FORMATS),
    [],
  );
  const formulaFormatOptions = useMemo(
    () => buildFormatOptions(FORMULA_FORMATS),
    [],
  );
  const tableFormatOptions = useMemo(
    () => buildFormatOptions(TABLE_FORMATS),
    [],
  );
  const csFormatOptions = useMemo(() => buildFormatOptions(CS_FORMATS), []);

  // Build a SoMark-shaped payload for both submit and verify flows.
  // Mirrors the legacy `useSubmitSoMark` hook so the backend contract
  // is unchanged: api_key/base_url at the instance level, all somark_*
  // feature/format fields inside model_info[0].extra.
  const buildPayload = useCallback(
    (values: SoMarkFormValues, instanceName: string) => {
      const extra = {
        somark_image_format: values.somark_image_format,
        somark_formula_format: values.somark_formula_format,
        somark_table_format: values.somark_table_format,
        somark_cs_format: values.somark_cs_format,
        somark_enable_text_cross_page: values.somark_enable_text_cross_page,
        somark_enable_table_cross_page: values.somark_enable_table_cross_page,
        somark_enable_title_level_recognition:
          values.somark_enable_title_level_recognition,
        somark_enable_inline_image: values.somark_enable_inline_image,
        somark_enable_table_image: values.somark_enable_table_image,
        somark_enable_image_understanding:
          values.somark_enable_image_understanding,
        somark_keep_header_footer: values.somark_keep_header_footer,
      };
      return {
        instance_name: instanceName,
        llm_factory: providerName,
        api_key: values.somark_api_key ?? '',
        base_url: values.somark_base_url,
        max_tokens: 0,
        model_info: [
          {
            model_name: values.llm_name,
            model_type: ['ocr'],
            max_tokens: 0,
            extra,
          },
        ],
      } as unknown as IAddProviderInstanceRequestBody;
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
          logs: tSetting('somark.baseUrlMessage'),
        } as VerifyResult;
      }
      const values = form.getValues();
      const payload = buildPayload(
        values,
        draftName.trim() || instance.instance_name,
      );
      const ret = await verifyProviderConnection({
        provider_name: providerName,
        api_key: (payload as any).api_key,
        base_url: (payload as any).base_url,
        model_info: (payload as any).model_info,
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

  // ── Dirty tracking (no auto-save) ────────────────────────────────
  // Baseline signature mirrors the persisted state so `getSavePayload`
  // can skip redundant saves. For drafts the baseline stays empty
  // (drafts are always dirty once a name is typed).
  const baselinePayloadRef = useRef<string>('');
  const draftNameRef = useRef(draftName);
  useEffect(() => {
    draftNameRef.current = draftName;
  });

  useEffect(() => {
    if (isDraft) {
      baselinePayloadRef.current = '';
      return;
    }
    if (!instanceDetails && !instance.id) return;
    const baseline = buildPayload(initialValues, instance.instance_name);
    const finalBaseline = {
      ...baseline,
      id: instanceDetails?.id || instance.id,
    };
    baselinePayloadRef.current = JSON.stringify(finalBaseline);
  }, [
    isDraft,
    initialValues,
    buildPayload,
    instance.instance_name,
    instance.id,
    instanceDetails,
  ]);

  const getSavePayload = useCallback(() => {
    const trimmed = draftNameRef.current.trim();
    if (isDraft) {
      if (!trimmed) return null;
      const values = form.getValues();
      const payload = buildPayload(values, trimmed);
      return {
        payload,
        instanceName: trimmed,
        isDraft: true,
        // SoMark drafts use the add endpoint (no id).
        apiKind: 'add' as const,
      };
    }
    const values = form.getValues();
    const payload = buildPayload(values, instance.instance_name);
    const finalPayload = {
      ...payload,
      id: instanceDetails?.id || instance.id,
    };
    const sig = JSON.stringify(finalPayload);
    if (sig === baselinePayloadRef.current) return null;
    return {
      payload: finalPayload,
      instanceName: instance.instance_name,
      isDraft: false,
      // SoMark saved cards update via `addProviderInstance` with an `id`
      // (matches the legacy auto-save behaviour).
      apiKind: 'add' as const,
    };
  }, [
    isDraft,
    form,
    buildPayload,
    instance.instance_name,
    instance.id,
    instanceDetails,
  ]);

  const markSaved = useCallback(() => {
    const result = getSavePayload();
    if (result) {
      baselinePayloadRef.current = JSON.stringify(result.payload);
    }
  }, [getSavePayload]);

  useImperativeHandle(
    ref,
    () => ({
      validate: async () => {
        if (isDraft && !draftNameRef.current.trim()) return false;
        const isValid = await form.trigger();
        return !!isValid;
      },
      getSavePayload,
      markSaved,
    }),
    [isDraft, form, getSavePayload, markSaved],
  );

  // ──────────────── Field group rendered in both modes ────────────────
  const renderFields = () => (
    <Form {...form}>
      <form className="space-y-6" onSubmit={(e) => e.preventDefault()}>
        <RAGFlowFormItem name="llm_name" label={tSetting('modelName')} required>
          <Input placeholder="somark-from-env-1" />
        </RAGFlowFormItem>

        <RAGFlowFormItem
          name="somark_base_url"
          label={tSetting('somark.baseUrl')}
          required
        >
          <Input placeholder={tSetting('somark.baseUrlPlaceholder')} />
        </RAGFlowFormItem>

        <RAGFlowFormItem
          name="somark_api_key"
          label={tSetting('somark.apiKey')}
        >
          <Input
            type="password"
            placeholder={tSetting('somark.apiKeyPlaceholder')}
          />
        </RAGFlowFormItem>

        <div className="text-sm font-semibold text-muted-foreground border-b pb-1">
          {tSetting('somark.sectionElementFormats')}
        </div>

        <RAGFlowFormItem
          name="somark_image_format"
          label={tSetting('somark.imageFormat')}
        >
          {(field) => (
            <RAGFlowSelect
              value={field.value}
              onChange={field.onChange}
              options={imageFormatOptions}
            />
          )}
        </RAGFlowFormItem>

        <RAGFlowFormItem
          name="somark_formula_format"
          label={tSetting('somark.formulaFormat')}
        >
          {(field) => (
            <RAGFlowSelect
              value={field.value}
              onChange={field.onChange}
              options={formulaFormatOptions}
            />
          )}
        </RAGFlowFormItem>

        <RAGFlowFormItem
          name="somark_table_format"
          label={tSetting('somark.tableFormat')}
        >
          {(field) => (
            <RAGFlowSelect
              value={field.value}
              onChange={field.onChange}
              options={tableFormatOptions}
            />
          )}
        </RAGFlowFormItem>

        <RAGFlowFormItem
          name="somark_cs_format"
          label={tSetting('somark.csFormat')}
        >
          {(field) => (
            <RAGFlowSelect
              value={field.value}
              onChange={field.onChange}
              options={csFormatOptions}
            />
          )}
        </RAGFlowFormItem>

        <div className="text-sm font-semibold text-muted-foreground border-b pb-1">
          {tSetting('somark.sectionFeatureConfig')}
        </div>

        <RAGFlowFormItem
          name="somark_enable_text_cross_page"
          label={tSetting('somark.enableTextCrossPage')}
          labelClassName="!mb-0"
        >
          {(field) => (
            <Switch checked={field.value} onCheckedChange={field.onChange} />
          )}
        </RAGFlowFormItem>

        <RAGFlowFormItem
          name="somark_enable_table_cross_page"
          label={tSetting('somark.enableTableCrossPage')}
          labelClassName="!mb-0"
        >
          {(field) => (
            <Switch checked={field.value} onCheckedChange={field.onChange} />
          )}
        </RAGFlowFormItem>

        <RAGFlowFormItem
          name="somark_enable_title_level_recognition"
          label={tSetting('somark.enableTitleLevelRecognition')}
          labelClassName="!mb-0"
        >
          {(field) => (
            <Switch checked={field.value} onCheckedChange={field.onChange} />
          )}
        </RAGFlowFormItem>

        <RAGFlowFormItem
          name="somark_enable_inline_image"
          label={tSetting('somark.enableInlineImage')}
          labelClassName="!mb-0"
        >
          {(field) => (
            <Switch checked={field.value} onCheckedChange={field.onChange} />
          )}
        </RAGFlowFormItem>

        <RAGFlowFormItem
          name="somark_enable_table_image"
          label={tSetting('somark.enableTableImage')}
          labelClassName="!mb-0"
        >
          {(field) => (
            <Switch checked={field.value} onCheckedChange={field.onChange} />
          )}
        </RAGFlowFormItem>

        <RAGFlowFormItem
          name="somark_enable_image_understanding"
          label={tSetting('somark.enableImageUnderstanding')}
          labelClassName="!mb-0"
        >
          {(field) => (
            <Switch checked={field.value} onCheckedChange={field.onChange} />
          )}
        </RAGFlowFormItem>

        <RAGFlowFormItem
          name="somark_keep_header_footer"
          label={tSetting('somark.keepHeaderFooter')}
          labelClassName="!mb-0"
        >
          {(field) => (
            <Switch checked={field.value} onCheckedChange={field.onChange} />
          )}
        </RAGFlowFormItem>
      </form>

      {/* VerifyButton lives inside <Form> (FormProvider) so its
          internal useFormContext() resolves the form instance.
          Rendered outside <form> so it never triggers submission. */}
      <div className="pt-3">
        <VerifyButton
          onVerify={handleVerify}
          isAbsolute={false}
          validLabel={tSetting('somark.verifyPassed')}
          invalidLabel={tSetting('somark.verifyFailed')}
        />
      </div>
    </Form>
  );

  return (
    <div
      className="border-b border-border-button mb-5 pb-5"
      data-testid={`instance-card-${instance.instance_name || 'draft'}`}
    >
      {isDraft ? (
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
                className="flex-1"
                data-testid="instance-name-input"
              />
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
          </div>

          {renderFields()}
        </div>
      ) : (
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
            <div className="px-2 pb-4 flex flex-col gap-4">
              {renderFields()}
            </div>
          </CollapsibleContent>
        </Collapsible>
      )}
    </div>
  );
});

export default SoMarkInstanceCard;

// Ensure the component is usable with the same props shape as the
// generic card (keeps the dispatch in provider-instance-card.tsx happy
// when forwarding props + ref).
export type { ProviderInstanceCardProps };
