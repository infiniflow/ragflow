import {
  AsyncTreeSelect,
  TreeNodeType,
} from '@/components/ui/async-tree-select';
import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useFetchPureFileList } from '@/hooks/use-file-request';
import { IModalProps } from '@/interfaces/common';
import { IFile } from '@/interfaces/database/file-manager';
import { isEmpty } from 'lodash';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';

export function MoveDialog({ hideModal, onOk, loading }: IModalProps<any>) {
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
          <ButtonLoading
            type="submit"
            onClick={handleSubmit}
            disabled={isEmpty(treeValue)}
            loading={loading}
          >
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
