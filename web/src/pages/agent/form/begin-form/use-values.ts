import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { isEmpty } from 'lodash';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { AgentDialogueMode } from '../../constant';
import { BeginQuery } from '../../interface';

export function useValues(node?: RAGFlowNodeType) {
  const { t } = useTranslation();

  const defaultValues = useMemo(
    () => ({
      enablePrologue: true,
      prologue: t('chat.setAnOpenerInitial'),
      mode: AgentDialogueMode.Conversational,
      inputs: [],
    }),
    [t],
  );

  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return defaultValues;
    }

    const inputs = Object.entries(formData?.inputs || {}).reduce<BeginQuery[]>(
      (pre, [key, value]) => {
        pre.push({ ...(value || {}), key });

        return pre;
      },
      [],
    );

    return { ...(formData || {}), inputs };
  }, [defaultValues, node?.data?.form]);

  return values;
}
