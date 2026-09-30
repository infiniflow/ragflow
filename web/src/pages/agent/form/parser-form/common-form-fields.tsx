import { useCrossLanguageOptions } from '@/components/cross-language-form-field';
import {
  LayoutRecognizeFormField,
  ParseDocumentType,
} from '@/components/layout-recognize-form-field';
import {
  SelectWithSearch,
  SelectWithSearchFlagOptionType,
} from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { Switch } from '@/components/ui/switch';
import { FileType } from '@/constants/file';
import { upperCase, upperFirst } from 'lodash';
import { useEffect } from 'react';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { useOwnerTenantId } from '../../context';
import {
  OutputFormatMap,
  SpreadsheetOutputFormat,
} from '../../constant/pipeline';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

const UppercaseFields = [
  SpreadsheetOutputFormat.Html,
  SpreadsheetOutputFormat.Json,
];

function buildOutputOptionsFormatMap() {
  return Object.entries(OutputFormatMap).reduce<
    Record<string, SelectWithSearchFlagOptionType[]>
  >((pre, [key, value]) => {
    pre[key] = Object.values(value).map((v) => ({
      label: UppercaseFields.some((x) => x === v)
        ? upperCase(v)
        : upperFirst(v),
      value: v,
    }));
    return pre;
  }, {});
}

export type OutputFormatFormFieldProps = CommonProps & {
  fileType: FileType;
};

export function OutputFormatFormField({
  prefix,
  fileType,
}: OutputFormatFormFieldProps) {
  const { t } = useTranslation();
  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix(`output_format`, prefix)}
      label={t('flow.outputFormat')}
    >
      <SelectWithSearch
        options={buildOutputOptionsFormatMap()[fileType]}
      ></SelectWithSearch>
    </RAGFlowFormItem>
  );
}

export function ParserMethodFormField({
  prefix,
  optionsWithoutLLM,
}: CommonProps & { optionsWithoutLLM?: { value: string; label: string }[] }) {
  const { t } = useTranslation();
  const ownerTenantId = useOwnerTenantId();
  return (
    <LayoutRecognizeFormField
      name={buildFieldNameWithPrefix(`parse_method`, prefix)}
      horizontal={false}
      optionsWithoutLLM={optionsWithoutLLM}
      label={t('flow.parserMethod')}
      ownerTenantId={ownerTenantId}
    ></LayoutRecognizeFormField>
  );
}

const TableResultTypeOptions: SelectWithSearchFlagOptionType[] = [
  { label: 'Markdown', value: '0' },
  { label: 'HTML', value: '1' },
];

const MarkdownImageResponseTypeOptions: SelectWithSearchFlagOptionType[] = [
  { label: 'URL', value: '0' },
  { label: 'Text', value: '1' },
];

type TcadpSelectFieldProps = CommonProps & {
  name: string;
  label: string;
  options: SelectWithSearchFlagOptionType[];
};

function TcadpSelectField({
  prefix,
  name,
  label,
  options,
}: TcadpSelectFieldProps) {
  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix(name, prefix)}
      label={label}
    >
      {(field) => (
        <SelectWithSearch
          value={field.value}
          onChange={field.onChange}
          options={options}
        ></SelectWithSearch>
      )}
    </RAGFlowFormItem>
  );
}

// TCADP parser options shared by the PDF and spreadsheet forms. Visible only
// when TCADP is the parse method, and seeds the default values on selection.
export function TcadpFormFields({ prefix }: CommonProps) {
  const { t } = useTranslation();
  const form = useFormContext();

  const parseMethod = useWatch({
    name: buildFieldNameWithPrefix('parse_method', prefix),
  });
  const shown = !!parseMethod && parseMethod === ParseDocumentType.TCADPParser;

  // Set default values for TCADP options when TCADP is selected
  useEffect(() => {
    if (!shown) {
      return;
    }
    const names = ['table_result_type', 'markdown_image_response_type'];
    names.forEach((name) => {
      const fieldName = buildFieldNameWithPrefix(name, prefix);
      if (!form.getValues(fieldName)) {
        form.setValue(fieldName, '1', {
          shouldValidate: true,
          shouldDirty: true,
        });
      }
    });
  }, [shown, form, prefix]);

  if (!shown) {
    return null;
  }

  return (
    <>
      <TcadpSelectField
        prefix={prefix}
        name="table_result_type"
        label={t('flow.tableResultType') || '表格返回形式'}
        options={TableResultTypeOptions}
      />
      <TcadpSelectField
        prefix={prefix}
        name="markdown_image_response_type"
        label={t('flow.markdownImageResponseType') || '图片返回形式'}
        options={MarkdownImageResponseTypeOptions}
      />
    </>
  );
}

export function TwoColumnCheckFormField({ prefix }: CommonProps) {
  const { t } = useTranslation();
  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix(`enable_multi_column`, prefix)}
      label={t('flow.enableMultiColumn')}
      horizontal={true}
      labelClassName="w-full"
      valueClassName="w-8"
      tooltip={t('flow.enableMultiColumnTip')}
    >
      {(field) => (
        <Switch
          checked={field.value}
          onCheckedChange={(checked) => {
            field.onChange?.(checked);
          }}
        />
      )}
    </RAGFlowFormItem>
  );
}

export function RmdirFormField({ prefix }: CommonProps) {
  const { t } = useTranslation();
  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix(`remove_toc`, prefix)}
      label={t('flow.removeToc')}
      horizontal={true}
      tooltip={t('flow.removeTocTip')}
      labelClassName="w-full"
      valueClassName="w-8"
    >
      {(field) => (
        <Switch
          checked={field.value}
          onCheckedChange={(checked) => {
            field.onChange?.(checked);
          }}
        />
      )}
    </RAGFlowFormItem>
  );
}

export function RemoveHeaderFooterFormField({ prefix }: CommonProps) {
  const { t } = useTranslation();
  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix(`remove_header_footer`, prefix)}
      label={t('flow.removeHeaderFooter')}
      horizontal={true}
      labelClassName="w-full"
      valueClassName="w-8"
    >
      {(field) => (
        <Switch
          checked={field.value}
          onCheckedChange={(checked) => {
            field.onChange?.(checked);
          }}
        />
      )}
    </RAGFlowFormItem>
  );
}

export function LanguageFormField({ prefix }: CommonProps) {
  const { t } = useTranslation();
  const crossLanguageOptions = useCrossLanguageOptions();

  return (
    <RAGFlowFormItem
      name={buildFieldNameWithPrefix(`lang`, prefix)}
      label={t('flow.lang')}
    >
      {(field) => (
        <SelectWithSearch
          options={crossLanguageOptions}
          value={field.value}
          onChange={field.onChange}
        ></SelectWithSearch>
      )}
    </RAGFlowFormItem>
  );
}
