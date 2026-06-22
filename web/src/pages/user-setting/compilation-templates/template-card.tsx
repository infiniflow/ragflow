import {
  ConfirmDeleteDialog,
  ConfirmDeleteDialogNode,
} from '@/components/confirm-delete-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { ICompilationTemplate } from '@/interfaces/database/compilation-template';
import { Pencil, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';

type TemplateCardProps = {
  data: ICompilationTemplate;
  onEdit: (id: string) => void;
  onDelete: (id: string) => void;
};

export function TemplateCard({ data, onEdit, onDelete }: TemplateCardProps) {
  const { t } = useTranslation();

  return (
    <Card className="group">
      <CardContent className="p-4 flex flex-col">
        <section className="flex justify-between gap-2">
          <h3 className="text-base font-normal truncate text-text-primary">
            {data.name}
          </h3>

          <div className="flex items-center gap-1 shrink-0 opacity-0 group-hover:opacity-100 focus-within:opacity-100 transition-opacity">
            <Button
              size="icon-xs"
              variant="ghost"
              aria-label={t('common.edit')}
              onClick={() => onEdit(data.id)}
            >
              <Pencil className="size-3.5" />
            </Button>

            <ConfirmDeleteDialog
              title={t('setting.deleteTemplateModalTitle')}
              content={{
                title: t('setting.deleteTemplateModalContent'),
                node: <ConfirmDeleteDialogNode name={data.name} />,
              }}
              onOk={() => onDelete(data.id)}
            >
              <Button
                size="icon-xs"
                variant="ghost"
                aria-label={t('common.delete')}
              >
                <Trash2 className="size-3.5" />
              </Button>
            </ConfirmDeleteDialog>
          </div>
        </section>

        <p className="mt-1 text-sm text-text-secondary line-clamp-1 flex-1">
          {data.description}
        </p>

        <Badge variant="secondary" className="mt-3 w-fit">
          {t(`knowledgeCompilation.kind.${data.kind}`)}
        </Badge>
      </CardContent>
    </Card>
  );
}
