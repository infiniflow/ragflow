import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { isEmpty, omit } from 'lodash';
import { useMemo } from 'react';
import { RetrievalFrom, initialRetrievalValues } from '../../constant';

export function useValues(node?: RAGFlowNodeType) {
  const defaultValues = useMemo(
    () => ({
      ...initialRetrievalValues,
    }),
    [],
  );

  const values = useMemo(() => {
    const formData = node?.data?.form as Record<string, any> | undefined;

    if (isEmpty(formData)) {
      return defaultValues;
    }

    // `dataset_ids` is the canonical field name today; older DSLs still
    // persist the dataset list under `kb_ids`, so fold the legacy key in on
    // load to keep the form and the saved DSL on a single field name.
    const legacyKbIds = formData?.kb_ids;

    return omit(
      {
        ...formData,
        dataset_ids: formData?.dataset_ids ?? legacyKbIds ?? [],
        // DSLs saved before Memory support carry neither key. Without a
        // concrete array the Memories select registers no value at all, so
        // the node would round-trip without `memory_ids` and the canvas
        // checklist could never tell it is unbound.
        memory_ids: formData?.memory_ids ?? [],
        retrieval_from: formData?.retrieval_from ?? RetrievalFrom.Dataset,
      },
      'top_k',
    );
  }, [defaultValues, node?.data?.form]);

  return values;
}
