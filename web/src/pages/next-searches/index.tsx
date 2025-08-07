import ListFilterBar from '@/components/list-filter-bar';
import { Input } from '@/components/originui/input';
import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import { useTranslate } from '@/hooks/common-hooks';
import { useFetchFlowList } from '@/hooks/flow-hooks';
import { Plus, Search } from 'lucide-react';
import { useState } from 'react';
import { SearchCard } from './search-card';

export default function SearchList() {
  const { data } = useFetchFlowList();
  const { t } = useTranslate('search');
  const [searchName, setSearchName] = useState('');
  const handleSearchChange = (value: string) => {
    console.log(value);
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
              Modal.show({
                title: (
                  <div className="rounded-sm bg-emerald-400 bg-gradient-to-t from-emerald-400 via-emerald-400 to-emerald-200 p-1 size-6 flex justify-center items-center">
                    <Search size={14} className="font-bold m-auto" />
                  </div>
                ),
                titleClassName: 'border-none',
                footerClassName: 'border-none',
                visible: true,
                children: (
                  <div>
                    <div>{t('createSearch')}</div>
                    <div>name:</div>
                    <Input
                      defaultValue={searchName}
                      onChange={(e) => {
                        console.log(e.target.value, e);
                        setSearchName(e.target.value);
                      }}
                    />
                  </div>
                ),
                onOk: () => {
                  console.log('ok', searchName);
                },
                onVisibleChange: (e) => {
                  Modal.hide();
                },
              });
            }}
          >
            <Plus className="mr-2 h-4 w-4" />
            {t('createSearch')}
          </Button>
        </ListFilterBar>
      </div>
      <div className="grid gap-6 sm:grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 2xl:grid-cols-5 max-h-[84vh] overflow-auto px-8">
        {data.map((x) => {
          return <SearchCard key={x.id} data={x}></SearchCard>;
        })}
      </div>
    </section>
  );
}
