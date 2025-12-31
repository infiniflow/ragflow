import message from '@/components/ui/message';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  DocumentApiAction,
  useSetDocumentMeta,
} from '@/hooks/use-document-request';
import kbService, {
  getMetaDataService,
  updateMetaData,
} from '@/services/knowledge-service';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { TFunction } from 'i18next';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import {
  IMetaDataReturnJSONSettings,
  IMetaDataReturnJSONType,
  IMetaDataReturnType,
  IMetaDataTableData,
  MetadataOperations,
  ShowManageMetadataModalProps,
} from '../interface';
export enum MetadataType {
  Manage = 1,
  UpdateSingle = 2,
  Setting = 3,
  SingleFileSetting = 4,
}

export const MetadataDeleteMap = (
  t: TFunction<'translation', undefined>,
): Record<
  MetadataType,
  {
    title: string;
    warnFieldText: string;
    warnValueText: string;
    warnFieldName: string;
    warnValueName: string;
  }
> => {
  return {
    [MetadataType.Manage]: {
      title: t('common.delete') + ' ' + t('knowledgeDetails.metadata.metadata'),
      warnFieldText: t('knowledgeDetails.metadata.deleteManageFieldAllWarn'),
      warnValueText: t('knowledgeDetails.metadata.deleteManageValueAllWarn'),
      warnFieldName: t('knowledgeDetails.metadata.fieldNameExists'),
      warnValueName: t('knowledgeDetails.metadata.valueExists'),
    },
    [MetadataType.Setting]: {
      title: t('common.delete') + ' ' + t('knowledgeDetails.metadata.metadata'),
      warnFieldText: t('knowledgeDetails.metadata.deleteSettingFieldWarn'),
      warnValueText: t('knowledgeDetails.metadata.deleteSettingValueWarn'),
      warnFieldName: t('knowledgeDetails.metadata.fieldExists'),
      warnValueName: t('knowledgeDetails.metadata.valueExists'),
    },
    [MetadataType.UpdateSingle]: {
      title: t('common.delete') + ' ' + t('knowledgeDetails.metadata.metadata'),
      warnFieldText: t('knowledgeDetails.metadata.deleteManageFieldSingleWarn'),
      warnValueText: t('knowledgeDetails.metadata.deleteManageValueSingleWarn'),
      warnFieldName: t('knowledgeDetails.metadata.fieldSingleNameExists'),
      warnValueName: t('knowledgeDetails.metadata.valueSingleExists'),
    },
    [MetadataType.SingleFileSetting]: {
      title: t('common.delete') + ' ' + t('knowledgeDetails.metadata.metadata'),
      warnFieldText: t('knowledgeDetails.metadata.deleteSettingFieldWarn'),
      warnValueText: t('knowledgeDetails.metadata.deleteSettingValueWarn'),
      warnFieldName: t('knowledgeDetails.metadata.fieldExists'),
      warnValueName: t('knowledgeDetails.metadata.valueSingleExists'),
    },
  };
};
export const util = {
  changeToMetaDataTableData(data: IMetaDataReturnType): IMetaDataTableData[] {
    return Object.entries(data).map(([key, value]) => {
      const values = value.map(([v]) => v.toString());
      console.log('values', values);
      return {
        field: key,
        description: '',
        values: values,
      } as IMetaDataTableData;
    });
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
    if (!Array.isArray(data)) return [];
    return data.map((item) => {
      return {
        field: item.key,
        description: item.description,
        values: item.enum,
        restrictDefinedValues: !!item.enum?.length,
      } as IMetaDataTableData;
    });
  },
};

