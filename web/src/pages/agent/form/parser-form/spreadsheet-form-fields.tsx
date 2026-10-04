import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import { ParserMethodFormField, TcadpFormFields } from './common-form-fields';
import { CommonProps } from './interface';

export function SpreadsheetFormFields({ prefix }: CommonProps) {
  // Spreadsheet only supports DeepDOC and TCADPParser
  const optionsWithoutLLM = [
    { label: ParseDocumentType.DeepDOC, value: ParseDocumentType.DeepDOC },
    {
      label: ParseDocumentType.TCADPParser,
      value: ParseDocumentType.TCADPParser,
    },
  ];

  return (
    <>
      <ParserMethodFormField
        prefix={prefix}
        optionsWithoutLLM={optionsWithoutLLM}
      ></ParserMethodFormField>
      <TcadpFormFields prefix={prefix} />
    </>
  );
}
