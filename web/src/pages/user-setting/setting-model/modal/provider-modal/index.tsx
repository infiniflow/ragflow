import { DynamicForm, DynamicFormRef } from '@/components/dynamic-form';
import { Modal } from '@/components/ui/modal/modal';
import { useCommonTranslation, useTranslate } from '@/hooks/common-hooks';
import { useAddInstanceModel } from '@/hooks/use-llm-request';
import { IProviderModelItem } from '@/interfaces/request/llm';
import { cn } from '@/lib/utils';
import {
  useFetchInstanceNameSet,
  useHideWhenInstanceExists,
  VerifyResult,
} from '@/pages/user-setting/setting-model/hooks';
import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { FieldValues } from 'react-hook-form';
import { LLMHeader } from '../../components/llm-header';
import VerifyButton from '../verify-button';
import { AddableToggleList } from './components/addable-toggle-list';
import { useCustomModelFields } from './components/use-custom-model-fields';
import {
  useListModelsOptions,
  useListModelsPicker,
  useProviderFields,
  useProviderModalActions,
} from './hooks';
import type { ProviderModalProps } from './types';

const ProviderModal = ({
  visible,
  hideModal,
  llmFactory,
  loading,
  editMode,
  viewMode,
  initialValues,
  baseUrlOptions,
  onOk,
  onVerify,
  onViewModeOk,
}: ProviderModalProps) => {
  const { t } = useTranslate('setting');
  const { t: tc } = useCommonTranslation();
  const formRef = useRef<DynamicFormRef>(null);
  const { instanceNameSet } = useFetchInstanceNameSet(llmFactory);
  const hideWhenInstanceExists = useHideWhenInstanceExists(instanceNameSet);
  const [verifyResult, setVerifyResult] = useState<VerifyResult | null>(null);
  const scrollAnchorRef = useRef<HTMLDivElement>(null);
  useEffect(() => {
    setVerifyResult(null);
    return () => {
      setVerifyResult(null);
    };
  }, [visible]);

  // When a verify result comes back, the VerifyButton renders new log
  // content below the existing form. Scroll the modal's scrollable area
  // to the bottom so the user actually sees the result. We walk up the
  // DOM from a ref inside the scrollable container (the Modal renders
  // it via a Radix Portal) and use rAF to wait for the new content to
  // be laid out before measuring scrollHeight.
  useEffect(() => {
    if (!verifyResult || !scrollAnchorRef.current) {
      return;
    }
    const scrollContainer =
      scrollAnchorRef.current.closest<HTMLElement>('.overflow-y-auto');
    if (!scrollContainer) {
      return;
    }
    requestAnimationFrame(() => {
      scrollContainer.scrollTop = scrollContainer.scrollHeight;
    });
  }, [verifyResult]);

  // Field config, default values, doc link, and the LIST_MODEL_PROVIDERS
  // flag are all derived from the current llmFactory / mode / initialValues.
  // `baseUrlRegionMaps` is forwarded to the actions hook so the modal can
  // populate the `region` submit field from the currently selected base URL.
  const {
    config,
    fields,
    defaultValues,
    docLinkText,
    hasModelNameField,
    baseUrlRegionMaps,
  } = useProviderFields({
    llmFactory,
    editMode,
    viewMode,
    initialValues,
    baseUrlOptions,
    hideWhenInstanceExists,
  });

  // Owns the "List Models" picker state and lifecycle. When
  // `hasModelNameField` is false the picker is hidden and this hook is
  // effectively idle (no fetch, no selection state in use).
  const {
    models,
    listLoading,
    selectedModelItems,
    modelInfoList,
    allSelected,
    handleListOpenChange,
    handleSelectModel,
    handleToggleAll,
    setModels,
  } = useListModelsPicker({
    visible,
    hasModelNameField,
    editMode,
    viewMode,
    initialValues,
    llmFactory,
    config,
    formRef,
  });

  // Mutation for adding a model to an existing instance (viewMode path)
  const { addInstanceModel } = useAddInstanceModel();

  // Dialog field schema for adding a custom model. Derived from
  // `IProviderModelItem` (the shape of items in `listModelsOptions`),
  // so the form automatically tracks the model interface. The hook lives
  // next to the dialog so the schema is the single source of truth.
  const customModelDialogFields = useCustomModelFields();

  // Get existing model names for uniqueness validation
  const existingNames = useMemo(() => models.map((m) => m.name), [models]);

  // Handle adding a custom model
  // - In viewMode, call the API to persist the model on the existing instance.
  // - Always update the local `models` catalog so the new option is visible.
  // - Selection is owned by `AddableToggleList` (it calls `handleSelectModel`
  //   after `onAdd` resolves). Do NOT also push into `selectedModelItems`
  //   here — that would race with the wrapper's toggle and the new option
  //   would be inserted then immediately removed.
  const handleAddCustomModel = useCallback(
    async (item: IProviderModelItem) => {
      if (viewMode && initialValues?.instance_name) {
        await addInstanceModel({
          provider_name: llmFactory,
          instance_name: initialValues.instance_name,
          model_name: item.name,
          model_type: item.model_types,
          max_tokens: item.max_tokens,
          extra: item.features
            ? {
                is_tools: item.features.includes('is_tools'),
              }
            : undefined,
        });
      }
      setModels((prev) =>
        prev.some((m) => m.name === item.name) ? prev : [...prev, item],
      );
    },
    [viewMode, initialValues, llmFactory, addInstanceModel, setModels],
  );

  // Render-only: turn the fetched model list into ToggleList options with
  // the "All models" sentinel row at the top.
  const listModelsOptions = useListModelsOptions({
    models,
    selectedModelItems,
    allSelected,
    handleSelectModel,
    handleToggleAll,
  });

  // Submit and verify handlers — branch on viewMode and on whether the
  // picker owns the model fields.
  const { handleVerify, handleSubmit } = useProviderModalActions({
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
  });

  return (
    <Modal
      title={<LLMHeader name={llmFactory} />}
      open={visible || false}
      onOpenChange={(open) => !open && hideModal?.()}
      maskClosable={false}
      footer={<div className="p-4"></div>}
    >
      <DynamicForm.Root
        key={`${visible}-${llmFactory}`}
        fields={fields}
        onSubmit={() => {
          // The actual submission is handled by SavingButton
        }}
        ref={formRef}
        defaultValues={defaultValues}
        labelClassName="font-normal"
      >
        {hasModelNameField && (
          <AddableToggleList
            className="w-full"
            buttonClassName="self-end"
            searchable={listModelsOptions.length > 10}
            btnText={t('listModels')}
            options={listModelsOptions}
            searchPlaceholder={t('listModelsSearchPlaceholder')}
            emptyText={t('listModelsEmpty')}
            searchLoading={listLoading}
            onOpenChange={handleListOpenChange}
            maxHeight={400}
            dialogTitle={t('addCustomModelTitle')}
            dialogFields={customModelDialogFields}
            dialogSubmitText={tc('confirm')}
            dialogCancelText={tc('cancel')}
            onAdd={handleAddCustomModel}
            handleSelectModel={handleSelectModel}
            existingNames={existingNames}
          />
        )}

        <div ref={scrollAnchorRef}>
          <VerifyButton
            onVerify={handleVerify}
            verifyCallback={(result: VerifyResult | null) => {
              setVerifyResult(result);
            }}
            className={cn({
              '!flex flex-col ![position:inherit] ':
                verifyResult && docLinkText && config.docLink,
            })}
          />
        </div>
        <div
          className={
            docLinkText
              ? 'absolute bottom-0 right-0 left-0 flex items-center justify-between w-full py-6 px-6'
              : 'absolute bottom-0 right-0 left-0 flex items-center justify-end w-full gap-2 py-6 px-6'
          }
        >
          {docLinkText && config.docLink && (
            <a
              href={config.docLink}
              target="_blank"
              rel="noreferrer"
              className={cn('text-primary hover:underline', {
                'ml-24': !verifyResult,
              })}
            >
              {docLinkText}
            </a>
          )}

          <div className="flex gap-2">
            <DynamicForm.CancelButton
              handleCancel={() => {
                hideModal?.();
              }}
            />
            <DynamicForm.SavingButton
              submitLoading={loading || false}
              buttonText={tc('ok')}
              submitFunc={(values: FieldValues) => {
                handleSubmit(values);
              }}
            />
          </div>
        </div>
      </DynamicForm.Root>
    </Modal>
  );
};

export default memo(ProviderModal);

// Export field configurations (for use by other modules)
export { FACTORIES_WITH_BASE_URL, getProviderConfig } from './field-config';
export type {
  FieldConfig,
  IViewModeOkPayload,
  ProviderConfig,
  ProviderModalProps,
  ShouldRenderToken,
} from './types';
