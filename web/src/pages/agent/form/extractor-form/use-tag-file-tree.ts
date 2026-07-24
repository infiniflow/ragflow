import { TreeNodeType } from '@/components/ui/async-tree-select';
import { useFetchPureFileList } from '@/hooks/use-file-request';
import { IFile } from '@/interfaces/database/file-manager';
import { isFolderType } from '@/pages/files/util';
import { getExtension } from '@/utils/document-util';
import { uniqBy } from 'lodash';
import { useCallback, useState } from 'react';

const AllowedExtensions = ['xlsx', 'xls', 'csv', 'txt'];

export function canSelectTagFile(node: TreeNodeType) {
  return Boolean(node.isLeaf);
}

export function useTagFileTree() {
  const { fetchList } = useFetchPureFileList();

  const [treeData, setTreeData] = useState<TreeNodeType[]>([]);

  const loadData = useCallback(
    async ({ id }: TreeNodeType) => {
      const ret = await fetchList(id as string);
      if (ret.code === 0) {
        setTreeData((tree) =>
          uniqBy(
            tree.concat(
              ret.data.files
                .filter(
                  (x: IFile) =>
                    (isFolderType(x.type) &&
                      x.name.toLowerCase() !== 'skills') ||
                    AllowedExtensions.includes(getExtension(x.name)),
                )
                .map((x: IFile) => ({
                  id: x.id,
                  parentId: x.parent_id,
                  title: x.name,
                  // has_child_folder only counts child folders, so folders
                  // must always be expandable to reach files inside them
                  isLeaf: !isFolderType(x.type),
                })),
            ),
            'id',
          ),
        );
      }
    },
    [fetchList],
  );

  return { treeData, loadData };
}
