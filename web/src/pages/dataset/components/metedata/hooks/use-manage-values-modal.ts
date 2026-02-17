import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  MetadataDeleteMap,
  MetadataType,
  metadataValueTypeEnum,
} from '../constant';
import { IManageValuesProps, IMetaDataTableData } from '../interface';

export const useManageValues = (props: IManageValuesProps) => {
  const {
    data,
    isAddValueMode,
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
    valueType: data.valueType || metadataValueTypeEnum.string,
    values: data.values || [''],
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

  const [shouldSave, setShouldSave] = useState(false);
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
    async (field: string, value: any) => {
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

          // const supportsEnum = isMetadataValueTypeWithEnum(nextValueType);
          // if (!supportsEnum) {
          //   setTempValues([]);
          // }
          return {
            ...prev,
            valueType: nextValueType,
            values: prev.values || [],
            restrictDefinedValues: prev.restrictDefinedValues,
          };
        }
        const newMetadata = {
          ...prev,
          [field]: value,
        };
        return newMetadata;
      });
      return true;
    },
    [existsKeys, type, t],
  );

  // Maintain separate state for each input box
  const [tempValues, setTempValues] = useState<string[]>(['']);

  useEffect(() => {
    setTempValues([...data.values]);
    setMetaData({
      ...data,
      valueType: data.valueType || metadataValueTypeEnum.string,
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
    // const supportsEnum = isMetadataValueTypeWithEnum(metaData.valueType);
    // if (!supportsEnum) {
    // onSave({
    //   ...metaData,
    //   values: [],
    //   restrictDefinedValues: false,
    // });
    // handleHideModal();
    // return;
    // }
    if (isAddValueMode) {
      addUpdateValue(
        metaData.field,
        undefined,
        metaData.values,
        metaData.valueType,
      );
    }
    // onSave(metaData);
    setShouldSave(true);
  }, [
    metaData,
    // onSave,
    // handleHideModal,
    type,
    valueError,
    isAddValueMode,
    addUpdateValue,
  ]);

  useEffect(() => {
    if (shouldSave) {
      const timer = setTimeout(() => {
        onSave(metaData);
        setShouldSave(false);
        clearTimeout(timer);
        handleHideModal();
      }, 100);
    }
  }, [shouldSave, onSave, handleHideModal, metaData]);

  // Handle blur event, synchronize to main state
  const handleValueBlur = useCallback(
    (values?: string[]) => {
      const newValues = values || tempValues;
      if (data.values.length > 0 && !isAddValueMode) {
        newValues.forEach((newValue, index) => {
          if (index < data.values.length) {
            const originalValue = data.values[index];
            if (originalValue !== newValue) {
              addUpdateValue(
                metaData.field,
                originalValue,
                newValue,
                metaData.valueType,
              );
            }
          } else {
            if (newValue) {
              addUpdateValue(metaData.field, '', newValue, metaData.valueType);
            }
          }
        });
      }
      handleChange('values', [...new Set([...newValues])]);
    },
    [handleChange, tempValues, metaData, data, addUpdateValue, isAddValueMode],
  );

  // Handle value changes, only update temporary state
  const handleValueChange = useCallback(
    (index: number, value: string, isUpdate: boolean = false) => {
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
        if (isUpdate) {
          handleValueBlur(newValues);
        }
        return newValues;
      });
    },
    [t, type, handleValueBlur],
  );

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

  const handleClearValues = useCallback(
    (isClearInitialValues = false) => {
      setTempValues(isClearInitialValues ? [] : ['']);
      setMetaData((prev) => ({
        ...prev,
        values: isClearInitialValues ? [] : [''],
      }));
    },
    [setTempValues, setMetaData],
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
    handleClearValues,
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
