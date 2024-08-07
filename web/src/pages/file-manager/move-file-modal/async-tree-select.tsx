import { useFetchPureFileList } from '@/hooks/file-manager-hooks';
import { IFile } from '@/interfaces/database/file-manager';
import type { GetProp, TreeSelectProps } from 'antd';
import { TreeSelect } from 'antd';
import { useCallback, useEffect, useState } from 'react';

type DefaultOptionType = GetProp<TreeSelectProps, 'treeData'>[number];

interface IProps {
  value?: string;
  onChange?: (value: string) => void;
}

const AsyncTreeSelect = ({ value, onChange }: IProps) => {
  const { fetchList } = useFetchPureFileList();
  const [treeData, setTreeData] = useState<Omit<DefaultOptionType, 'label'>[]>(
    [],
  );

  const onLoadData: TreeSelectProps['loadData'] = useCallback(
    async ({ id }) => {
      const ret = await fetchList(id);
      if (ret.retcode === 0) {
        setTreeData((tree) => {
          return tree.concat(
            ret.data.files
              .filter((x: IFile) => x.type === 'folder')
              .map((x: IFile) => ({
                id: x.id,
                pId: x.parent_id,
                value: x.id,
                title: x.name,
                isLeaf: false,
              })),
          );
        });
      }
    },
    [fetchList],
  );

  const handleChange = (newValue: string) => {
    onChange?.(newValue);
  };

  useEffect(() => {
    onLoadData?.({ id: '', props: '' });
  }, [onLoadData]);

  return (
    <TreeSelect
      treeDataSimpleMode
      style={{ width: '100%' }}
      value={value}
      dropdownStyle={{ maxHeight: 400, overflow: 'auto' }}
      placeholder="Please select"
      onChange={handleChange}
      loadData={onLoadData}
      treeData={treeData}
    />
  );
};

export default AsyncTreeSelect;
