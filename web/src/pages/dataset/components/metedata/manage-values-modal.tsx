import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import { DynamicForm, FormFieldType } from '@/components/dynamic-form';
import EditTag from '@/components/edit-tag';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { DateInput } from '@/components/ui/input-date';
import { Modal } from '@/components/ui/modal/modal';
import { formatDate } from '@/utils/date';
import dayjs from 'dayjs';
import { Plus, Trash2 } from 'lucide-react';
import { memo, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  MetadataType,
  metadataValueTypeEnum,
  metadataValueTypeOptions,
} from './constant';
import { useManageValues } from './hooks/use-manage-values-modal';
import { IManageValuesProps, MetadataValueType } from './interface';

// Create a separate input component, wrapped with memo to avoid unnecessary re-renders
const ValueInputItem = memo(
  ({
    item,
    index,
    type,
    onValueChange,
    onDelete,
    onBlur,
    isCanDelete = true,
  }: {
    item: string;
    index: number;
    type: MetadataValueType;
    onValueChange: (index: number, value: string, isUpdate?: boolean) => void;
    onDelete: (index: number) => void;
    onBlur: (index: number) => void;
    isCanDelete?: boolean;
  }) => {
    const value = useMemo(() => {
      if (type === 'time') {
        if (item) {
          try {
            // Using dayjs to parse date strings in various formats including DD/MM/YYYY
            const parsedDate = dayjs(item, [
              'YYYY-MM-DD HH:mm:ss',
              'DD/MM/YYYY HH:mm:ss',
              'YYYY-MM-DD',
              'DD/MM/YYYY',
            ]);

            if (!parsedDate.isValid()) {
              console.error('Invalid date format:', item);
              return undefined; // Return current date as fallback
            }
            return parsedDate.toDate();
          } catch (error) {
            console.error('Error parsing date:', item, error);
            return undefined; // Return current date as fallback
          }
        }
        return undefined;
      }
      return item;
    }, [item, type]);

    return (
      <div
        key={`value-item-${index}`}
        className="flex items-center gap-2.5 w-full"
      >
        <div className="flex-1 w-full">
          {type === 'time' && (
            <DateInput
              value={value as Date}
              onChange={(value) => {
                onValueChange(
                  index,
                  formatDate(value, 'YYYY-MM-DDTHH:mm:ss'),
                  true,
                );
              }}
              showTimeSelect={true}
            />
          )}
          {type !== 'time' && (
            <Input
              value={value as string}
              type={type === 'number' ? 'number' : 'text'}
              onChange={(e) => onValueChange(index, e.target.value)}
              onBlur={() => onBlur(index)}
            />
          )}
        </div>
        {isCanDelete && (
          <Button
            type="button"
            variant="delete"
            className="border border-border-button px-1 h-6 w-6 rounded-sm"
            onClick={() => onDelete(index)}
          >
            <Trash2 size={14} className="w-4 h-4" />
          </Button>
        )}
      </div>
    );
  },
);

ValueInputItem.displayName = 'ValueInputItem';

export const ManageValuesModal = (props: IManageValuesProps) => {
  const {
    title,
    isEditField,
    visible,
    isAddValue,
    isShowValueSwitch,
    isShowDescription,
    isVerticalShowValue,
    isShowType,
    type: metadataType,
  } = props;
  const {
    metaData,
    tempValues,
    valueError,
    deleteDialogContent,
    handleClearValues,
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

  const formRef = useRef<any>();

  const [valueType, setValueType] = useState<MetadataValueType>(
    metaData.valueType || 'string',
  );

  // Define form fields based on component properties
  const formFields = [
    ...(isEditField
      ? [
          {
            name: 'field',
            label: t('knowledgeDetails.metadata.fieldName'),
            type: FormFieldType.Text,
            required: true,
            validation: {
              pattern: /^[a-zA-Z_]*$/,
              message: t('knowledgeDetails.metadata.fieldNameInvalid'),
            },
            defaultValue: metaData.field,
            onChange: (value: string) => handleChange('field', value),
          },
        ]
      : []),
    ...(isShowType
      ? [
          {
            name: 'valueType',
            label: 'Type',
            type: FormFieldType.Select,
            options: metadataValueTypeOptions,
            defaultValue: metaData.valueType || metadataValueTypeEnum.string,
            onChange: (value: string) => {
              setValueType(value as MetadataValueType);
              handleChange('valueType', value);
              if (
                metadataType === MetadataType.Manage ||
                metadataType === MetadataType.UpdateSingle
              ) {
                handleClearValues();
              }

              if (
                metadataType === MetadataType.Setting ||
                metadataType === MetadataType.SingleFileSetting
              ) {
                if (
                  value !== metadataValueTypeEnum.list &&
                  value !== metadataValueTypeEnum.string
                ) {
                  handleChange('restrictDefinedValues', false);
                  handleClearValues(true);
                  formRef.current?.form.setValue(
                    'restrictDefinedValues',
                    false,
                  );
                }
              }
            },
          },
        ]
      : []),
    ...(isShowDescription
      ? [
          {
            name: 'description',
            label: t('knowledgeDetails.metadata.description'),
            type: FormFieldType.Textarea,
            tooltip: t('knowledgeDetails.metadata.descriptionTip'),
            defaultValue: metaData.description,
            className: 'mt-2',
            onChange: (value: string) => handleChange('description', value),
          },
        ]
      : []),
    ...(isShowValueSwitch
      ? [
          {
            name: 'restrictDefinedValues',
            label: t('knowledgeDetails.metadata.restrictDefinedValues'),
            tooltip: t('knowledgeDetails.metadata.restrictDefinedValuesTip'),
            type: FormFieldType.Switch,
            defaultValue: metaData.restrictDefinedValues || false,
            shouldRender: (formData: any) => {
              return (
                formData.valueType === 'list' || formData.valueType === 'string'
              );
            },
            onChange: (value: boolean) =>
              handleChange('restrictDefinedValues', value),
          },
        ]
      : []),
  ];

  // Handle form submission
  const handleSubmit = () => {
    handleSave();
  };

  return (
    <Modal
      title={title}
      open={visible}
      onCancel={handleHideModal}
      className="!w-[460px]"
      okText={t('common.confirm')}
      onOk={() => formRef.current?.submit(handleSubmit)}
      maskClosable={false}
      footer={null}
    >
      <div className="flex flex-col gap-4">
        {!isEditField && (
          <div className="text-base p-5 border border-border-button rounded-lg">
            {metaData.field}
          </div>
        )}

        {formFields.length > 0 && (
          <DynamicForm.Root
            ref={formRef}
            fields={formFields}
            onSubmit={handleSubmit}
            className="space-y-4"
          />
        )}

        {((metaData.restrictDefinedValues && isShowValueSwitch) ||
          !isShowValueSwitch) && (
          <div className="flex flex-col gap-2">
            <div className="flex justify-between items-center">
              <div>{t('knowledgeDetails.metadata.values')}</div>
              {isAddValue &&
                isVerticalShowValue &&
                metaData.valueType === metadataValueTypeEnum['list'] && (
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
                      type={valueType || 'string'}
                      onValueChange={handleValueChange}
                      onDelete={(idx: number) => {
                        showDeleteModal(item, () => {
                          handleDelete(idx);
                        });
                      }}
                      isCanDelete={tempValues.length > 1}
                      onBlur={() => handleValueBlur()}
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
