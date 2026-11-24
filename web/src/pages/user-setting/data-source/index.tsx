/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import type { MouseEvent } from 'react';
import { useTranslation } from 'react-i18next';

import { PageFrame } from '@/components/page-frame';
import { TruncatedText } from '@/components/truncated-text';
import { Button } from '@/components/ui/button';
import { Plus } from 'lucide-react';
import { useVisibleRoleIntegrationItems } from '@/ee/hooks/use-visible-role-integration-items';
import AddDataSourceModal from './add-datasource-modal';
import {
  getAvailableDataSourceKeys,
  getCategorizedDataSourceKeys,
} from './backend-adapter';
import { AddedSourceCard } from './component/added-source-card';
import { useDataSourceInfo } from './constant';
import { useAddDataSource, useListDataSource } from './hooks';
import { IDataSorceInfo } from './interface';

type AvailableSourceCardProps = {
  source: IDataSorceInfo;
  onAdd: (source: IDataSorceInfo) => void;
};

const AvailableSourceCard = ({ source, onAdd }: AvailableSourceCardProps) => {
  const { t } = useTranslation();
  const { name, icon } = source;

  const handleAdd = () => {
    onAdd(source);
  };

  const handleButtonClick = (event: MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation();
    handleAdd();
  };

  return (
    <article
      className="group relative flex size-full cursor-pointer items-center gap-2.5 rounded-lg border-0.5 border-border-button p-5 hover:bg-bg-card focus-within:bg-bg-card"
      onClick={handleAdd}
    >
      <span className="w-6 shrink-0">{icon}</span>

      <TruncatedText
        as="h4"
        className="min-w-0 flex-1 truncate text-base font-normal text-text-primary"
        tooltip={name}
      >
        {name}
      </TruncatedText>

      <Button
        size="icon-sm"
        className="ml-auto shrink-0 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
        aria-label={t('setting.add')}
        onClick={handleButtonClick}
      >
        <Plus className="size-4" />
      </Button>
    </article>
  );
};

const DataSource = () => {
  const { t } = useTranslation();
  const { dataSourceInfo } = useDataSourceInfo();
  const dataSourceTemplates = useVisibleRoleIntegrationItems(
    'data_source',
    getAvailableDataSourceKeys().map((id) => ({
      id,
      name: dataSourceInfo[id].name,
      description: dataSourceInfo[id].description,
      icon: dataSourceInfo[id].icon,
    })),
    (item) =>
      String(item.id) === 'dingtalk_ai_table'
        ? ['dingtalk_ai_table', 'dingtalk_wiki', 'dingtalk_drive']
        : [item.id],
  );
  const visibleDataSourceKeys = new Set(
    dataSourceTemplates.map((source) => source.id),
  );
  const dataSourceGroups = getCategorizedDataSourceKeys()
    .map((group) => ({
      ...group,
      sources: group.sources
        .filter((id) => visibleDataSourceKeys.has(id))
        .map((id) => ({
          id,
          name: dataSourceInfo[id].name,
          description: dataSourceInfo[id].description,
          icon: dataSourceInfo[id].icon,
        })),
    }))
    .filter((group) => group.sources.length > 0);

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
    <PageFrame>
      <header className="shrink-0 p-5">
        <h2 className="text-2xl font-medium text-text-primary">
          {t('setting.dataSources')}
        </h2>
        <p className="mt-1 text-sm text-text-secondary">
          {t('setting.datasourceDescription')}
        </p>
      </header>

      <div className="min-h-0 flex-1 overflow-x-hidden overflow-y-auto p-5">
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
          <div className="flex flex-col gap-8">
            {dataSourceGroups.map((group) => (
              <section key={group.category}>
                <h3 className="mb-4 text-lg font-medium text-text-primary">
                  {t(`setting.dataSourceCategories.${group.category}`)}
                </h3>
                <ul className="@container grid gap-4 sm:grid-cols-1 lg:grid-cols-2 xl:grid-cols-2 2xl:grid-cols-4 3xl:grid-cols-4">
                  {group.sources.map((source) => (
                    <li key={source.id} className="h-full">
                      <AvailableSourceCard
                        source={source}
                        onAdd={showAddingModal}
                      />
                    </li>
                  ))}
                </ul>
              </section>
            ))}
          </div>
        </section>
      </div>

      {addingModalVisible && (
        <AddDataSourceModal
          visible
          loading={addLoading}
          hideModal={hideAddingModal}
          onOk={(data) => {
            return handleAddOk(data);
          }}
          sourceData={addSource}
        />
      )}
    </PageFrame>
  );
};

export default DataSource;
