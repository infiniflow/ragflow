import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { cn } from '@/lib/utils';
import { BrushCleaning } from 'lucide-react';
import { ReactNode, useCallback } from 'react';
import { ConfirmDeleteDialog } from './confirm-delete-dialog';
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
};

export function BulkOperateBar({
  list,
  count,
  className,
}: BulkOperateBarProps) {
  const isDeleteItem = useCallback((id: string) => {
    return id === 'delete';
  }, []);

  return (
    <Card className={cn('mb-4', className)}>
      <CardContent className="p-1 pl-5 flex items-center gap-6">
        <section className="text-text-sub-title-invert flex items-center gap-2">
          <span>Selected: {count} Files</span>
          <BrushCleaning className="size-3" />
        </section>
        <Separator orientation={'vertical'} className="h-3"></Separator>
        <ul className="flex gap-2">
          {list.map((x) => (
            <li
              key={x.id}
              className={cn({ ['text-state-error']: isDeleteItem(x.id) })}
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
