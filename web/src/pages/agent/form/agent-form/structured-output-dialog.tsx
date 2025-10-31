import {
  JSONSchema,
  JsonSchemaVisualizer,
  SchemaVisualEditor,
} from '@/components/jsonjoy-builder';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { IModalProps } from '@/interfaces/common';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';

export function StructuredOutputDialog({
  hideModal,
  onOk,
  initialValues,
}: IModalProps<any>) {
  const { t } = useTranslation();
  const [schema, setSchema] = useState<JSONSchema>(initialValues);

  const handleOk = useCallback(() => {
    onOk?.(schema);
  }, [onOk, schema]);

  return (
    <Dialog onOpenChange={hideModal} open>
      <DialogContent className="md:max-w-[1200px] h-[50vh]">
        <DialogHeader>
          <DialogTitle> {t('flow.structuredOutput.configuration')}</DialogTitle>
        </DialogHeader>
        <section className="flex overflow-auto">
          <div className="flex-1">
            <SchemaVisualEditor schema={schema} onChange={setSchema} />
          </div>
          <div className="flex-1">
            <JsonSchemaVisualizer schema={schema} onChange={setSchema} />
          </div>
        </section>
        <DialogFooter>
          <DialogClose asChild>
            <Button variant="outline">{t('common.cancel')}</Button>
          </DialogClose>
          <Button type="button" onClick={handleOk}>
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
