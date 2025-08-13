import ListFilterBar from '@/components/list-filter-bar';
import { Input } from '@/components/originui/input';
import { Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Modal } from '@/components/ui/modal/modal';
import { RAGFlowPagination } from '@/components/ui/ragflow-pagination';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { pick } from 'lodash';
import { Plus, Search } from 'lucide-react';
import { useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { useCreateSearch, useFetchSearchList } from './hooks';
import { SearchCard } from './search-card';
const searchFormSchema = z.object({
  name: z.string().min(1, {
    message: 'Name is required',
  }),
});

type SearchFormValues = z.infer<typeof searchFormSchema>;
export default function SearchList() {
  // const { data } = useFetchFlowList();
  const { t } = useTranslate('search');
  const { isLoading, createSearch } = useCreateSearch();
  const {
    data: list,
    searchParams,
    setSearchListParams,
  } = useFetchSearchList();
  const [openCreateModal, setOpenCreateModal] = useState(false);
  const form = useForm<SearchFormValues>({
    resolver: zodResolver(searchFormSchema),
    defaultValues: {
      name: '',
    },
  });
  const handleSearchChange = (value: string) => {
    console.log(value);
  };

  const onSubmit = async (values: SearchFormValues) => {
    await createSearch({ name: values.name });
    if (!isLoading) {
      setOpenCreateModal(false);
    }
    form.reset({ name: '' });
  };
  const openCreateModalFun = () => {
    setOpenCreateModal(true);
  };
  const handlePageChange = (page: number, pageSize: number) => {
    setSearchListParams({ ...searchParams, page, page_size: pageSize });
  };
  return (
    <section>
      <div className="px-8 pt-8">
        <ListFilterBar
          icon={
            <div className="rounded-sm bg-emerald-400 bg-gradient-to-t from-emerald-400 via-emerald-400 to-emerald-200 p-1 size-6 flex justify-center items-center">
              <Search size={14} className="font-bold m-auto" />
            </div>
          }
          title="Search apps"
          showFilter={false}
          onSearchChange={(e) => handleSearchChange(e.target.value)}
        >
          <Button
            variant={'default'}
            onClick={() => {
              openCreateModalFun();
            }}
          >
            <Plus className="mr-2 h-4 w-4" />
            {t('createSearch')}
          </Button>
        </ListFilterBar>
      </div>
      <div className="grid gap-6 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 max-h-[84vh] overflow-auto px-8">
        {list?.data.search_apps.map((x) => {
          return <SearchCard key={x.id} data={x}></SearchCard>;
        })}
        {/* {data.map((x) => {
          return <SearchCard key={x.id} data={x}></SearchCard>;
        })} */}
      </div>
      {list?.data.total && (
        <RAGFlowPagination
          {...pick(searchParams, 'current', 'pageSize')}
          total={list?.data.total}
          onChange={handlePageChange}
          on
        />
      )}
      <Modal
        open={openCreateModal}
        onOpenChange={(open) => {
          setOpenCreateModal(open);
        }}
        title={
          <div className="rounded-sm bg-emerald-400 bg-gradient-to-t from-emerald-400 via-emerald-400 to-emerald-200 p-1 size-6 flex justify-center items-center">
            <Search size={14} className="font-bold m-auto" />
          </div>
        }
        className="!w-[480px] rounded-xl"
        titleClassName="border-none"
        footerClassName="border-none"
        showfooter={false}
        maskClosable={false}
      >
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)}>
            <div className="text-base mb-4">{t('createSearch')}</div>

            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    <span className="text-destructive mr-1"> *</span>Name
                  </FormLabel>
                  <FormControl>
                    <Input {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className="flex justify-end gap-2 mt-8 mb-6">
              <Button
                type="button"
                variant="outline"
                onClick={() => setOpenCreateModal(false)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={isLoading}>
                {isLoading ? 'Confirm...' : 'Confirm'}
              </Button>
            </div>
          </form>
        </Form>
      </Modal>
    </section>
  );
}
