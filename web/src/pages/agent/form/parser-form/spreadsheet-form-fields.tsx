import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { useTranslation } from 'react-i18next';
import { TcadpFormFields } from './common-form-fields';
import { CommonProps } from './interface';
import { buildFieldNameWithPrefix } from './utils';

export function SpreadsheetFormFields({ prefix }: CommonProps) {
  const { t } = useTranslation();
  const parserMethodOptions = [
    { label: ParseDocumentType.DeepDOC, value: ParseDocumentType.DeepDOC },
    {
      label: ParseDocumentType.TCADPParser,
      value: ParseDocumentType.TCADPParser,
    },
  ];

  return (
    <>
      <RAGFlowFormItem
        name={buildFieldNameWithPrefix('parse_method', prefix)}
        label={t('flow.parserMethod')}
        className="space-y-0"
      >
        <SelectWithSearch options={parserMethodOptions} />
      </RAGFlowFormItem>
      <TcadpFormFields prefix={prefix} />
    </>
  );
}
