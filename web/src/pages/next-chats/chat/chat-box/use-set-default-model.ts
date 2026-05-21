import { LlmModelType } from '@/constants/knowledge';
import { useComposeLlmOptionsByModelTypes } from '@/hooks/use-llm-request';
import { useMount } from 'ahooks';
import { UseFormReturn } from 'react-hook-form';

export function useSetDefaultModel(form: UseFormReturn<any>) {
  const modelOptions = useComposeLlmOptionsByModelTypes([
    LlmModelType.Chat,
    LlmModelType.Image2text,
  ]);

  useMount(() => {
    const firstModel = modelOptions.at(0)?.options.at(0)?.value;
    if (firstModel) {
      form.setValue('llm_id', firstModel);
    }
  });
}
