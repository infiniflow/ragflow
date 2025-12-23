import message from '@/components/ui/message';
import { useSetModalState } from '@/hooks/common-hooks';
import { useSetDocumentMeta } from '@/hooks/use-document-request';
import kbService, {
  getMetaDataService,
  updateMetaData,
} from '@/services/knowledge-service';
import { useQuery } from '@tanstack/react-query';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'umi';
import {
  IMetaDataReturnJSONSettings,
  IMetaDataReturnJSONType,
  IMetaDataReturnType,
  IMetaDataTableData,
  MetadataOperations,
  ShowManageMetadataModalProps,
} from './interface';
export enum MetadataType {
  Manage = 1,
  UpdateSingle = 2,
  Setting = 3,
}
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
      return {
        field: key,
        description: '',
        values: value,
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

  const addUpdateValue = useCallback(
    (key: string, value: string | string[]) => {
      setOperations((prev) => ({
        ...prev,
        updates: [...prev.updates, { key, value }],
      }));
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

  const { operations, addDeleteRow, addDeleteValue, addUpdateValue } =
    useMetadataOperations();

  const { setDocumentMeta } = useSetDocumentMeta();

  useEffect(() => {
    if (data) {
      setTableData(data);
    } else {
      setTableData([]);
    }
  }, [data]);

  useEffect(() => {
    if (metaData) {
      setTableData(metaData);
    } else {
      setTableData([]);
    }
  }, [metaData]);

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
        message.success(t('message.operated'));
        callback();
      }
    },
    [operations, id, t],
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
    [tableData, id],
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
        default:
          handleSaveManage(callback);
          break;
      }
    },
    [handleSaveManage, type, handleSaveUpdateSingle, handleSaveSettings],
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

export const useManageValues = () => {
  const [updateValues, setUpdateValues] = useState<{
    field: string;
    values: string[];
  } | null>(null);
  return { updateValues, setUpdateValues };
};
