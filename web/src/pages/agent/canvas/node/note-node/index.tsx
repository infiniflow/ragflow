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
import { useChangeName, useWatchFormChange } from './use-watch-change';

const FormSchema = z.object({
  text: z.string(),
});

function NoteNode({ data, id }: NodeProps<INoteNode>) {
  const { t } = useTranslation();

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: data.form,
  });

  const { handleChangeName } = useChangeName(id);

  useWatchFormChange(id, form);

  return (
    <NodeWrapper className="p-0  w-full h-full flex flex-col rounded-md ">
      <NodeResizeControl minWidth={190} minHeight={128} style={controlStyle}>
        <ResizeIcon />
      </NodeResizeControl>
      <section className="px-1 py-2 flex gap-2 bg-background-highlight items-center note-drag-handle rounded-s-md">
        <NotebookPen className="size-4" />
        <Input
          type="text"
          defaultValue={data.name}
          onChange={handleChangeName}
        ></Input>
      </section>
      <Form {...form}>
        <form className="flex-1">
          <FormField
            control={form.control}
            name="text"
            render={({ field }) => (
              <FormItem className="h-full">
                <FormControl>
                  <Textarea
                    placeholder={t('flow.notePlaceholder')}
                    className="resize-none rounded-none p-1 h-full overflow-auto bg-background-header-bar"
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
