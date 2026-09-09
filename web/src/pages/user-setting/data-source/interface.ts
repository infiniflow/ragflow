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

import { RunningStatus } from '@/constants/knowledge';
import { DataSourceKey } from './constant';

export interface IDataSorceInfo {
  id: DataSourceKey;
  name: string;
  description: string;
  icon: React.ReactNode;
}

export type IDataSource = IDataSourceBase & {
  config: any;
  indexing_start: null | string;
  input_type: string;
  prune_freq: number;
  refresh_freq: number;
  status: string;
  tenant_id: string;
  update_date: string;
  update_time: number;
};

export interface IDataSourceBase {
  id: string;
  name: string;
  source: DataSourceKey;
}

export interface IDataSourceLog {
  connector_id: string;
  docs_removed_from_index?: number;
  error_count: number;
  error_msg: string;
  id: string;
  kb_id: string;
  kb_name: string;
  new_docs_indexed: number;
  prune_freq?: number;
  refresh_freq?: number;
  status: RunningStatus;
  task_type?: string;
  time_started?: string | null;
  total_docs_indexed?: number;
  update_date: string;
}

interface IDataSourceInfoItem {
  name: string;
  description: string;
  icon: JSX.Element;
}

export type IDataSourceInfoMap = Record<DataSourceKey, IDataSourceInfoItem>;
