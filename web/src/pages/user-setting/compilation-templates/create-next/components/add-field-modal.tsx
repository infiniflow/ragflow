import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Modal } from '@/components/ui/modal/modal';
import { Textarea } from '@/components/ui/textarea';
import { ICompilationTemplateSection } from '@/interfaces/database/compilation-template';
import { startCase } from 'lodash';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';

import { FieldLabelKeyMap } from '@/pages/user-setting/compilation-templates/create-next/utils';

import { useAddFieldForm } from '../hooks/use-add-field-form';

type AddFieldModalProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sectionName: string;
  builtinSection?: ICompilationTemplateSection;
  initialField?: Record<string, string>;
  onAdd: (field: Record<string, string>) => void;
};

export function AddFieldModal({
  open,
  onOpenChange,
  sectionName,
  builtinSection,
  initialField,
  onAdd,
}: AddFieldModalProps) {
  const { t } = useTranslation();
  const {
    form,
    fieldKeys,
    hasTypeField,
    typeOptions,
    handleTypeChange,
    handleSubmit,
  } = useAddFieldForm({
    open,
    builtinSection,
    initialField,
  });

  const nonTypeKeys = fieldKeys.filter((key) => key !== 'type');

  const handleClose = useCallback(() => {
    onOpenChange(false);
  }, [onOpenChange]);

  const handleConfirm = useCallback(
    (field: Record<string, string>) => {
      onAdd(field);
      onOpenChange(false);
    },
    [onAdd, onOpenChange],
  );

  const getFieldLabel = useCallback(
    (key: string) => {
      return FieldLabelKeyMap[key] ? t(FieldLabelKeyMap[key]) : startCase(key);
    },
    [t],
  );

  return (
    <Modal
      open={open}
      onOpenChange={onOpenChange}
      title={`${initialField ? t('setting.editFieldModalTitle') : t('setting.addFieldModalTitle')} - ${startCase(sectionName)}`}
      size="default"
      footer={
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={handleClose}>
            {t('common.cancel')}
          </Button>
          <Button type="button" onClick={handleSubmit(handleConfirm)}>
            {t('common.confirm')}
          </Button>
        </div>
      }
    >
      <Form {...form}>
        <div className="space-y-4">
          {hasTypeField && (
            <RAGFlowFormItem name="type" label={getFieldLabel('type')}>
              {(field) => (
                <SelectWithSearch
                  {...field}
                  options={typeOptions}
                  allowClear
                  onChange={(value) => {
                    field.onChange(value);
                    handleTypeChange(value);
                  }}
                  placeholder={t('setting.selectFieldType')}
                  allowCustomValue
                />
              )}
            </RAGFlowFormItem>
          )}

          {nonTypeKeys.map((key) => (
            <RAGFlowFormItem key={key} name={key} label={getFieldLabel(key)}>
              <Textarea
                placeholder={t('setting.descriptionPlaceholder')}
                rows={key === 'description' ? 4 : 10}
                resize="vertical"
              />
            </RAGFlowFormItem>
          ))}
        </div>
      </Form>
    </Modal>
  );
}
