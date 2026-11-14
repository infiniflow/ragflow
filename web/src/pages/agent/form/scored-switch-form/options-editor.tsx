import { Button } from '@/components/ui/button';
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Separator } from '@/components/ui/separator';
import { BlurTextarea } from '@/components/ui/textarea';
import { useTranslate } from '@/hooks/common-hooks';
import { PlusOutlined } from '@ant-design/icons';
import { useUpdateNodeInternals } from '@xyflow/react';
import humanId from 'human-id';
import { Trash2 } from 'lucide-react';
import { useCallback } from 'react';
import { useFieldArray, useFormContext } from 'react-hook-form';
import { v4 as uuid } from 'uuid';
import useGraphStore from '../../store';

export function ScoredSwitchOptions({ nodeId }: { nodeId?: string }) {
  const { t } = useTranslate('flow');
  const form = useFormContext();
  const updateNodeInternals = useUpdateNodeInternals();
  const deleteEdges = useGraphStore(
    (state) => state.deleteEdgesBySourceAndSourceHandle,
  );
  const { fields, append, remove } = useFieldArray({
    name: 'items',
    control: form.control,
  });

  const handleAdd = useCallback(() => {
    append({ name: humanId(), description: '', uuid: uuid() });
    if (nodeId) {
      updateNodeInternals(nodeId);
    }
  }, [append, nodeId, updateNodeInternals]);

  const handleRemove = useCallback(
    (index: number) => () => {
      const optionId = form.getValues(`items.${index}.uuid`);
      remove(index);
      if (nodeId && optionId) {
        deleteEdges(nodeId, optionId);
        updateNodeInternals(nodeId);
      }
    },
    [deleteEdges, form, nodeId, remove, updateNodeInternals],
  );

  return (
    <section className="flex flex-col gap-4">
      {fields.map((field, index) => (
        <div key={field.id} className="space-y-2">
          <div className="flex items-center justify-between gap-2">
            <span className="truncate text-sm">
              {form.getValues(`items.${index}.name`)}
            </span>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="w-9 p-0"
              disabled={fields.length <= 2}
              onClick={handleRemove(index)}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
          <FormField
            control={form.control}
            name={`items.${index}.name`}
            render={({ field: nameField }) => (
              <FormItem>
                <FormControl>
                  <Input {...nameField} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name={`items.${index}.description`}
            render={({ field: descriptionField }) => (
              <FormItem>
                <FormLabel>{t('description')}</FormLabel>
                <FormControl>
                  <BlurTextarea {...descriptionField} rows={2} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name={`items.${index}.uuid`}
            render={() => <div></div>}
          />
          <Separator />
        </div>
      ))}
      <Button type="button" onClick={handleAdd}>
        <PlusOutlined />
        {t('scoredSwitchAddOption')}
      </Button>
    </section>
  );
}
