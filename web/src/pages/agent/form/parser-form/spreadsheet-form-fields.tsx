import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import {
  ModelTreeSelectFormField,
  ModelTypeMap,
} from '@/components/model-tree-select';
import {
  SelectWithSearch,
  SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { TableColumnSettingsFields } from '@/components/table-column-settings-form-fields';
import { isEmpty } from 'lodash';
import { useEffect, useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useOwnerTenantId } from '../../context';
import {
  FlattenMediaToTextFormField,
  ParserMethodFormField,
} from './common-form-fields';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

const tableResultTypeOptions: SelectWithSearchFlagOptionType[] = [
  { label: 'Markdown', value: '0' },
  { label: 'HTML', value: '1' },
];

const markdownImageResponseTypeOptions: SelectWithSearchFlagOptionType[] = [
  { label: 'URL', value: '0' },
  { label: 'Text', value: '1' },
];

export function SpreadsheetFormFields({ prefix, isTableParser }: CommonProps) {
  const { t } = useTranslation();
  const form = useFormContext();
  const ownerTenantId = useOwnerTenantId();

  const parseMethodName = buildFieldNameWithPrefix('parse_method', prefix);
  const columnModeName = buildFieldNameWithPrefix('column_mode', prefix);
  const columnRolesName = buildFieldNameWithPrefix('column_roles', prefix);
  const columnNamesName = buildFieldNameWithPrefix('column_names', prefix);

  const parseMethod = useWatch({
    name: parseMethodName,
  });
  const flattenMediaToText = useWatch({
    name: buildFieldNameWithPrefix('flatten_media_to_text', prefix),
  });
  const columnMode = useWatch({
    control: form.control,
    name: columnModeName,
    defaultValue: 'auto',
  });
  const columnNames = useWatch({
    control: form.control,
    name: columnNamesName,
    defaultValue: [],
  });
  const columnRoles = useWatch({
    control: form.control,
    name: columnRolesName,
    defaultValue: {},
  });

  const handleModeChange = (value: string) => {
    form.setValue(columnModeName, value as 'auto' | 'manual', {
      shouldValidate: true,
      shouldDirty: true,
    });
  };

  const handleRoleChange = (columnName: string, role: string) => {
    const current =
      (form.getValues(columnRolesName) as Record<string, string>) || {};
    form.setValue(
      columnRolesName,
      {
        ...current,
        [columnName]: role,
      },
      {
        shouldValidate: true,
        shouldDirty: true,
      },
    );
  };

  // Spreadsheet only supports DeepDOC and TCADPParser
  const optionsWithoutLLM = [
    { label: ParseDocumentType.DeepDOC, value: ParseDocumentType.DeepDOC },
    {
      label: ParseDocumentType.TCADPParser,
      value: ParseDocumentType.TCADPParser,
    },
  ];

  const tcadpOptionsShown = useMemo(() => {
    return (
      !isEmpty(parseMethod) && parseMethod === ParseDocumentType.TCADPParser
    );
  }, [parseMethod]);

  // Set default values for TCADP options when TCADP is selected
  useEffect(() => {
    if (tcadpOptionsShown) {
      const tableResultTypeName = buildFieldNameWithPrefix(
        'table_result_type',
        prefix,
      );
      const markdownImageResponseTypeName = buildFieldNameWithPrefix(
        'markdown_image_response_type',
        prefix,
      );

      if (isEmpty(form.getValues(tableResultTypeName))) {
        form.setValue(tableResultTypeName, '1', {
          shouldValidate: true,
          shouldDirty: true,
        });
      }
      if (isEmpty(form.getValues(markdownImageResponseTypeName))) {
        form.setValue(markdownImageResponseTypeName, '1', {
          shouldValidate: true,
          shouldDirty: true,
        });
      }
    }
  }, [tcadpOptionsShown, form, prefix]);

  return (
    <>
      <ParserMethodFormField
        prefix={prefix}
        optionsWithoutLLM={optionsWithoutLLM}
      ></ParserMethodFormField>
      <FlattenMediaToTextFormField prefix={prefix} />
      {!flattenMediaToText && (
        <ModelTreeSelectFormField
          name={buildFieldNameWithPrefix('vlm.llm_id', prefix)}
          label={t('chat.model')}
          modelTypes={ModelTypeMap.img2txt_id}
          allowClear
          ownerTenantId={ownerTenantId}
        />
      )}
      {tcadpOptionsShown && (
        <>
          <RAGFlowFormItem
            name={buildFieldNameWithPrefix('table_result_type', prefix)}
            label={t('flow.tableResultType') || '表格返回形式'}
          >
            {(field) => (
              <SelectWithSearch
                value={field.value}
                onChange={field.onChange}
                options={tableResultTypeOptions}
              ></SelectWithSearch>
            )}
          </RAGFlowFormItem>
          <RAGFlowFormItem
            name={buildFieldNameWithPrefix(
              'markdown_image_response_type',
              prefix,
            )}
            label={t('flow.markdownImageResponseType') || '图片返回形式'}
          >
            {(field) => (
              <SelectWithSearch
                value={field.value}
                onChange={field.onChange}
                options={markdownImageResponseTypeOptions}
              ></SelectWithSearch>
            )}
          </RAGFlowFormItem>
        </>
      )}
      {isTableParser !== false && (
        <TableColumnSettingsFields
          idPrefix={prefix}
          mode={columnMode}
          columns={columnNames}
          roles={columnRoles}
          onModeChange={handleModeChange}
          onRoleChange={handleRoleChange}
        />
      )}
    </>
  );
}
