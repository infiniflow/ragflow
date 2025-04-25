import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { ReactNode, useCallback } from 'react';
import { ConfirmDeleteDialog } from './confirm-delete-dialog';

export type BulkOperateItemType = {
  id: string;
  label: ReactNode;
  icon: ReactNode;
  onClick(): void;
};

type BulkOperateBarProps = { list: BulkOperateItemType[] };

export function BulkOperateBar({ list }: BulkOperateBarProps) {
  const isDeleteItem = useCallback((id: string) => {
    return id === 'delete';
  }, []);

  return (
    <Card className="mb-4">
      <CardContent className="p-1">
        <ul className="flex gap-2">
          {list.map((x) => (
            <li
              key={x.id}
              className={cn({ ['text-text-delete-red']: isDeleteItem(x.id) })}
            >
              <ConfirmDeleteDialog
                hidden={!isDeleteItem(x.id)}
                onOk={x.onClick}
              >
                <Button
                  variant={'ghost'}
                  onClick={isDeleteItem(x.id) ? () => {} : x.onClick}
                >
                  {x.icon} {x.label}
                </Button>
              </ConfirmDeleteDialog>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
