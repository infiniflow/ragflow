import message from '@/components/ui/message';
import { useSetModalState } from '@/hooks/common-hooks';
import { useSelectedIds } from '@/hooks/logic-hooks/use-row-selection';
import { DocumentApiAction } from '@/hooks/use-document-request';
import kbService, {
  getMetaDataService,
  updateMetaData,
} from '@/services/knowledge-service';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { RowSelectionState } from '@tanstack/react-table';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import {
  DEFAULT_VALUE_TYPE,
  MetadataType,
  metadataValueTypeEnum,
} from '../constant';
import {
  IBuiltInMetadataItem,
  IMetaDataReturnJSONSettings,
  IMetaDataReturnJSONType,
  IMetaDataReturnType,
  IMetaDataTableData,
  MetadataOperations,
  MetadataValueType,
  ShowManageMetadataModalProps,
  UpdateOperation,
} from '../interface';

export const util = {
  changeToMetaDataTableData(data: IMetaDataReturnType): IMetaDataTableData[] {
    const res = Object.entries(data).map(
      ([key, value]: [
        string,
        (
          | { type: string; values: Array<Array<string | number>> }
          | Array<Array<string | number>>
        ),
      ]) => {
        if (Array.isArray(value)) {
          const values = value.map(([v]) => v.toString());
          return {
            field: key,
            valueType: DEFAULT_VALUE_TYPE,
            description: '',
            values: values,
          } as IMetaDataTableData;
        }
        const { type, values } = value;
        const valueArr = values.map(([v]) => v.toString());
        return {
          field: key,
          valueType: type,
          description: '',
          values: valueArr,
        } as IMetaDataTableData;
      },
    );
    return res;
  },

  JSONToMetaDataTableData(
    data: Record<string, string | string[]>,
  ): IMetaDataTableData[] {
    return Object.entries(data).map(([key, value]) => {
      let thisValue = [] as string[];
      if (value && Array.isArray(value)) {
        thisValue = value;
      } else if (value && typeof value === 'string') {
        thisValue = [value];
      } else if (value && typeof value === 'object') {
        thisValue = [JSON.stringify(value)];
      } else if (value) {
        thisValue = [value.toString()];
      }

      return {
        field: key,
        description: '',
        values: thisValue,
      } as IMetaDataTableData;
    });
  },

  tableDataToMetaDataJSON(data: IMetaDataTableData[]): IMetaDataReturnJSONType {
    return data.reduce<IMetaDataReturnJSONType>((pre, cur) => {
      pre[cur.field] = cur.values;
      return pre;
    }, {});
  },

  tableDataToMetaDataSettingJSON(
    data: IMetaDataTableData[],
  ): IMetaDataReturnJSONSettings {
    return data.map((item) => {
      return {
        key: item.field,
        description: item.description,
        enum: item.values,
      };
    });
  },

  metaDataSettingJSONToMetaDataTableData(
    data: IMetaDataReturnJSONSettings,
  ): IMetaDataTableData[] {
    if (!data) return [];
    if (Array.isArray(data)) {
      return data.map((item) => {
        return {
          field: item.key,
          description: item.description,
          values: item.enum || [],
          restrictDefinedValues: !!item.enum?.length,
          valueType: DEFAULT_VALUE_TYPE,
        } as IMetaDataTableData;
      });
    }
    const properties = data.properties || {};
    return Object.entries(properties).map(([key, property]) => {
      const valueType = 'string';
      const values = property.enum || property.items?.enum || [];
      return {
        field: key,
        description: property.description || '',
        values,
        restrictDefinedValues: !!values.length,
        valueType,
      } as IMetaDataTableData;
    });
  },
};

