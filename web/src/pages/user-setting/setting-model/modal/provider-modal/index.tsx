import { DynamicForm, DynamicFormRef } from '@/components/dynamic-form';
import { Modal } from '@/components/ui/modal/modal';
import { ToggleList } from '@/components/ui/toggle-list';
import { useCommonTranslation, useTranslate } from '@/hooks/common-hooks';
import {
  useFetchInstanceNameSet,
  useHideWhenInstanceExists,
} from '@/pages/user-setting/setting-model/hooks';
import { memo, useRef } from 'react';
import { FieldValues } from 'react-hook-form';
import { LLMHeader } from '../../components/llm-header';
import VerifyButton from '../verify-button';
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
          <ToggleList
            className="w-full"
            buttonClassName="self-end"
            // searchable
            btnText={hasModelNameField ? t('listModels') : 'Select an option'}
            options={
              hasModelNameField
                ? listModelsOptions
                : listModelsOptions.length > 0
                  ? listModelsOptions
                  : []
            }
            searchPlaceholder={t('listModelsSearchPlaceholder')}
            emptyText={t('listModelsEmpty')}
            searchLoading={listLoading}
            onOpenChange={handleListOpenChange}
            maxHeight={400}
            closeOnOutsideClick
          />
        )}

        <VerifyButton onVerify={handleVerify} />

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
              className="text-primary hover:underline ml-24"
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
