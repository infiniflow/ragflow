import { NodeProps, NodeResizeControl } from '@xyflow/react';

import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { INoteNode } from '@/interfaces/database/flow';
import { zodResolver } from '@hookform/resolvers/zod';
import { NotebookPen } from 'lucide-react';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { NodeWrapper } from '../node-wrapper';
import { ResizeIcon, controlStyle } from '../resize-icon';
import { useWatchFormChange, useWatchNameFormChange } from './use-watch-change';

const FormSchema = z.object({
  text: z.string(),
});

const NameFormSchema = z.object({
  name: z.string(),
});

function NoteNode({ data, id, selected }: NodeProps<INoteNode>) {
  const { t } = useTranslation();

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: data.form,
  });

  const nameForm = useForm<z.infer<typeof NameFormSchema>>({
    resolver: zodResolver(NameFormSchema),
    defaultValues: { name: data.name },
  });

  useWatchFormChange(id, form);

  useWatchNameFormChange(id, nameForm);

  return (
    <NodeWrapper
      className="p-0  w-full h-full flex flex-col"
      selected={selected}
    >
      <NodeResizeControl minWidth={190} minHeight={128} style={controlStyle}>
        <ResizeIcon />
      </NodeResizeControl>
      <section className="p-2 flex gap-2 bg-background-note items-center note-drag-handle rounded-t">
        <NotebookPen className="size-4" />
        <Form {...nameForm}>
          <form className="flex-1">
            <FormField
              control={nameForm.control}
              name="name"
              render={({ field }) => (
                <FormItem className="h-full">
                  <FormControl>
                    <Input
                      placeholder={t('flow.notePlaceholder')}
                      {...field}
                      type="text"
                      className="bg-transparent border-none focus-visible:outline focus-visible:outline-text-sub-title"
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </form>
        </Form>
      </section>
      <Form {...form}>
        <form className="flex-1 p-1">
          <FormField
            control={form.control}
            name="text"
            render={({ field }) => (
              <FormItem className="h-full">
                <FormControl>
                  <Textarea
                    placeholder={t('flow.notePlaceholder')}
                    className="resize-none rounded-none p-1 h-full overflow-auto bg-transparent focus-visible:ring-0 border-none"
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </form>
      </Form>
    </NodeWrapper>
  );
}

export default memo(NoteNode);
