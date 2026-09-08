import { ButtonLoading } from '@/components/ui/button';
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
import { MultiSelect } from '@/components/ui/multi-select';
import {
  useFetchDatasetsByIds,
  useFetchKnowledgeList,
} from '@/hooks/use-knowledge-request';
import { IModalProps } from '@/interfaces/common';
import { IDataset } from '@/interfaces/database/dataset';
import { useDebounce } from 'ahooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { Link2 } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { UseHandleConnectToKnowledgeReturnType } from './hooks';

const FormId = 'LinkToDatasetForm';

const FormSchema = z.object({
  knowledgeIds: z.array(z.string()).min(0, {
    message: 'Username must be at least 1 characters.',
  }),
});

function LinkToDatasetForm({
  initialConnectedIds,
  onConnectToKnowledgeOk,
}: Pick<
  UseHandleConnectToKnowledgeReturnType,
  'initialConnectedIds' | 'onConnectToKnowledgeOk'
>) {
  const { t } = useTranslation();
  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      knowledgeIds: initialConnectedIds,
    },
  });

  const [searchString, setSearchString] = useState('');
  const debouncedSearchString = useDebounce(searchString, { wait: 500 });
  const { list, loading, handleScroll, hasNextPage } = useFetchKnowledgeList(
    false,
    debouncedSearchString,
  );

  const selectedIds = form.watch('knowledgeIds');

  // Cache all datasets the user has seen so names survive across searches
  // and scroll. The paginated list may not contain every selected dataset
  // (e.g. initially-connected ones), so resolve the rest by ID. Kept as
  // state so badges re-render once a missing name is fetched.
  const [datasetCache, setDatasetCache] = useState<Map<string, IDataset>>(
    new Map(),
  );

  const missingIds = useMemo(() => {
    const loadedIds = new Set(list.map((d) => d.id));
    return selectedIds.filter(
      (id) => !loadedIds.has(id) && !datasetCache.has(id),
    );
  }, [list, selectedIds, datasetCache]);

  const { data: missingDatasets } = useFetchDatasetsByIds(missingIds);

  useEffect(() => {
    setDatasetCache((prev) => {
      const next = new Map(prev);
      let changed = false;
      list.forEach((item) => {
        if (!next.has(item.id)) {
          next.set(item.id, item);
          changed = true;
        }
      });
      missingDatasets?.forEach((item) => {
        if (!next.has(item.id)) {
          next.set(item.id, item);
          changed = true;
        }
      });
      return changed ? next : prev;
    });
  }, [list, missingDatasets]);

  const options = useMemo(
    () =>
      list.map((item) => ({
        label: item.name,
        value: item.id,
      })),
    [list],
  );

  function onSubmit(data: z.infer<typeof FormSchema>) {
    const kbsInfo = data.knowledgeIds.map((id) => {
      const dataset = datasetCache.get(id);
      return {
        kb_id: id,
        kb_name: dataset?.name ?? id,
      };
    });
    onConnectToKnowledgeOk(kbsInfo);
  }

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-6"
        id={FormId}
      >
        <FormField
          control={form.control}
          name="knowledgeIds"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('common.name')}</FormLabel>
              <FormControl>
                <MultiSelect
                  options={options}
                  onValueChange={field.onChange}
                  defaultValue={field.value}
                  placeholder={t('fileManager.pleaseSelect')}
                  maxCount={100}
                  searchValue={searchString}
                  onSearchChange={setSearchString}
                  isSearching={loading}
                  shouldFilter={false}
                  onListScroll={hasNextPage ? handleScroll : undefined}
                  // Selected datasets filtered out of the current list page
                  // still show their names via the seen-datasets cache.
                  getOptionLabel={(value) => datasetCache.get(value)?.name}
                  modalPopover
                />
              </FormControl>

              <FormMessage />
            </FormItem>
          )}
        />
      </form>
    </Form>
  );
}

export function LinkToDatasetDialog({
  hideModal,
  initialConnectedIds,
  onConnectToKnowledgeOk,
  loading,
}: IModalProps<any> &
  Pick<
    UseHandleConnectToKnowledgeReturnType,
    'initialConnectedIds' | 'onConnectToKnowledgeOk'
  >) {
  const { t } = useTranslation();
  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>{t('fileManager.addToKnowledge')}</DialogTitle>
        </DialogHeader>
        <LinkToDatasetForm
          initialConnectedIds={initialConnectedIds}
          onConnectToKnowledgeOk={onConnectToKnowledgeOk}
        ></LinkToDatasetForm>
        <DialogFooter>
          <ButtonLoading type="submit" form={FormId} loading={loading}>
            <div className="flex gap-2 items-center">
              <Link2 /> Save
            </div>
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
