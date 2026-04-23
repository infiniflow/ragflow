import { useTranslation } from 'react-i18next';

import { Button } from '@/components/ui/button';
import { Plus } from 'lucide-react';
import { ProfileSettingWrapperCard } from '../components/user-setting-header';
import AddDataSourceModal from './add-datasource-modal';
import { AddedSourceCard } from './component/added-source-card';
import { DataSourceKey, useDataSourceInfo } from './constant';
import { useAddDataSource, useListDataSource } from './hooks';
import { IDataSorceInfo } from './interface';

const AvailableSourceCard = ({
  name,
  description,
  icon,
  onAdd,
}: IDataSorceInfo & { onAdd: () => void }) => {
  const { t } = useTranslation();

  return (
    <article
      className="
        size-full p-2.5 border-0.5 border-border-button rounded-lg relative group hover:bg-bg-card focus-within:bg-bg-card
        grid grid-cols-[auto_1fr] grid-rows-[auto_1fr] gap-x-2.5 gap-y-1"
      style={{
        gridTemplateAreas: '"icon title" "icon description"',
      }}
      onClick={() => onAdd()}
    >
      <span className="w-6" style={{ gridArea: 'icon' }}>
        {icon}
      </span>

      <header className="flex items-center gap-2" style={{ gridArea: 'title' }}>
        <h3 className="text-base text-text-primary">{name}</h3>

        <Button
          size="auto"
          className="ml-auto px-1 py-0.5 gap-0.5 text-xs items-center opacity-0 transition-all group-hover:opacity-100 group-focus-within:opacity-100"
          onClick={(e: any) => {
            e.stopPropagation();
            onAdd();
          }}
        >
          <Plus className="size-[1em]" />
          {t('setting.add')}
        </Button>
      </header>

      <p
        style={{ gridArea: 'description' }}
        className="text-xs text-text-secondary"
      >
        {description}
      </p>
    </article>
  );
};

const DataSource = () => {
  const { t } = useTranslation();
  const { dataSourceInfo } = useDataSourceInfo();
  const dataSourceTemplates = Object.values(DataSourceKey).map((id) => {
    return {
      id,
      name: dataSourceInfo[id].name,
      description: dataSourceInfo[id].description,
      icon: dataSourceInfo[id].icon,
    };
  });

  // useListTenantUser();
  const { categorizedList } = useListDataSource();

  const {
    addSource,
    addLoading,
    addingModalVisible,
    handleAddOk,
    hideAddingModal,
    showAddingModal,
  } = useAddDataSource({});

  return (
    <ProfileSettingWrapperCard
      header={
        <header>
          <h2 className="text-2xl font-medium text-text-primary">
            {t('setting.dataSources')}
          </h2>
          <p className="mt-1 text-sm text-text-secondary ">
            {t('setting.datasourceDescription')}
          </p>
        </header>
      }
    >
      <div className="h-full p-5 overflow-x-hidden overflow-y-auto">
        <section className="flex flex-col gap-3">
          {categorizedList?.length <= 0 && (
            <div className="text-text-secondary w-full flex justify-center items-center h-20">
              {t('setting.sourceEmptyTip')}
            </div>
          )}
          {categorizedList.map((item, index) => (
            <AddedSourceCard key={index} {...item} />
          ))}
        </section>

        <section className="mt-8">
          <header className="flex flex-row items-center justify-between space-y-0 p-0 pb-4">
            {/* <Users className="mr-2 h-5 w-5 text-[#1677ff]" /> */}
            <h2 className="text-2xl font-medium">
              {t('setting.availableSources')}
              <div className="text-sm text-text-secondary font-normal mt-1.5">
                {t('setting.availableSourcesDescription')}
              </div>
            </h2>
          </header>

          {/* <TenantTable searchTerm={searchTerm}></TenantTable> */}
          <ul className="@container grid sm:grid-cols-1 lg:grid-cols-2 xl:grid-cols-2 2xl:grid-cols-4 3xl:grid-cols-4 gap-4">
            {dataSourceTemplates.map((item) => (
              <li key={item.id} className="h-full">
                <AvailableSourceCard
                  {...item}
                  onAdd={() => showAddingModal(item)}
                />
              </li>
            ))}
          </ul>
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
        />
      )}
    </ProfileSettingWrapperCard>
  );
};

export default DataSource;