export const useMetadataOperations = () => {
  const [operations, setOperations] = useState<MetadataOperations>({
    deletes: [],
    updates: [],
  });
  // const operationsRef = useRef(operations);

  const addDeleteRow = useCallback((key: string) => {
    setOperations((prev) => ({
      ...prev,
      deletes: [...prev.deletes, { key }],
    }));
  }, []);

  const addDeleteBatch = useCallback((keys: string[]) => {
    setOperations((prev) => ({
      ...prev,
      deletes: [...prev.deletes, ...keys.map((key) => ({ key }))],
    }));
  }, []);

  const addDeleteValue = useCallback((key: string, value: string) => {
    setOperations((prev) => ({
      ...prev,
      deletes: [...prev.deletes, { key, value }],
    }));
  }, []);

  const addUpdateValue = useCallback(
    (
      key: string,
      originalValue: string,
      newValue: string | string[],
      type?: MetadataValueType,
    ) => {
      let newValuesRes: string | string[];
      if (type !== metadataValueTypeEnum['list']) {
        if (Array.isArray(newValue) && newValue.length > 0) {
          newValuesRes = newValue[0];
        } else {
          newValuesRes = newValue;
        }
      } else {
        newValuesRes = newValue;
      }
      setOperations((prev) => {
        let updatedUpdates = [...prev.updates];
        const existsIndex = prev.updates.findIndex(
          (update) =>
            update.key === key &&
            update.match === originalValue &&
            update.match !== '',
        );
        if (existsIndex > -1) {
          updatedUpdates[existsIndex] = {
            key,
            match: originalValue,
            value: newValuesRes,
            valueType: type || DEFAULT_VALUE_TYPE,
          };

          // operationsRef.current = updatedOperations;
        } else {
          updatedUpdates.push({
            key,
            match: originalValue,
            value: newValuesRes,
            valueType: type,
          });
        }
        updatedUpdates = updatedUpdates.reduce((pre, cur) => {
          if (
            !pre.some(
              (item) =>
                item.key === cur.key &&
                item.match === cur.match &&
                item.value === cur.value,
            )
          ) {
            pre.push(cur);
          }
          return pre;
        }, [] as UpdateOperation[]);

        const updatedOperations = {
          ...prev,
          updates: updatedUpdates,
        };
        return updatedOperations;
      });
    },
    [],
  );

  const resetOperations = useCallback(() => {
    setOperations({
      deletes: [],
      updates: [],
    });
  }, []);

  return {
    // operationsRef,
    operations,
    addDeleteBatch,
    addDeleteRow,
    addDeleteValue,
    addUpdateValue,
    resetOperations,
  };
};

export const useFetchMetaDataManageData = (
  type: MetadataType = MetadataType.Manage,
  documentIds?: string[],
) => {
  const { id } = useParams();

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IMetaDataTableData[]>({
    queryKey: ['fetchMetaData', id, documentIds],
    enabled:
      !!id &&
      (type === MetadataType.Manage || type === MetadataType.UpdateSingle),
    initialData: [],
    gcTime: 1000,
    queryFn: async () => {
      const { data } = await getMetaDataService({
        kb_id: id as string,
        doc_ids: documentIds,
      });
      if (data?.data?.summary) {
        const res = util.changeToMetaDataTableData(data.data.summary);
        return res;
      }
      return [];
    },
  });
  return {
    data,
    loading,
    refetch,
  };
};

