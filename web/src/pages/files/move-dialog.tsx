import {
  AsyncTreeSelect,
  TreeNodeType,
} from '@/components/ui/async-tree-select';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useFetchPureFileList } from '@/hooks/file-manager-hooks';
import { IModalProps } from '@/interfaces/common';
import { IFile } from '@/interfaces/database/file-manager';
import { isEmpty } from 'lodash';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';

export function MoveDialog({ hideModal, onOk }: IModalProps<any>) {
  const { t } = useTranslation();

  const { fetchList } = useFetchPureFileList();

  const [treeValue, setTreeValue] = useState<number | string>('');

  const [treeData, setTreeData] = useState([]);

  const onLoadData = useCallback(
    async ({ id }: TreeNodeType) => {
      const ret = await fetchList(id as string);
      if (ret.code === 0) {
        setTreeData((tree) => {
          return tree.concat(
            ret.data.files
              .filter((x: IFile) => x.type === 'folder')
              .map((x: IFile) => ({
                id: x.id,
                parentId: x.parent_id,
                title: x.name,
                isLeaf:
                  typeof x.has_child_folder === 'boolean'
                    ? !x.has_child_folder
                    : false,
              })),
          );
        });
      }
    },
    [fetchList],
  );

  const handleSubmit = useCallback(() => {
    onOk?.(treeValue);
  }, [onOk, treeValue]);

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('common.move')}</DialogTitle>
        </DialogHeader>
        <div>
          <AsyncTreeSelect
            treeData={treeData}
            value={treeValue}
            onChange={setTreeValue}
            loadData={onLoadData}
          ></AsyncTreeSelect>
        </div>
        <DialogFooter>
          <Button
            type="submit"
            onClick={handleSubmit}
            disabled={isEmpty(treeValue)}
          >
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
