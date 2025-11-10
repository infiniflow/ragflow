import { CardTitle } from '@/components/ui/card';
import { useTranslation } from 'react-i18next';

import Spotlight from '@/components/spotlight';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { Plus } from 'lucide-react';
import AddDataSourceModal from './add-datasource-modal';
import { AddedSourceCard } from './component/added-source-card';
import { DataSourceInfo, DataSourceKey } from './contant';
import { useAddDataSource, useListDataSource } from './hooks';
import { IDataSorceInfo } from './interface';
const dataSourceTemplates = [
  {
    id: DataSourceKey.CONFLUENCE,
    name: DataSourceInfo[DataSourceKey.CONFLUENCE].name,
    description: DataSourceInfo[DataSourceKey.CONFLUENCE].description,
    icon: DataSourceInfo[DataSourceKey.CONFLUENCE].icon,
  },
  {
    id: DataSourceKey.S3,
    name: DataSourceInfo[DataSourceKey.S3].name,
    description: DataSourceInfo[DataSourceKey.S3].description,
    icon: DataSourceInfo[DataSourceKey.S3].icon,
  },
  {
    id: DataSourceKey.GOOGLE_DRIVE,
    name: DataSourceInfo[DataSourceKey.GOOGLE_DRIVE].name,
    description: DataSourceInfo[DataSourceKey.GOOGLE_DRIVE].description,
    icon: DataSourceInfo[DataSourceKey.GOOGLE_DRIVE].icon,
  },
  {
    id: DataSourceKey.DISCORD,
    name: DataSourceInfo[DataSourceKey.DISCORD].name,
    description: DataSourceInfo[DataSourceKey.DISCORD].description,
    icon: DataSourceInfo[DataSourceKey.DISCORD].icon,
  },
  {
    id: DataSourceKey.NOTION,
    name: DataSourceInfo[DataSourceKey.NOTION].name,
    description: DataSourceInfo[DataSourceKey.NOTION].description,
    icon: DataSourceInfo[DataSourceKey.NOTION].icon,
  },
];
const DataSource = () => {
  const { t } = useTranslation();

  // useListTenantUser();
  const { categorizedList } = useListDataSource();

  const {
    addSource,
    addLoading,
    addingModalVisible,
    handleAddOk,
    hideAddingModal,
    showAddingModal,
  } = useAddDataSource();

  const AbailableSourceCard = ({
    id,
    name,
    description,
    icon,
  }: IDataSorceInfo) => {
    return (
      <div
        className="p-[10px] border border-border-button rounded-lg relative group hover:bg-bg-card"
        onClick={() =>
          showAddingModal({
            id,
            name,
            description,
            icon,
          })
        }
      >
        <div className="flex gap-2">
          <div className="w-6 h-6">{icon}</div>
          <div className="flex flex-1 flex-col items-start gap-2">
            <div className="text-base text-text-primary">{name}</div>
            <div className="text-xs text-text-secondary">{description}</div>
          </div>
        </div>
        <div className=" absolute top-2 right-2">
          <Button className=" rounded-md px-1 text-bg-base gap-1 bg-text-primary text-xs py-0 h-6 items-center hidden group-hover:flex">
            <Plus size={12} />
            {t('setting.add')}
          </Button>
        </div>
      </div>
    );
  };

  return (
    <div className="w-full flex flex-col gap-4  relative ">
      <Spotlight />

      {/* <Card className="bg-transparent border-none px-0"> */}
      <section className="flex flex-row items-center justify-between space-y-0 px-4 pt-4 pb-0">
        <div className="text-2xl font-medium">
          {t('setting.dataSources')}
          <div className="text-sm text-text-secondary">
            {t('setting.datasourceDescription')}
          </div>
        </div>
      </section>
      {/* </Card> */}
      <Separator className="border-border-button bg-border-button " />
      <div className=" flex flex-col gap-4 p-4 max-h-[calc(100vh-120px)] overflow-y-auto overflow-x-hidden scrollbar-auto">
        <div className="flex flex-col gap-3">
          {categorizedList.map((item, index) => (
            <AddedSourceCard key={index} {...item} />
          ))}
        </div>
        <section className="bg-transparent border-none mt-8">
          <header className="flex flex-row items-center justify-between space-y-0 p-0 pb-4">
            {/* <Users className="mr-2 h-5 w-5 text-[#1677ff]" /> */}
            <CardTitle className="text-2xl font-semibold">
              {t('setting.availableSources')}
              <div className="text-sm text-text-secondary font-normal">
                {t('setting.availableSourcesDescription')}
              </div>
            </CardTitle>
          </header>
          <main className="p-0">
            {/* <TenantTable searchTerm={searchTerm}></TenantTable> */}
            <div className="grid sm:grid-cols-1 lg:grid-cols-2 xl:grid-cols-2 2xl:grid-cols-4 3xl:grid-cols-4 gap-4">
              {dataSourceTemplates.map((item, index) => (
                <AbailableSourceCard {...item} key={index} />
              ))}
            </div>
          </main>
        </section>
      </div>

      {addingModalVisible && (
        <AddDataSourceModal
          visible
          loading={addLoading}
          hideModal={hideAddingModal}
          onOk={(data) => {
            console.log(data);
            handleAddOk(data);
          }}
          sourceData={addSource}
        ></AddDataSourceModal>
      )}
    </div>
  );
};

export default DataSource;
