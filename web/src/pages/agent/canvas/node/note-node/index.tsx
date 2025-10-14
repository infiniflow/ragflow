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

type NoteNodeProps = NodeProps<INoteNode> & {
  useWatchNoteFormChange?: typeof useWatchFormChange;
  useWatchNoteNameFormChange?: typeof useWatchNameFormChange;
};

function NoteNode({
  data,
  id,
  selected,
  useWatchNoteFormChange,
  useWatchNoteNameFormChange,
}: NoteNodeProps) {
  const { t } = useTranslation();

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: data.form,
  });

  const nameForm = useForm<z.infer<typeof NameFormSchema>>({
    resolver: zodResolver(NameFormSchema),
    defaultValues: { name: data.name },
  });

  (useWatchNoteFormChange || useWatchFormChange)(id, form);

  (useWatchNoteNameFormChange || useWatchNameFormChange)(id, nameForm);

  return (
    <NodeWrapper
      className="p-0  w-full h-full flex flex-col bg-bg-component border border-state-warning rounded-lg shadow-md pb-1"
      selected={selected}
    >
      <NodeResizeControl minWidth={190} minHeight={128} style={controlStyle}>
        <ResizeIcon />
      </NodeResizeControl>
      <section className="px-2 py-1 flex gap-2 items-center note-drag-handle rounded-t border-t-2 border-state-warning">
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
                      className="bg-transparent border-none focus-visible:outline focus-visible:outline-text-sub-title p-1"
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
        <form className="flex-1 px-1 min-h-1">
          <FormField
            control={form.control}
            name="text"
            render={({ field }) => (
              <FormItem className="h-full">
                <FormControl>
                  <Textarea
                    placeholder={t('flow.notePlaceholder')}
                    className="resize-none rounded-none p-1 py-0 overflow-auto bg-transparent focus-visible:ring-0 border-none text-text-secondary focus-visible:ring-offset-0 !text-xs"
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
