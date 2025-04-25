import { IParserConfig } from '@/interfaces/database/document';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { DocumentType } from '../layout-recognize-form-field';

export function useDefaultParserValues() {
  const { t } = useTranslation();

  const defaultParserValues = useMemo(() => {
    const defaultParserValues = {
      task_page_size: 12,
      layout_recognize: DocumentType.DeepDOC,
      chunk_token_num: 512,
      delimiter: '\n',
      auto_keywords: 0,
      auto_questions: 0,
      html4excel: false,
      raptor: {
        use_raptor: false,
        prompt: t('knowledgeConfiguration.promptText'),
        max_token: 256,
        threshold: 0.1,
        max_cluster: 64,
        random_seed: 0,
      },
      graphrag: {
        use_graphrag: false,
      },
      entity_types: [],
      pages: [],
    };

    return defaultParserValues;
  }, [t]);

  return defaultParserValues;
}

export function useFillDefaultValueOnMount() {
  const defaultParserValues = useDefaultParserValues();

  const fillDefaultValue = useCallback(
    (parserConfig: IParserConfig) => {
      return Object.entries(defaultParserValues).reduce<Record<string, any>>(
        (pre, [key, value]) => {
          if (key in parserConfig) {
            pre[key] = parserConfig[key as keyof IParserConfig];
          } else {
            pre[key] = value;
          }
          return pre;
        },
        {},
      );
    },
    [defaultParserValues],
  );

  return fillDefaultValue;
}