const fetchTypeList = [MetadataType.Manage, MetadataType.UpdateSingle];
export const useManageMetaDataModal = (
  metaData: IMetaDataTableData[] = [],
  type: MetadataType = MetadataType.Manage,
  otherData?: Record<string, any>,
  documentIds?: string[],
) => {
  const { id } = useParams();
  const { t } = useTranslation();
  const { data, loading } = useFetchMetaDataManageData(type, documentIds);

  const [tableData, setTableData] = useState<IMetaDataTableData[]>(metaData);
  const queryClient = useQueryClient();
  const {
    // operationsRef,
    operations,
    addDeleteRow,
    addDeleteBatch,
    addDeleteValue,
    addUpdateValue,
    resetOperations,
  } = useMetadataOperations();

  // const { setDocumentMeta } = useSetDocumentMeta();

  useEffect(() => {
    if (fetchTypeList.includes(type)) {
      if (data) {
        setTableData(data);
      } else {
        setTableData([]);
      }
    }
  }, [data, type]);

  useEffect(() => {
    if (!fetchTypeList.includes(type)) {
      if (metaData) {
        setTableData(metaData);
      } else {
        setTableData([]);
      }
    }
  }, [metaData, type]);

  const handleDeleteSingleValue = useCallback(
    (field: string, value: string) => {
      addDeleteValue(field, value);

      setTableData((prevTableData) => {
        const newTableData = prevTableData.map((item) => {
          if (item.field === field) {
            return {
              ...item,
              values: item.values.filter((v) => v !== value),
            };
          }
          return item;
        });
        return newTableData;
      });
    },
    [addDeleteValue],
  );

  const handleDeleteSingleRow = useCallback(
    (field: string) => {
      addDeleteRow(field);
      setTableData((prevTableData) => {
        const newTableData = prevTableData.filter(
          (item) => item.field !== field,
        );
        return newTableData;
      });
    },
    [addDeleteRow],
  );

  const handleDeleteBatchRow = useCallback(
    (fields: string[]) => {
      addDeleteBatch(fields);
      setTableData((prevTableData) => {
        const newTableData = prevTableData.filter(
          (item) => !fields.includes(item.field),
        );
        return newTableData;
      });
    },
    [addDeleteBatch],
  );

  const handleSaveManage = useCallback(
    async (callback: () => void) => {
      console.log('handleSaveManage', tableData);
      const { data: res } = await updateMetaData({
        kb_id: id as string,
        data: operations,
        doc_ids: documentIds,
      });
      if (res.code === 0) {
        queryClient.invalidateQueries({
          queryKey: [DocumentApiAction.FetchDocumentList],
        });
        resetOperations();
        message.success(t('message.operated'));
        callback();
      }
    },
    [operations, id, t, queryClient, resetOperations, documentIds, tableData],
  );

  // const handleSaveUpdateSingle = useCallback(
  //   async (callback: () => void) => {
  //     const reqData = util.tableDataToMetaDataJSON(tableData);
  //     if (otherData?.id) {
  //       const ret = await setDocumentMeta({
  //         documentId: otherData?.id,
  //         meta: JSON.stringify(reqData),
  //       });
  //       if (ret === 0) {
  //         // message.success(t('message.success'));
  //         callback();
  //       }
  //     }
  //   },
  //   [tableData, otherData, setDocumentMeta],
  // );

  const handleSaveSettings = useCallback(
    async (callback: () => void, builtInMetadata?: IBuiltInMetadataItem[]) => {
      const data = util.tableDataToMetaDataSettingJSON(tableData);
      const { data: res } = await kbService.kbUpdateMetaData({
        kb_id: id,
        metadata: data,
        builtInMetadata: builtInMetadata || [],
      });
      if (res.code === 0) {
        message.success(t('message.operated'));
        callback?.();
      }
      // callback?.();
      return {
        metadata: data,
        builtInMetadata: builtInMetadata || [],
      };
    },
    [tableData, t, id],
  );

  const handleSaveSingleFileSettings = useCallback(
    async (callback: () => void) => {
      const data = util.tableDataToMetaDataSettingJSON(tableData);
      if (otherData?.documentId) {
        const { data: res } = await kbService.documentUpdateMetaData({
          doc_id: otherData.documentId,
          metadata: data,
        });
        if (res.code === 0) {
          message.success(t('message.operated'));
          callback?.();
        }
      }

      return data;
    },
    [tableData, t, otherData],
  );

  const handleSave = useCallback(
    async ({
      callback,
      builtInMetadata,
    }: {
      callback: () => void;
      builtInMetadata?: IBuiltInMetadataItem[];
    }) => {
      switch (type) {
        case MetadataType.UpdateSingle:
          // handleSaveUpdateSingle(callback);
          handleSaveManage(callback);
          break;
        case MetadataType.Manage:
          handleSaveManage(callback);
          break;
        case MetadataType.Setting:
          return handleSaveSettings(callback, builtInMetadata);

        case MetadataType.SingleFileSetting:
          return handleSaveSingleFileSettings(callback);
        default:
          handleSaveManage(callback);
          break;
      }
    },
    [
      handleSaveManage,
      type,
      // handleSaveUpdateSingle,
      handleSaveSettings,
      handleSaveSingleFileSettings,
    ],
  );

  return {
    tableData,
    setTableData,
    handleDeleteSingleValue,
    handleDeleteSingleRow,
    handleDeleteBatchRow,
    loading,
    handleSave,
    addUpdateValue,
    addDeleteValue,
  };
};

export const useManageMetadata = () => {
  const [tableData, setTableData] = useState<IMetaDataTableData[]>([]);
  const [config, setConfig] = useState<ShowManageMetadataModalProps>(
    {} as ShowManageMetadataModalProps,
  );
  const {
    visible: manageMetadataVisible,
    showModal,
    hideModal: hideManageMetadataModal,
  } = useSetModalState();
  const showManageMetadataModal = useCallback(
    (config?: ShowManageMetadataModalProps) => {
      const { metadata } = config || {};
      if (metadata) {
        setTableData(metadata);
      }
      if (config) {
        setConfig(config);
      }
      showModal();
    },
    [showModal],
  );
  return {
    manageMetadataVisible,
    showManageMetadataModal,
    hideManageMetadataModal,
    tableData,
    config,
  };
};

export const useOperateData = ({
  rowSelection,
  list,
  handleDeleteBatchRow,
}: {
  rowSelection: RowSelectionState;
  list: IMetaDataTableData[];
  handleDeleteBatchRow: (keys: string[]) => void;
}) => {
  const mapList = useMemo(() => {
    return list.map((x) => {
      return { ...x, id: x.field };
    });
  }, [list]);
  const { selectedIds: selectedRowKeys } = useSelectedIds(
    rowSelection,
    mapList,
  );
  // const handleDelete = useCallback(() => {
  //   console.log('rowSelection', rowSelection);
  // }, [rowSelection]);

  const handleDelete = useCallback(() => {
    const deletedKeys = selectedRowKeys.filter((x) =>
      mapList.some((y) => y.id === x),
    );
    handleDeleteBatchRow(deletedKeys);
    return;
  }, [selectedRowKeys, mapList, handleDeleteBatchRow]);
  return { handleDelete };
};
