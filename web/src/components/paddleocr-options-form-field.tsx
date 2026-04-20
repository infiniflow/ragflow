import { LLMFactory } from '@/constants/llm';
import { useFormContext, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

export function PaddleOCROptionsFormField() {
  const form = useFormContext();
  const { t } = useTranslation();

  const layoutRecognize = useWatch({
    control: form.control,
    name: 'parser_config.layout_recognize',
  });

  // Check if PaddleOCR is selected (the value contains 'PaddleOCR' or matches the factory name)
  const isPaddleOCRSelected =
    layoutRecognize?.includes(LLMFactory.PaddleOCR) ||
    layoutRecognize?.toLowerCase()?.includes('paddleocr');

  if (!isPaddleOCRSelected) {
    return null;
  }

  return (
    <div className="space-y-4 border-l-2 border-primary/30 pl-4 ml-2">
      <div className="text-sm font-medium text-text-secondary">
        {t('knowledgeConfiguration.paddleocrOptions', 'PaddleOCR Options')}
      </div>

      <div className="rounded-md border border-dashed border-border/60 bg-muted/20 px-3 py-3 text-sm">
        <div className="font-medium text-text-secondary">
          {t(
            'knowledgeConfiguration.paddleocrPresetManaged',
            'These settings are defined by the selected PaddleOCR model preset.',
          )}
        </div>
        <div className="mt-2 space-y-1.5">
          {[
            {
              id: 'api-url',
              label: t(
                'knowledgeConfiguration.paddleocrApiUrl',
                'PaddleOCR API URL',
              ),
            },
            {
              id: 'access-token',
              label: t(
                'knowledgeConfiguration.paddleocrAccessToken',
                'AI Studio Access Token',
              ),
            },
            {
              id: 'algorithm',
              label: t(
                'knowledgeConfiguration.paddleocrAlgorithm',
                'PaddleOCR Algorithm',
              ),
            },
            {
              id: 'request-timeout',
              label: t(
                'knowledgeConfiguration.paddleocrRequestTimeout',
                'Request timeout (seconds)',
              ),
            },
          ].map(({ id, label }) => (
            <div
              key={id}
              className="flex items-center justify-between gap-3 rounded-md bg-background/60 px-3 py-2"
            >
              <span className="text-text-secondary">{label}</span>
              <span className="text-xs text-muted-foreground">
                {t(
                  'knowledgeConfiguration.paddleocrPresetManagedValue',
                  'Preset-defined',
                )}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
