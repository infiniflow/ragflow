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

import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { Settings, Trash2 } from 'lucide-react';
import { useDataSourceInfo } from '../constant';
import { useDeleteDataSource } from '../hooks';
import { IDataSorceInfo, IDataSourceBase } from '../interface';
import { delSourceModal } from './delete-source-modal';

export type IAddedSourceCardProps = IDataSorceInfo & {
  list: IDataSourceBase[];
};
export const AddedSourceCard = (props: IAddedSourceCardProps) => {
  const { list, name, icon } = props;
  const { handleDelete } = useDeleteDataSource();
  const { navigateToDataSourceDetail } = useNavigatePage();
  const { dataSourceInfo } = useDataSourceInfo();
  const toDetail = (id: string) => {
    navigateToDataSourceDetail(id);
  };
  return (
    <Card className="bg-transparent border border-border-button px-5 pt-[10px] pb-5 rounded-md">
      <CardHeader className="flex flex-row items-center justify-between space-y-0 p-0 pb-3">
        {/* <Users className="mr-2 h-5 w-5 text-[#1677ff]" /> */}
        <CardTitle className="text-base items-center flex gap-1 font-normal">
          {icon}
          {name}
        </CardTitle>
      </CardHeader>
      <CardContent className="p-2 flex flex-col gap-2">
        {list.map((item) => (
          <div
            key={item.id}
            className="flex flex-row items-center justify-between rounded-md bg-bg-card px-[10px] py-4"
          >
            <div className="text-sm text-text-primary ">{item.name}</div>
            <div className="text-sm text-text-secondary  flex gap-2">
              <Button
                variant={'ghost'}
                className="rounded-lg px-2 py-1 bg-transparent hover:bg-bg-card"
                onClick={() => {
                  toDetail(item.id);
                }}
              >
                <Settings size={14} />
              </Button>
              {/* <ConfirmDeleteDialog onOk={() => handleDelete(item)}> */}
              <Button
                variant={'ghost'}
                className="rounded-lg px-2 py-1 bg-transparent hover:bg-state-error-5 hover:text-state-error"
                onClick={() =>
                  delSourceModal({
                    data: item,
                    dataSourceInfo: dataSourceInfo,
                    onOk: () => {
                      handleDelete(item);
                    },
                  })
                }
              >
                <Trash2 className="cursor-pointer" size={14} />
              </Button>
              {/* </ConfirmDeleteDialog> */}
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
};
