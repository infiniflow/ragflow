import type { TablePaginationConfig } from 'ant-design-vue';
import type { FilterValue, SorterResult, TableCurrentDataSource } from 'ant-design-vue/lib/table/interface';
import type { DefaultRecordType } from 'ant-design-vue/lib/vc-table/interface';

export interface ISearchColumn {
  name: string
  title: string
  hide?: boolean
  type: string
  initialValue?: any
  display?: 'visible' | 'hidden' | 'none'
  props?: Record<string, any>
  reactions?: [() => void]
}

export interface IQueryParams {
  searchValues: Record<string, string>
  pagination: TablePaginationConfig
  filters: Record<string, FilterValue | null>
  sorter: SorterResult<DefaultRecordType> | SorterResult<DefaultRecordType>[]
  source: TableCurrentDataSource<DefaultRecordType>
}
