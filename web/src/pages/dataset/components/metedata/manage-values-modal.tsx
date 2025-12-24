import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import EditTag from '@/components/edit-tag';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Modal } from '@/components/ui/modal/modal';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { Plus, Trash2 } from 'lucide-react';
import { memo, useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { MetadataType } from './hook';
import { IManageValuesProps, IMetaDataTableData } from './interface';

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
    data,
    isEditField,
    visible,
    isAddValue,
    isShowDescription,
    isShowValueSwitch,
    isVerticalShowValue,
    hideModal,
    onSave,
    addUpdateValue,
    addDeleteValue,
    existsKeys,
    type,
  } = props;
  const [metaData, setMetaData] = useState(data);
  const { t } = useTranslation();
  const [valueError, setValueError] = useState<Record<string, string>>({
    field: '',
    values: '',
  });
  const [deleteDialogContent, setDeleteDialogContent] = useState({
    visible: false,
    title: '',
    name: '',
    warnText: '',
    onOk: () => {},
    onCancel: () => {},
  });
  const hideDeleteModal = () => {
    setDeleteDialogContent({
      visible: false,
      title: '',
      name: '',
      warnText: '',
      onOk: () => {},
      onCancel: () => {},
    });
  };

  // Use functional update to avoid closure issues
  const handleChange = useCallback(
    (field: string, value: any) => {
      if (field === 'field' && existsKeys.includes(value)) {
        setValueError((prev) => {
          return {
            ...prev,
            field:
              type === MetadataType.Setting
                ? t('knowledgeDetails.metadata.fieldExists')
                : t('knowledgeDetails.metadata.fieldNameExists'),
          };
        });
      } else if (field === 'field' && !existsKeys.includes(value)) {
        setValueError((prev) => {
          return {
            ...prev,
            field: '',
          };
        });
      }
      setMetaData((prev) => ({
        ...prev,
        [field]: value,
      }));
    },
    [existsKeys, type, t],
  );

  // Maintain separate state for each input box
  const [tempValues, setTempValues] = useState<string[]>([...data.values]);

  useEffect(() => {
    setTempValues([...data.values]);
    setMetaData(data);
  }, [data]);

  const handleHideModal = useCallback(() => {
    hideModal();
    setMetaData({} as IMetaDataTableData);
  }, [hideModal]);

  const handleSave = useCallback(() => {
    if (type === MetadataType.Setting && valueError.field) {
      return;
    }
    if (!metaData.restrictDefinedValues && isShowValueSwitch) {
      const newMetaData = { ...metaData, values: [] };
      onSave(newMetaData);
    } else {
      onSave(metaData);
    }
    handleHideModal();
  }, [metaData, onSave, handleHideModal, isShowValueSwitch, type, valueError]);

  // Handle value changes, only update temporary state
  const handleValueChange = useCallback(
    (index: number, value: string) => {
      setTempValues((prev) => {
        if (prev.includes(value)) {
          setValueError((prev) => {
            return {
              ...prev,
              values: t('knowledgeDetails.metadata.valueExists'),
            };
          });
        } else {
          setValueError((prev) => {
            return {
              ...prev,
              values: '',
            };
          });
        }
        const newValues = [...prev];
        newValues[index] = value;

        return newValues;
      });
    },
    [t],
  );

  // Handle blur event, synchronize to main state
  const handleValueBlur = useCallback(() => {
    addUpdateValue(metaData.field, [...new Set([...tempValues])]);
    handleChange('values', [...new Set([...tempValues])]);
  }, [handleChange, tempValues, metaData, addUpdateValue]);

  // Handle delete operation
  const handleDelete = useCallback(
    (index: number) => {
      setTempValues((prev) => {
        const newTempValues = [...prev];
        addDeleteValue(metaData.field, newTempValues[index]);
        newTempValues.splice(index, 1);
        return newTempValues;
      });

      // Synchronize to main state
      setMetaData((prev) => {
        const newMetaDataValues = [...prev.values];
        newMetaDataValues.splice(index, 1);
        return {
          ...prev,
          values: newMetaDataValues,
        };
      });
    },
    [addDeleteValue, metaData],
  );

  const showDeleteModal = (item: string, callback: () => void) => {
    setDeleteDialogContent({
      visible: true,
      title: t('common.delete') + ' ' + t('knowledgeDetails.metadata.metadata'),
      name: metaData.field + '/' + item,
      warnText: t('knowledgeDetails.metadata.deleteWarn', {
        field:
          t('knowledgeDetails.metadata.field') +
          '/' +
          t('knowledgeDetails.metadata.values'),
      }),
      onOk: () => {
        hideDeleteModal();
        callback();
      },
      onCancel: () => {
        hideDeleteModal();
      },
    });
  };

  // Handle adding new value
  const handleAddValue = useCallback(() => {
    setTempValues((prev) => [...new Set([...prev, ''])]);

    // Synchronize to main state
    setMetaData((prev) => ({
      ...prev,
      values: [...new Set([...prev.values, ''])],
    }));
  }, []);

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
        {isShowDescription && (
          <div className="flex flex-col gap-2">
            <div>{t('knowledgeDetails.metadata.description')}</div>
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
        {isShowValueSwitch && (
          <div className="flex flex-col gap-2">
            <div>{t('knowledgeDetails.metadata.restrictDefinedValues')}</div>
            <div>
              <Switch
                checked={metaData.restrictDefinedValues || false}
                onCheckedChange={(checked) =>
                  handleChange('restrictDefinedValues', checked)
                }
              />
            </div>
          </div>
        )}
        {((metaData.restrictDefinedValues && isShowValueSwitch) ||
          !isShowValueSwitch) && (
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
