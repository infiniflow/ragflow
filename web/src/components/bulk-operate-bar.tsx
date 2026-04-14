import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { BrushCleaning } from 'lucide-react';
import { ReactNode, useId } from 'react';
import { useTranslation } from 'react-i18next';
import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from './confirm-delete-dialog';
import { Separator } from './ui/separator';

export type BulkOperateItemType = {
  id: string;
  label: ReactNode;
  icon: ReactNode;
  onClick(): void;
};

type BulkOperateBarProps = {
  list: BulkOperateItemType[];
  count: number;
  className?: string;
  unit?: string;
};

export function BulkOperateBar({
  list,
  count,
  className,
  unit,
}: BulkOperateBarProps) {
  const { t } = useTranslation();
  const ariaDescriptionId = useId();

  return (
    <Card
      className={className}
      role="menu"
      aria-label={t('common.bulkOperate')}
      aria-describedby={ariaDescriptionId}
    >
      <CardContent className="ps-5 pe-1 py-1 flex items-center gap-6">
        <p
          id={ariaDescriptionId}
          className="text-sm text-text-secondary flex items-center gap-2"
        >
          {t('common.selected')}: {count} {unit ?? t('knowledgeDetails.files')}
          <BrushCleaning className="size-[1em]" />
        </p>

        <Separator orientation={'vertical'} className="h-[1em]"></Separator>

        <ul className="flex gap-2">
          {list.map((x) => {
            const isDeleteItem = x.id === 'delete';

            const buttonEl = (
              <Button
                variant={isDeleteItem ? 'danger' : 'outline'}
                onClick={isDeleteItem ? () => {} : x.onClick}
                role="menuitem"
              >
                {x.icon} {x.label}
              </Button>
            );

            return (
              <li key={x.id}>
                {isDeleteItem ? (
                  <ConfirmDeleteDialog
                    key="deleteModal"
                    onOk={x.onClick}
                    title={
                      unit
                        ? t('common.delete') + ' ' + unit
                        : t('deleteModal.delFiles')
                    }
                    content={{
                      title: t('common.deleteThem'),
                      node: (
                        <ConfirmDeleteDialogNode
                          name={`${unit ? t('common.selected') + ' ' + count + ' ' + unit : t('deleteModal.delFilesContent', { count })}`}
                        />
                      ),
                    }}
                  >
                    {buttonEl}
                  </ConfirmDeleteDialog>
                ) : (
                  buttonEl
                )}
              </li>
            );
          })}
        </ul>
      </CardContent>
    </Card>
  );
}
