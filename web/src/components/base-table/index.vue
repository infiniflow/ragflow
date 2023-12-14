<script lang="ts" setup>
import { computed, onMounted, ref, unref, useAttrs } from 'vue';
import { createForm, onFormValuesChange, setValidateLanguage } from '@formily/core';
import { Form, FormItemProps, TableProps, message } from 'ant-design-vue';
import {
  Field,
  FormProvider,
  connect,
  mapProps,
} from '@formily/vue';
import type { PaginationProps } from 'ant-design-vue';
import { assign, isUndefined } from 'lodash-es';
import type { IQueryParams, ISearchColumn } from './types';
import { componentsMap } from './constants';

const props = withDefaults(defineProps<{
  hasCustomFields?: boolean
  searchColumn?: Array<ISearchColumn>
  pagination?: PaginationProps
  searchLabelCol?: FormItemProps['labelCol']
  request?(searchValues: IQueryParams['searchValues'],
    pagination: IQueryParams['pagination'],
    filters: IQueryParams['filters'],
    sorter: IQueryParams['sorter'],): Promise<any>
}>(), {
  searchColumn: () => {
    return [];
  },
  tabOption: () => {
    return [];
  },
});
const emits = defineEmits(['reset', 'search', 'tabChange', 'searchFormChange']);
defineOptions({
  inheritAttrs: false,
});
setValidateLanguage('cn');

const FormItem = connect(
  Form.Item,
  mapProps({
    validateStatus: true,
    title: 'label',
    required: true,
  }),
);
const form = createForm({
  effects() {
    onFormValuesChange((form) => {
      emits('searchFormChange', form);
    });
  },
});
const expand = ref(false);
const searchColumn = computed(() => {
  return props.searchColumn;
});

// 表格相关
const tableRef = ref();
const attrs = useAttrs() as unknown as TableProps;
const pageIndex = ref(1);
// eslint-disable-next-line vue/no-setup-props-destructure
const pageSize = ref(props.pagination?.defaultPageSize || 10);
const total = ref(0);
const filterValue = ref<Record<string, any>>();
const sortValue = ref<Record<string, any>>();
const finalPagination = computed(() => ({
  total: total.value,
  current: pageIndex.value,
  pageSize: pageSize.value,
  position: ['bottomRight'],
  showTotal: (total: number, range: Array<number>) => `第 ${range[0]}-${range[1]} 条/总共 ${total} 条`,
  showSizeChanger: true,
  ...props.pagination,
}));

function finalHandleTableChange(pagination: IQueryParams['pagination'], filters: IQueryParams['filters'], sorter: IQueryParams['sorter'], source: IQueryParams['source']) {
  filterValue.value = filters;
  sortValue.value = sorter;
  if (attrs?.onChange) {
    pageSize.value = pagination.pageSize!;
    pageIndex.value = pagination.current!;
    attrs?.onChange(pagination, filters, sorter, source);
  } else {
    query(pagination, filters, sorter);
  }
}

const tableLoading = ref(false);
const dataSource = ref<Array<any>>([]);
const tableRequest = computed(() => {
  return props.request;
});

async function query(pagination?: IQueryParams['pagination'], filters?: IQueryParams['filters'], sorter?: IQueryParams['sorter']) {
  tableLoading.value = true;

  if (pagination) {
    pageSize.value = pagination.pageSize!;
    pageIndex.value = pagination.current!;
  }

  const req = unref(tableRequest)!;

  try {
    const { data } = await req(form.values, {
      pageSize: pageSize.value,
      current: pageIndex.value,
    }, filters!, sorter!);
    const { result, page } = unref(data);
    dataSource.value = result;
    total.value = page.total;
    pageIndex.value = page.current;
    tableLoading.value = false;
  } catch (error) {
    console.log('error---', error);
    message.error('fail to fetch table');
    tableLoading.value = false;
  }
}

function reset() {
  form.reset();
  query();
  emits('reset');
}

function search() {
  query({
    pageSize: pageSize.value,
    current: 1,
  });
  emits('search');
}

function reload(params: IQueryParams['pagination']) {
  query(params);
}

const finalAttrs = computed(() => {
  return {
    ...attrs,
    onChange: finalHandleTableChange,
    pagination: finalPagination,
    dataSource: dataSource.value,
    loading: isUndefined(attrs.loading) ? tableLoading : attrs.loading,
    scroll: assign({ y: 800 }, attrs?.scroll || {}),
  };
});

defineExpose({
  search,
  reload,
  getTableValue: () => {
    return {
      pagination: {
        current: pageIndex.value,
        size: pageSize.value,
      },
      searchValue: form.values,
      filters: filterValue.value,
      sorter: sortValue.value,
    };
  },
  getSearchForm: () => {
    return form;
  },
});

onMounted(() => {
  query();
});
</script>

<template>
  <div class="base-table-wrap">
    <div
      v-if="searchColumn.length > 0 || hasCustomFields"
      class="base-table-search"
    >
      <form-provider :form="form">
        <a-row :gutter="24" type="flex">
          <template v-for="item of searchColumn" :key="item.name">
            <a-col
              v-show="expand || !item?.hide"
              :span="4"
            >
              <field
                :name="item.name"
                :title="item?.title || ''"
                :decorator="[FormItem]"
                :initial-value="item.initialValue"
                :display="item.display"
                :component="[componentsMap[item.type], { ...item.props }]"
                :reactions="item.reactions"
              />
            </a-col>
          </template>
          <slot name="custom-fields" />
          <a-col v-if="searchColumn.length > 0">
            <div class="flex" :class="{ 'justify-end': searchColumn.length > 2 }">
              <a-space>
                <a-button
                  :disabled="tableLoading"
                  @click="reset"
                >
                  重置
                </a-button>
                <a-button type="primary" :loading="tableLoading" @click="search">
                  查询
                </a-button>
              </a-space>
            </div>
          </a-col>
        </a-row>
      </form-provider>
    </div>
    <div>
      <a-table v-bind="finalAttrs" ref="tableRef" :loading="tableLoading">
        <template #title>
          <div class="base-table-opt">
            <slot name="operation" />
          </div>
        </template>
        <template #headerCell="{ column, record, index }">
          <slot :column="column" :record="record" :index="index" name="headerCell" />
        </template>
        <template #bodyCell="{ column, record, index }">
          <slot :column="column" :record="record" :index="index" name="bodyCell" />
        </template>
        <template v-if="$slots.expandColumnTitle" #expandColumnTitle="{ record }">
          <slot :record="record" name="expandColumnTitle" />
        </template>
        <template v-if="$slots.expandedRowRender" #expandedRowRender="{ record }">
          <slot :record="record" name="expandedRowRender" />
        </template>
        <slot />
      </a-table>
    </div>
  </div>
</template>
