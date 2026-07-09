import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ICompilationTemplateGroup } from '@/interfaces/database/compilation-template';
import { Trash2 } from 'lucide-react';
import { PropsWithChildren, useCallback } from 'react';
import { useTranslation } from 'react-i18next';

type TemplateDropdownProps = PropsWithChildren<{
  data: ICompilationTemplateGroup;
  onDelete: (id: string) => void;
}>;

export function TemplateDropdown({
  children,
  data,
  onDelete,
}: TemplateDropdownProps) {
  const { t } = useTranslation();

  const handleDelete = useCallback(() => {
    onDelete(data.id);
  }, [data.id, onDelete]);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <div onClick={(e) => e.stopPropagation()}>{children}</div>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <ConfirmDeleteDialog
          title={t('setting.deleteTemplateGroupModalTitle')}
          content={{
            title: t('setting.deleteTemplateGroupModalContent'),
            node: <ConfirmDeleteDialogNode name={data.name} />,
          }}
          onOk={handleDelete}
        >
          <DropdownMenuItem
            className="text-state-error"
            onSelect={(e) => e.preventDefault()}
            onClick={(e) => e.stopPropagation()}
          >
            {t('common.delete')}
            <Trash2 className="size-4" />
          </DropdownMenuItem>
        </ConfirmDeleteDialog>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
