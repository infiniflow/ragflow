import { Languages } from '@/components/cross-language-form-field';
import { ParseDocumentType } from '@/components/layout-recognize-form-field';
import { Operator } from '@/constants/agent';
import { FileType } from '@/constants/file';
import { normalizeParserFormValues } from '@/pages/agent/form/parser-form/utils';
import { transformApiResponseToForm } from '@/pages/agent/form/title-chunker-form/hook';
import {
  getOperatorType,
  normalizeOperatorForm,
  transformExtractorConfigToForm,
  transformFormConfigToApi,
} from '@/utils/pipeline-operator';
import type { TFunction } from 'i18next';

export function normalizeParserConfigForComparison(
  config: Record<string, any> | undefined,
  operatorDefaults: Record<string, any>,
  t: TFunction,
) {
  const entries = Object.entries({ ...operatorDefaults, ...config })
    .filter(([operatorId]) => operatorId.includes(':'))
    .map(([operatorId, values]) => {
      const operatorType = getOperatorType(operatorId);
      let normalized = normalizeOperatorForm(operatorId, values);

      // Mirror initialization performed inside the operator forms, including
      // defaults that their mounted controls write back without user edits.
      if (operatorType === Operator.Parser) {
        normalized = normalizeParserFormValues(normalized);
        normalized.setups = normalized.setups.map(
          (setup: Record<string, any>) => {
            const next = { ...setup };
            if (
              setup.fileFormat === FileType.PDF &&
              setup.parse_method &&
              ![
                ParseDocumentType.DeepDOC,
                ParseDocumentType.PlainText,
                ParseDocumentType.TCADPParser,
              ].includes(setup.parse_method) &&
              !setup.lang
            ) {
              next.lang = Languages[0];
            }
            if (
              [
                FileType.PDF,
                FileType.Spreadsheet,
                FileType.PowerPoint,
              ].includes(setup.fileFormat) &&
              setup.parse_method === ParseDocumentType.TCADPParser
            ) {
              next.table_result_type ||= '1';
              next.markdown_image_response_type ||= '1';
            }
            return next;
          },
        );
      } else if (operatorType === Operator.Extractor) {
        normalized = transformExtractorConfigToForm(normalized);
        for (const feature of ['keywords', 'questions', 'summary']) {
          normalized[feature] = {
            ...normalized[feature],
            system_prompt:
              normalized[feature]?.system_prompt ||
              t(`flow.prompts.system.${feature}`),
          };
        }
      } else if (
        operatorType === Operator.TitleChunker ||
        operatorType === Operator.ManualChunker
      ) {
        normalized = transformApiResponseToForm(normalized);
        if (operatorType === Operator.ManualChunker) {
          normalized.method = 'group';
        }
      }

      return [operatorId, transformFormConfigToApi(operatorType, normalized)];
    });

  // Registered optional controls can add undefined properties. Compare the
  // serialized settings sent to the API, where those properties are omitted.
  return JSON.parse(JSON.stringify(Object.fromEntries(entries)));
}
