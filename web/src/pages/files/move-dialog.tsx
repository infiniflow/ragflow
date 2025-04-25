import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import SingleTreeSelect, { TreeNode } from '@/components/ui/single-tree-select';
import { IModalProps } from '@/interfaces/common';
import { useTranslation } from 'react-i18next';

export function MoveDialog({ hideModal }: IModalProps<any>) {
  const { t } = useTranslation();

  const treeData: TreeNode[] = [
    {
      id: 1,
      label: 'Node 1',
      children: [
        { id: 11, label: 'Node 1.1' },
        { id: 12, label: 'Node 1.2' },
      ],
    },
    {
      id: 2,
      label: 'Node 2',
      children: [
        {
          id: 21,
          label: 'Node 2.1',
          children: [
            { id: 211, label: 'Node 2.1.1' },
            { id: 212, label: 'Node 2.1.2' },
          ],
        },
      ],
    },
  ];

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('common.move')}</DialogTitle>
        </DialogHeader>
        <div>
          <SingleTreeSelect treeData={treeData}></SingleTreeSelect>
        </div>
        <DialogFooter>
          <Button type="submit">Save changes</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
