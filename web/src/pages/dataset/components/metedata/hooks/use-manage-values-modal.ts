import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  isMetadataValueTypeWithEnum,
  MetadataDeleteMap,
  MetadataType,
} from '../hooks/use-manage-modal';
import { IManageValuesProps, IMetaDataTableData } from '../interface';

export const useManageValues = (props: IManageValuesProps) => {
  const {
    data,

    hideModal,
    onSave,
    addUpdateValue,
    addDeleteValue,
    existsKeys,
    type,
  } = props;
  const { t } = useTranslation();
  const [metaData, setMetaData] = useState<IMetaDataTableData>({
    ...data,
    valueType: data.valueType || 'string',
  });
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
            field: MetadataDeleteMap(t)[type as MetadataType].warnFieldName,
            // type === MetadataType.Setting
            //   ? t('knowledgeDetails.metadata.fieldExists')
            //   : t('knowledgeDetails.metadata.fieldNameExists'),
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
      setMetaData((prev) => {
        if (field === 'valueType') {
          const nextValueType = (value ||
            'string') as IMetaDataTableData['valueType'];
          const supportsEnum = isMetadataValueTypeWithEnum(nextValueType);
          if (!supportsEnum) {
            setTempValues([]);
          }
          return {
            ...prev,
            valueType: nextValueType,
            values: supportsEnum ? prev.values : [],
            restrictDefinedValues: supportsEnum
              ? prev.restrictDefinedValues || nextValueType === 'enum'
              : false,
          };
        }
        return {
          ...prev,
          [field]: value,
        };
      });
    },
    [existsKeys, type, t],
  );

  // Maintain separate state for each input box
  const [tempValues, setTempValues] = useState<string[]>([...data.values]);

  useEffect(() => {
    setTempValues([...data.values]);
    setMetaData({
      ...data,
      valueType: data.valueType || 'string',
    });
  }, [data]);

  const handleHideModal = useCallback(() => {
    hideModal();
    setMetaData({} as IMetaDataTableData);
  }, [hideModal]);

  const handleSave = useCallback(() => {
    if (type === MetadataType.Setting && valueError.field) {
      return;
    }
    const supportsEnum = isMetadataValueTypeWithEnum(metaData.valueType);
    if (!supportsEnum) {
      onSave({
        ...metaData,
        values: [],
        restrictDefinedValues: false,
      });
      handleHideModal();
      return;
    }
    onSave(metaData);
    handleHideModal();
  }, [metaData, onSave, handleHideModal, type, valueError]);

  // Handle value changes, only update temporary state
  const handleValueChange = useCallback(
    (index: number, value: string) => {
      setTempValues((prev) => {
        if (prev.includes(value)) {
          setValueError((prev) => {
            return {
              ...prev,
              values: MetadataDeleteMap(t)[type as MetadataType].warnValueName,
              // t('knowledgeDetails.metadata.valueExists'),
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
    [t, type],
  );

  // Handle blur event, synchronize to main state
  const handleValueBlur = useCallback(() => {
    if (data.values.length > 0) {
      tempValues.forEach((newValue, index) => {
        if (index < data.values.length) {
          const originalValue = data.values[index];
          if (originalValue !== newValue) {
            addUpdateValue(metaData.field, originalValue, newValue);
          }
        } else {
          if (newValue) {
            addUpdateValue(metaData.field, '', newValue);
          }
        }
      });
    }
    handleChange('values', [...new Set([...tempValues])]);
  }, [handleChange, tempValues, metaData, data, addUpdateValue]);

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
      title: t('common.delete') + ' ' + t('knowledgeDetails.metadata.value'),
      name: item,
      warnText: MetadataDeleteMap(t)[type as MetadataType].warnValueText,
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

  return {
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
  };
};
