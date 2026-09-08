import { TreeNodeType } from '@/components/ui/async-tree-select';
import { useFetchPureFileList } from '@/hooks/use-file-request';
import { IFile } from '@/interfaces/database/file-manager';
import { isFolderType } from '@/pages/files/util';
import fileManagerService from '@/services/file-manager-service';
import { getExtension } from '@/utils/document-util';
import { uniqBy } from 'lodash';
import { useCallback, useEffect, useRef, useState } from 'react';

const AllowedExtensions = ['xlsx', 'xls', 'csv', 'txt'];

export function canSelectTagFile(node: TreeNodeType) {
  return Boolean(node.isLeaf);
}

function toTreeNode(x: IFile): TreeNodeType {
  return {
    id: x.id,
    parentId: x.parent_id,
    title: x.name,
    // has_child_folder only counts child folders, so folders
    // must always be expandable to reach files inside them
    isLeaf: !isFolderType(x.type),
  };
}

export function useTagFileTree(value?: string) {
  const { fetchList } = useFetchPureFileList();

  const [treeData, setTreeData] = useState<TreeNodeType[]>([]);

  const appendFiles = useCallback((files: IFile[]) => {
    setTreeData((tree) =>
      uniqBy(
        tree.concat(
          files
            .filter(
              (x) =>
                (isFolderType(x.type) && x.name.toLowerCase() !== 'skills') ||
                AllowedExtensions.includes(getExtension(x.name)),
            )
            .map(toTreeNode),
        ),
        'id',
      ),
    );
  }, []);

  const loadData = useCallback(
    async ({ id }: TreeNodeType) => {
      const ret = await fetchList(id as string);
      if (ret.code === 0) {
        appendFiles(ret.data.files);
      }
    },
    [fetchList, appendFiles],
  );

  // Resolve a previously saved selection: the tree is lazy-loaded, so a file
  // nested in subfolders is not in treeData on mount and its title cannot be
  // displayed. Walk the ancestor chain top-down and load each level.
  const resolvedValueRef = useRef<string>('');
  useEffect(() => {
    if (
      !value ||
      resolvedValueRef.current === value ||
      treeData.some((x) => x.id === value)
    ) {
      return;
    }
    resolvedValueRef.current = value;
    (async () => {
      const { data } = await fileManagerService.getAllParentFolder(
        {},
        `${value}/ancestors`,
      );
      const folders: IFile[] = data?.data?.parent_folders?.toReversed() ?? [];
      // The root folder is its own parent (parent_id === id). Keeping it in
      // the tree breaks AsyncTreeSelect's root detection (no node would count
      // as top-level, so nothing renders), so drop it — its children then
      // render as top-level nodes, same as a fresh listing.
      const chain = folders.filter((x) => x.id !== x.parent_id);
      setTreeData((tree) => uniqBy(tree.concat(chain.map(toTreeNode)), 'id'));
      for (const folder of folders) {
        const ret = await fetchList(folder.id);
        if (ret.code === 0) {
          appendFiles(ret.data.files);
        }
      }
    })();
  }, [value, treeData, fetchList, appendFiles]);

  return { treeData, loadData };
}
