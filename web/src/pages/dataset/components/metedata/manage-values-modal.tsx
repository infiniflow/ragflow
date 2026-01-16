import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import EditTag from '@/components/edit-tag';
import { Button } from '@/components/ui/button';
import { FormLabel } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Modal } from '@/components/ui/modal/modal';
import { RAGFlowSelect } from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { Plus, Trash2 } from 'lucide-react';
import { memo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  isMetadataValueTypeWithEnum,
  metadataValueTypeOptions,
} from './hooks/use-manage-modal';
import { useManageValues } from './hooks/use-manage-values-modal';
import { IManageValuesProps } from './interface';

// Create a separate input component, wrapped with memo to avoid unnecessary re-renders
const ValueInputItem = memo(
  ({
    item,
    index,
    onValueChange,
    onDelete,
    onBlur,
  }: {
    item: string;
    index: number;
    onValueChange: (index: number, value: string) => void;
    onDelete: (index: number) => void;
    onBlur: (index: number) => void;
  }) => {
    return (
      <div
        key={`value-item-${index}`}
        className="flex items-center gap-2.5 w-full"
      >
        <div className="flex-1 w-full">
          <Input
            value={item}
            onChange={(e) => onValueChange(index, e.target.value)}
            onBlur={() => onBlur(index)}
          />
        </div>
        <Button
          type="button"
          variant="delete"
          className="border border-border-button px-1 h-6 w-6 rounded-sm"
          onClick={() => onDelete(index)}
        >
          <Trash2 size={14} className="w-4 h-4" />
        </Button>
      </div>
    );
  },
);

export const ManageValuesModal = (props: IManageValuesProps) => {
  const {
    title,
    isEditField,
    visible,
    isAddValue,
    isShowDescription,
    isVerticalShowValue,
    isShowType,
  } = props;
  const {
    metaData,
    tempValues,
    valueError,
    deleteDialogContent,
    handleChange,
    handleValueChange,
    handleValueBlur,
    handleDelete,
    handleAddValue,
    showDeleteModal,
    handleSave,
    handleHideModal,
  } = useManageValues(props);
  const { t } = useTranslation();
  const canShowValues = isMetadataValueTypeWithEnum(metaData.valueType);

  return (
    <Modal
      title={title}
      open={visible}
      onCancel={handleHideModal}
      className="!w-[460px]"
      okText={t('common.confirm')}
      onOk={handleSave}
      maskClosable={false}
      footer={null}
    >
      <div className="flex flex-col gap-4">
        {!isEditField && (
          <div className="text-base p-5 border border-border-button rounded-lg">
            {metaData.field}
          </div>
        )}
        {isEditField && (
          <div className="flex flex-col gap-2">
            <div>{t('knowledgeDetails.metadata.fieldName')}</div>
            <div>
              <Input
                value={metaData.field}
                onChange={(e) => {
                  const value = e.target?.value || '';
                  if (/^[a-zA-Z_]*$/.test(value)) {
                    handleChange('field', value);
                  }
                }}
              />
              <div className="text-state-error text-sm">{valueError.field}</div>
            </div>
          </div>
        )}
        {isShowType && (
          <div className="flex flex-col gap-2">
            <div>Type</div>
            <RAGFlowSelect
              value={metaData.valueType || 'string'}
              options={metadataValueTypeOptions}
              onChange={(value) => handleChange('valueType', value)}
            />
          </div>
        )}
        {isShowDescription && (
          <div className="flex flex-col gap-2">
            <FormLabel
              className="text-text-primary text-base"
              tooltip={t('knowledgeDetails.metadata.descriptionTip')}
            >
              {t('knowledgeDetails.metadata.description')}
            </FormLabel>
            <div>
              <Textarea
                value={metaData.description}
                onChange={(e) => {
                  handleChange('description', e.target?.value || '');
                }}
              />
            </div>
          </div>
        )}
        {canShowValues && (
          <div className="flex flex-col gap-2">
            <div className="flex justify-between items-center">
              <div>{t('knowledgeDetails.metadata.values')}</div>
              {isAddValue && isVerticalShowValue && (
                <div>
                  <Button
                    variant={'ghost'}
                    className="border border-border-button"
                    onClick={handleAddValue}
                  >
                    <Plus />
                  </Button>
                </div>
              )}
            </div>
            {isVerticalShowValue && (
              <div className="flex flex-col gap-2 w-full">
                {tempValues?.map((item, index) => {
                  return (
                    <ValueInputItem
                      key={`value-item-${index}`}
                      item={item}
                      index={index}
                      onValueChange={handleValueChange}
                      onDelete={(idx: number) => {
                        showDeleteModal(item, () => {
                          handleDelete(idx);
                        });
                      }}
                      onBlur={handleValueBlur}
                    />
                  );
                })}
              </div>
            )}
            {!isVerticalShowValue && (
              <EditTag
                value={metaData.values}
                onChange={(value) => {
                  // find deleted value
                  const item = metaData.values.find(
                    (item) => !value.includes(item),
                  );
                  if (item) {
                    showDeleteModal(item, () => {
                      // handleDelete(idx);
                      handleChange('values', value);
                    });
                  } else {
                    handleChange('values', value);
                  }
                }}
              />
            )}
            <div className="text-state-error text-sm">{valueError.values}</div>
          </div>
        )}
        {deleteDialogContent.visible && (
          <ConfirmDeleteDialog
            open={deleteDialogContent.visible}
            onCancel={deleteDialogContent.onCancel}
            onOk={deleteDialogContent.onOk}
            title={deleteDialogContent.title}
            content={{
              node: (
                <ConfirmDeleteDialogNode
                  name={deleteDialogContent.name}
                  warnText={deleteDialogContent.warnText}
                />
              ),
            }}
          />
        )}
      </div>
    </Modal>
  );
};
