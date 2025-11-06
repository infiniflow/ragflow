import { RunningStatus } from '@/constants/knowledge';
import { DataSourceKey } from './contant';

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
  error_count: number;
  error_msg: string;
  id: string;
  kb_id: string;
  kb_name: string;
  name: string;
  new_docs_indexed: number;
  poll_range_end: null | string;
  poll_range_start: null | string;
  reindex: string;
  source: DataSourceKey;
  status: RunningStatus;
  tenant_id: string;
  timeout_secs: number;
}
