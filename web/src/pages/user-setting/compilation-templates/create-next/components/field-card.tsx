import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Pencil, Trash2 } from 'lucide-react';

type FieldCardProps = {
  title?: string;
  field: Record<string, string>;
  onEdit: () => void;
  onDelete: () => void;
};

export function FieldCard({ title, field, onEdit, onDelete }: FieldCardProps) {
  return (
    <Card className="border-border-button bg-transparent group">
      <CardContent className="p-4 space-y-3">
        <div className="flex items-start justify-between gap-2">
          <div className="space-y-2 space-x-2 flex-1 min-w-0">
            {title && (
              <span className="text-sm font-medium text-text-primary">
                {title}
              </span>
            )}
          </div>
          <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity shrink-0">
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              onClick={onEdit}
              className="text-text-secondary hover:text-text-primary"
            >
              <Pencil className="size-4" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              onClick={onDelete}
              className="text-text-secondary hover:text-state-error"
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        </div>

        {field.description && (
          <p className="text-sm text-text-secondary line-clamp-3">
            {field.description}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
