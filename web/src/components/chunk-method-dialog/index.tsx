import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { IParserConfig } from '@/interfaces/database/document';
import { IChangeParserConfigRequestBody } from '@/interfaces/request/document';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../ui/select';
import { useFetchParserListOnMount } from './hooks';

interface IProps
  extends IModalProps<{
    parserId: string;
    parserConfig: IChangeParserConfigRequestBody;
  }> {
  loading: boolean;
  parserId: string;
  parserConfig: IParserConfig;
  documentExtension: string;
  documentId: string;
}

export function ChunkMethodDialog({
  hideModal,
  onOk,
  parserId,
  documentId,
  documentExtension,
}: IProps) {
  const { t } = useTranslate('knowledgeDetails');

  const { parserList } = useFetchParserListOnMount(
    documentId,
    parserId,
    documentExtension,
    // form,
  );

  const FormSchema = z.object({
    name: z
      .string()
      .min(1, {
        message: 'namePlaceholder',
      })
      .trim(),
  });
  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: { name: '' },
  });

  async function onSubmit(data: z.infer<typeof FormSchema>) {
    const ret = await onOk?.();
    if (ret) {
      hideModal?.();
    }
  }

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('chunkMethod')}</DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-6">
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('name')}</FormLabel>
                  <FormControl>
                    <Select
                      {...field}
                      autoComplete="off"
                      onValueChange={field.onChange}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder="Select a verified email to display" />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {parserList.map((x) => (
                          <SelectItem value={x.value} key={x.value}>
                            {x.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </form>
        </Form>
        <DialogFooter>
          <Button type="submit">Save changes</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
