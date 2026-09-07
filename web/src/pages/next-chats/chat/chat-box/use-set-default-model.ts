import { ModelTypeMap } from '@/components/model-tree-select';
import { useFetchAllAddedModels } from '@/hooks/use-llm-request';
import { getRealModelName } from '@/utils/llm-util';
import { useEffect, useRef } from 'react';
import { UseFormReturn } from 'react-hook-form';

export function useSetDefaultModel(form: UseFormReturn<any>) {
  const { data: allAddedModels } = useFetchAllAddedModels();
  const hasSet = useRef(false);

  useEffect(() => {
    if (hasSet.current || !allAddedModels.length) return;
    const chatModels = allAddedModels.filter((m) =>
      m.model_type?.some((t) => ModelTypeMap.llm_id.includes(t)),
    );
    const first = chatModels[0];
    if (first) {
      const modelName = getRealModelName(first.name);
      form.setValue(
        'llm_id',
        `${modelName}@${first.instance_name}@${first.provider_name}`,
      );
      hasSet.current = true;
    }
  }, [allAddedModels, form]);
}
