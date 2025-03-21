import get from 'lodash/get';
import { MouseEventHandler, useCallback, useMemo } from 'react';
import { v4 as uuid } from 'uuid';
import { IGenerateParameter, IInvokeVariable } from '../../interface';
import useGraphStore from '../../store';

export const useHandleOperateParameters = (nodeId: string) => {
  const { getNode, updateNodeForm } = useGraphStore((state) => state);
  const node = getNode(nodeId);
  const dataSource: {
    id?: string;
    key: string;
  }[] = useMemo(() => get(node, 'data.form.variables', []), [node]);

  const changeValue = useCallback(
    (row: IInvokeVariable, field: string, value: string) => {
      const newData = [...dataSource];
      const index = newData.findIndex((item) => row.id === item.id);
      const item = newData[index];
      newData.splice(index, 1, {
        ...item,
        [field]: value,
      });

      updateNodeForm(nodeId, { variables: newData });
    },
    [dataSource, nodeId, updateNodeForm],
  );

  const handleComponentIdChange = useCallback(
    (row: IInvokeVariable) => (value: string) => {
      changeValue(row, 'key', value);
    },
    [changeValue],
  );

  const handleRemove = useCallback(
    (id?: string) => () => {
      const newData = dataSource.filter((item) => item.id !== id);
      updateNodeForm(nodeId, { variables: newData });
    },
    [updateNodeForm, nodeId, dataSource],
  );

  const handleAdd: MouseEventHandler = useCallback(
    (e) => {
      e.preventDefault();
      e.stopPropagation();
      updateNodeForm(nodeId, {
        variables: [
          ...dataSource,
          {
            id: uuid(),
            key: undefined,
          },
        ],
      });
    },
    [dataSource, nodeId, updateNodeForm],
  );

  const handleSave = (row: IGenerateParameter) => {
    const newData = [...dataSource];
    const index = newData.findIndex((item) => row.id === item.id);
    const item = newData[index];
    newData.splice(index, 1, {
      ...item,
      ...row,
    });

    updateNodeForm(nodeId, { variables: newData });
  };

  return {
    handleAdd,
    handleRemove,
    handleComponentIdChange,
    handleSave,
    dataSource,
  };
};
