import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { zodResolver } from '@hookform/resolvers/zod';
import { ReactNode, useCallback, useId } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';

const FormSchema = z.object({
  // Alias is optional and mirrors the user_canvas_version.alias column.
  alias: z.string().trim().max(255).optional(),
});

interface AliasFormDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  initialAlias?: string;
  onSubmit: (alias?: string) => void;
  children?: ReactNode;
}

export function AliasFormDialog({
  open,
  onOpenChange,
  title,
  description,
  initialAlias = '',
  onSubmit,
  children,
}: AliasFormDialogProps) {
  const { t } = useTranslation();
  const formId = useId();

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: { alias: initialAlias },
  });

  const handleConfirm = useCallback(
    (values: z.infer<typeof FormSchema>) => {
      onSubmit(values.alias || undefined);
      form.reset();
      onOpenChange(false);
    },
    [onSubmit, form, onOpenChange],
  );

  const handleOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (!nextOpen) {
        form.reset();
      }
      onOpenChange(nextOpen);
    },
    [form, onOpenChange],
  );

  const handleCancelClick = useCallback(() => {
    handleOpenChange(false);
  }, [handleOpenChange]);

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        {description && (
          <DialogDescription className="text-sm text-text-secondary">
            {description}
          </DialogDescription>
        )}
        <Form {...form}>
          <form
            id={formId}
            onSubmit={form.handleSubmit(handleConfirm)}
            className="space-y-3"
          >
            {children}
            <RAGFlowFormItem name="alias" label={t('flow.version.alias')}>
              <Input placeholder={t('flow.version.alias')} autoComplete="off" />
            </RAGFlowFormItem>
          </form>
        </Form>
        <DialogFooter className="gap-2 mt-4">
          <Button variant="outline" onClick={handleCancelClick}>
            {t('common.cancel')}
          </Button>
          <Button type="submit" form={formId}>
            {t('common.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