export const useMetadataOperations = () => {
  const [operations, setOperations] = useState<MetadataOperations>({
    deletes: [],
    updates: [],
  });

  const addDeleteRow = useCallback((key: string) => {
    setOperations((prev) => ({
      ...prev,
      deletes: [...prev.deletes, { key }],
    }));
  }, []);

  const addDeleteValue = useCallback((key: string, value: string) => {
    setOperations((prev) => ({
      ...prev,
      deletes: [...prev.deletes, { key, value }],
    }));
  }, []);

  // const addUpdateValue = useCallback(
  //   (key: string, value: string | string[]) => {
  //     setOperations((prev) => ({
  //       ...prev,
  //       updates: [...prev.updates, { key, value }],
  //     }));
  //   },
  //   [],
  // );
  const addUpdateValue = useCallback(
    (key: string, originalValue: string, newValue: string) => {
      setOperations((prev) => {
        const existsIndex = prev.updates.findIndex(
          (update) => update.key === key && update.match === originalValue,
        );

        if (existsIndex > -1) {
          const updatedUpdates = [...prev.updates];
          updatedUpdates[existsIndex] = {
            key,
            match: originalValue,
            value: newValue,
          };
          return {
            ...prev,
            updates: updatedUpdates,
          };
        }
        return {
          ...prev,
          updates: [
            ...prev.updates,
            { key, match: originalValue, value: newValue },
          ],
        };
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
    operations,
    addDeleteRow,
    addDeleteValue,
    addUpdateValue,
    resetOperations,
  };
};

export const useFetchMetaDataManageData = (
  type: MetadataType = MetadataType.Manage,
) => {
  const { id } = useParams();
  // const [data, setData] = useState<IMetaDataTableData[]>([]);
  // const [loading, setLoading] = useState(false);
  // const fetchData = useCallback(async (): Promise<IMetaDataTableData[]> => {
  //   setLoading(true);
  //   const { data } = await getMetaDataService({
  //     kb_id: id as string,
  //   });
  //   setLoading(false);
  //   if (data?.data?.summary) {
  //     return util.changeToMetaDataTableData(data.data.summary);
  //   }
  //   return [];
  // }, [id]);
  // useEffect(() => {
  //   if (type === MetadataType.Manage) {
  //     fetchData()
  //       .then((res) => {
  //         setData(res);
  //       })
  //       .catch((res) => {
  //         console.error(res);
  //       });
  //   }
  // }, [type, fetchData]);

  const {
    data,
    isFetching: loading,
    refetch,
  } = useQuery<IMetaDataTableData[]>({
    queryKey: ['fetchMetaData', id],
    enabled: !!id && type === MetadataType.Manage,
    initialData: [],
    gcTime: 1000,
    queryFn: async () => {
      const { data } = await getMetaDataService({
        kb_id: id as string,
      });
      if (data?.data?.summary) {
        return util.changeToMetaDataTableData(data.data.summary);
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

export const useManageMetaDataModal = (
  metaData: IMetaDataTableData[] = [],
  type: MetadataType = MetadataType.Manage,
  otherData?: Record<string, any>,
) => {
  const { id } = useParams();
  const { t } = useTranslation();
  const { data, loading } = useFetchMetaDataManageData(type);

  const [tableData, setTableData] = useState<IMetaDataTableData[]>(metaData);
  const queryClient = useQueryClient();
  const {
    operations,
    addDeleteRow,
    addDeleteValue,
    addUpdateValue,
    resetOperations,
  } = useMetadataOperations();

  const { setDocumentMeta } = useSetDocumentMeta();

  useEffect(() => {
    if (type === MetadataType.Manage) {
      if (data) {
        setTableData(data);
      } else {
        setTableData([]);
      }
    }
  }, [data, type]);

  useEffect(() => {
    if (type !== MetadataType.Manage) {
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
        // console.log('newTableData', newTableData, prevTableData);
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
        // console.log('newTableData', newTableData, prevTableData);
        return newTableData;
      });
    },
    [addDeleteRow],
  );

  const handleSaveManage = useCallback(
    async (callback: () => void) => {
      const { data: res } = await updateMetaData({
        kb_id: id as string,
        data: operations,
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
    [operations, id, t, queryClient, resetOperations],
  );

  const handleSaveUpdateSingle = useCallback(
    async (callback: () => void) => {
      const reqData = util.tableDataToMetaDataJSON(tableData);
      if (otherData?.id) {
        const ret = await setDocumentMeta({
          documentId: otherData?.id,
          meta: JSON.stringify(reqData),
        });
        if (ret === 0) {
          // message.success(t('message.success'));
          callback();
        }
      }
    },
    [tableData, otherData, setDocumentMeta],
  );

  const handleSaveSettings = useCallback(
    async (callback: () => void) => {
      const data = util.tableDataToMetaDataSettingJSON(tableData);
      const { data: res } = await kbService.kbUpdateMetaData({
        kb_id: id,
        metadata: data,
      });
      if (res.code === 0) {
        message.success(t('message.operated'));
        callback?.();
      }

      return data;
    },
    [tableData, id, t],
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
    async ({ callback }: { callback: () => void }) => {
      switch (type) {
        case MetadataType.UpdateSingle:
          handleSaveUpdateSingle(callback);
          break;
        case MetadataType.Manage:
          handleSaveManage(callback);
          break;
        case MetadataType.Setting:
          return handleSaveSettings(callback);
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
      handleSaveUpdateSingle,
      handleSaveSettings,
      handleSaveSingleFileSettings,
    ],
  );

  return {
    tableData,
    setTableData,
    handleDeleteSingleValue,
    handleDeleteSingleRow,
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
        // const dataTemp = Object.entries(metadata).map(([key, value]) => {
        //   return {
        //     field: key,
        //     description: '',
        //     values: Array.isArray(value) ? value : [value],
        //   } as IMetaDataTableData;
        // });
        setTableData(metadata);
        console.log('metadata-2', metadata);
      }
      console.log('metadata-3', metadata);
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
