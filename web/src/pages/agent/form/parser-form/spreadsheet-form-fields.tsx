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
import { FormControl, FormItem, FormLabel } from '@/components/ui/form';
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { isEmpty } from 'lodash';
import { useEffect, useMemo } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useOwnerTenantId } from '../../context';
import { FileType } from '../../constant/pipeline';
import {
  FlattenMediaToTextFormField,
  ParserMethodFormField,
} from './common-form-fields';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix, isForeignParseMethod } from './utils';

const ROLE_OPTIONS = [
  { value: 'both', labelKey: 'tableColumnRoleBoth' },
  { value: 'indexing', labelKey: 'tableColumnRoleIndexing' },
  { value: 'metadata', labelKey: 'tableColumnRoleMetadata' },
] as const;

function selectTableColumnRoleValue(raw: string | undefined): string {
  if (!raw) return 'both';
  return raw === 'vectorize' ? 'indexing' : raw;
}

const tableResultTypeOptions: SelectWithSearchFlagOptionType[] = [
  { label: 'Markdown', value: '0' },
  { label: 'HTML', value: '1' },
];

const markdownImageResponseTypeOptions: SelectWithSearchFlagOptionType[] = [
  { label: 'URL', value: '0' },
  { label: 'Text', value: '1' },
];

export function SpreadsheetFormFields({ prefix }: CommonProps) {
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

  const mode = columnMode === 'manual' ? 'manual' : 'auto';
  const columns: string[] = Array.isArray(columnNames) ? columnNames : [];

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

  useEffect(() => {
    const current = form.getValues(parseMethodName);
    // On a file-type switch the field remounts and react-hook-form re-seeds it
    // from the node's saved form data, so it can hold another file type's
    // static parse method (e.g. ocr) — reset it to DeepDOC in that case too.
    if (
      isEmpty(current) ||
      isForeignParseMethod(FileType.Spreadsheet, current)
    ) {
      form.setValue(parseMethodName, ParseDocumentType.DeepDOC, {
        shouldValidate: true,
        shouldDirty: true,
      });
    }
  }, [form, parseMethodName]);

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
      <FormItem className="space-y-2">
        <FormLabel className="text-sm font-medium">
          {t('knowledgeConfiguration.tableColumnMode')}
        </FormLabel>
        <FormControl>
          <RadioGroup
            value={mode}
            onValueChange={handleModeChange}
            className="flex gap-4"
          >
            <div className="flex items-center space-x-2">
              <RadioGroupItem value="auto" id={`${prefix}-table-mode-auto`} />
              <label
                htmlFor={`${prefix}-table-mode-auto`}
                className="text-sm font-normal cursor-pointer"
              >
                {t('knowledgeConfiguration.tableColumnModeAuto')}
              </label>
            </div>
            <div className="flex items-center space-x-2">
              <RadioGroupItem
                value="manual"
                id={`${prefix}-table-mode-manual`}
              />
              <label
                htmlFor={`${prefix}-table-mode-manual`}
                className="text-sm font-normal cursor-pointer"
              >
                {t('knowledgeConfiguration.tableColumnModeManual')}
              </label>
            </div>
          </RadioGroup>
        </FormControl>
      </FormItem>

      {mode === 'auto' && (
        <p className="text-sm text-muted-foreground">
          {t('knowledgeConfiguration.tableColumnModeAutoDescription')}
        </p>
      )}

      {mode === 'manual' && columns.length === 0 && (
        <p className="text-sm text-muted-foreground">
          {t('knowledgeConfiguration.tableColumnRolesEmpty')}
        </p>
      )}

      {mode === 'manual' && columns.length > 0 && (
        <>
          <p className="text-sm text-muted-foreground mb-3">
            {t('knowledgeConfiguration.tableColumnRolesTip')}
          </p>
          <div className="space-y-3">
            {columns.map((col) => (
              <FormItem key={col} className="flex flex-row items-center gap-4">
                <FormLabel className="min-w-[120px] shrink-0 text-sm font-normal">
                  {col}
                </FormLabel>
                <FormControl>
                  <Select
                    value={selectTableColumnRoleValue(
                      columnRoles && columnRoles[col],
                    )}
                    onValueChange={(value) => handleRoleChange(col, value)}
                  >
                    <SelectTrigger className="w-[160px]">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {ROLE_OPTIONS.map((opt) => (
                        <SelectItem key={opt.value} value={opt.value}>
                          {t(`knowledgeConfiguration.${opt.labelKey}`)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </FormControl>
              </FormItem>
            ))}
          </div>
          <p className="text-xs text-muted-foreground mt-3">
            {t('knowledgeConfiguration.tableColumnRolesReparseTip')}
          </p>
        </>
      )}
    </>
  );
}
