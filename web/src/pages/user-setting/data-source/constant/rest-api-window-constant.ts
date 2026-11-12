import {
  FilterFormField,
  FormFieldConfig,
  FormFieldType,
} from '@/components/dynamic-form';
import type { TFunction } from 'i18next';
import { z } from 'zod';

export const RestAPIWindowToggleKey = `${FilterFormField}_rest_api_window`;

export const RestAPIWindowDefaultValues = {
  start_param: '',
  end_param: '',
  initial_start: '',
  time_format: 'rfc3339',
  response_time_format: 'rfc3339',
  max_window_seconds: '',
};

export function restAPIWindowEnabled(values: any) {
  return (
    values?.config?.method === 'GET' &&
    values.config[RestAPIWindowToggleKey] === true
  );
}

const RestAPIWindowTimeFormats = [
  { label: 'RFC3339 (UTC)', value: 'rfc3339' },
  { label: 'Unix (s)', value: 'unix_seconds' },
  { label: 'Unix (ms)', value: 'unix_milliseconds' },
];

const OptionalIntegerInput = z.union([z.string(), z.number()]);

export function getRestAPIWindowFormFields(t: TFunction): FormFieldConfig[] {
  const isPageWindow = (values: any) =>
    restAPIWindowEnabled(values) && values.config.pagination_type === 'page';
  const isPageOrOffsetWindow = (values: any) =>
    restAPIWindowEnabled(values) &&
    ['page', 'offset'].includes(values.config.pagination_type);
  const validateOptionalPositiveInteger = (value: unknown) => {
    const text = String(value ?? '').trim();
    return !text || /^[1-9]\d*$/.test(text)
      ? true
      : t('setting.restApiWindowPositiveInteger');
  };

  return [
    {
      label: t('setting.restApiWindowEnabled'),
      name: `config.${RestAPIWindowToggleKey}`,
      type: FormFieldType.Switch,
      required: false,
      defaultValue: false,
      shouldRender: (values) => values?.config?.method === 'GET',
    },
    {
      label: t('setting.restApiWindowStartParam'),
      name: 'config.request_window.start_param',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'startTime',
      shouldRender: restAPIWindowEnabled,
    },
    {
      label: t('setting.restApiWindowEndParam'),
      name: 'config.request_window.end_param',
      type: FormFieldType.Text,
      required: true,
      placeholder: 'endTime',
      shouldRender: restAPIWindowEnabled,
    },
    {
      label: t('setting.restApiWindowInitialStart'),
      name: 'config.request_window.initial_start',
      type: FormFieldType.Text,
      required: true,
      placeholder: '2026-10-01T00:00:00Z',
      tooltip: t('setting.restApiWindowInitialStartTip'),
      shouldRender: restAPIWindowEnabled,
    },
    {
      label: t('setting.restApiWindowRequestTimeFormat'),
      name: 'config.request_window.time_format',
      type: FormFieldType.Select,
      required: true,
      options: RestAPIWindowTimeFormats,
      shouldRender: restAPIWindowEnabled,
    },
    {
      label: t('setting.restApiWindowResponseTimeFormat'),
      name: 'config.request_window.response_time_format',
      type: FormFieldType.Select,
      required: true,
      options: RestAPIWindowTimeFormats,
      shouldRender: restAPIWindowEnabled,
    },
    {
      label: t('setting.restApiWindowMaxSeconds'),
      name: 'config.request_window.max_window_seconds',
      type: FormFieldType.Text,
      required: false,
      schema: OptionalIntegerInput,
      placeholder: '2592000',
      tooltip: t('setting.restApiWindowMaxSecondsTip'),
      shouldRender: restAPIWindowEnabled,
      customValidate: validateOptionalPositiveInteger,
    },
    {
      label: t('setting.restApiWindowPageParam'),
      name: 'config.pagination_config.page_param',
      type: FormFieldType.Text,
      placeholder: 'page',
      shouldRender: isPageWindow,
    },
    {
      label: t('setting.restApiWindowPageSizeParam'),
      name: 'config.pagination_config.page_size_param',
      type: FormFieldType.Text,
      placeholder: 'pageSize',
      shouldRender: isPageWindow,
    },
    {
      label: t('setting.dataSourceFieldPageSize'),
      name: 'config.pagination_config.page_size',
      type: FormFieldType.Text,
      schema: OptionalIntegerInput,
      customValidate: validateOptionalPositiveInteger,
      shouldRender: isPageOrOffsetWindow,
    },
    {
      label: t('setting.restApiWindowHasNextPage'),
      name: 'config.pagination_config.has_next_page_field',
      type: FormFieldType.Text,
      placeholder: 'hasNext',
      tooltip: t('setting.restApiWindowHasNextPageTip'),
      shouldRender: (values) =>
        restAPIWindowEnabled(values) &&
        values.config.pagination_type !== 'none',
    },
  ];
}
