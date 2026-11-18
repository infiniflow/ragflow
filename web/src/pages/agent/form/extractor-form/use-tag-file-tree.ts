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

export function useTagFileTree(value?: string, ownerTenantId?: string) {
  const { fetchList } = useFetchPureFileList();

  const [treeData, setTreeData] = useState<TreeNodeType[]>([]);
  const ownerTenantIdRef = useRef(ownerTenantId);
  ownerTenantIdRef.current = ownerTenantId;
  const appendFiles = useCallback((files: IFile[], tenantId?: string) => {
    if (ownerTenantIdRef.current !== tenantId) {
      return;
    }
    setTreeData((tree) =>
      uniqBy(
        tree.concat(
          files
            .filter(
              (x) =>
                isFolderType(x.type) ||
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
      const ret = await fetchList({
        parentId: id as string,
        tenantId: ownerTenantId,
      }).catch(() => null);
      if (ret?.code === 0) {
        appendFiles(ret.data.files, ownerTenantId);
      }
    },
    [fetchList, appendFiles, ownerTenantId],
  );

  // Resolve a previously saved selection: the tree is lazy-loaded, so a file
  // nested in subfolders is not in treeData on mount and its title cannot be
  // displayed. Walk the ancestor chain top-down and load each level.
  const resolvedValueRef = useRef<string>('');
  useEffect(() => {
    setTreeData([]);
    resolvedValueRef.current = '';
  }, [ownerTenantId]);

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
      const response = await fileManagerService
        .getAllParentFolder(
          {
            ...(ownerTenantId ? { tenant_id: ownerTenantId } : {}),
          },
          `${value}/ancestors`,
        )
        .catch(() => null);
      if (!response) {
        return;
      }
      if (ownerTenantIdRef.current !== ownerTenantId) {
        return;
      }
      const folders: IFile[] =
        response.data?.data?.parent_folders?.toReversed() ?? [];
      // The root folder is its own parent (parent_id === id). Keeping it in
      // the tree breaks AsyncTreeSelect's root detection (no node would count
      // as top-level, so nothing renders), so drop it — its children then
      // render as top-level nodes, same as a fresh listing.
      const chain = folders.filter((x) => x.id !== x.parent_id);
      setTreeData((tree) => uniqBy(tree.concat(chain.map(toTreeNode)), 'id'));
      for (const folder of folders) {
        // The ancestor response starts with the selected file itself. Only
        // folders can be used as parent_id when loading child entries.
        if (!isFolderType(folder.type)) {
          continue;
        }
        const ret = await fetchList({
          parentId: folder.id,
          tenantId: ownerTenantId,
        }).catch(() => null);
        if (!ret) {
          return;
        }
        if (ownerTenantIdRef.current !== ownerTenantId) {
          return;
        }
        if (ret.code === 0) {
          appendFiles(ret.data.files, ownerTenantId);
        }
      }
    })();
  }, [value, treeData, fetchList, appendFiles, ownerTenantId]);

  return { treeData, loadData };
}
