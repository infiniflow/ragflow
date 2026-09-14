import { RAGFlowNodeType } from '@/interfaces/database/agent';
import { isEmpty, omit } from 'lodash';
import { useMemo } from 'react';
import { initialRetrievalValues } from '../../constant';

export function useValues(node?: RAGFlowNodeType) {
  const defaultValues = useMemo(
    () => ({
      ...initialRetrievalValues,
    }),
    [],
  );

  const values = useMemo(() => {
    const formData = node?.data?.form;

    if (isEmpty(formData)) {
      return defaultValues;
    }

    // `dataset_ids` is the canonical field name today; older DSLs still
    // persist the dataset list under `kb_ids`, so fold the legacy key in on
    // load to keep the form and the saved DSL on a single field name.
    const legacyKbIds = (formData as Record<string, any>)?.kb_ids;
    const datasetIds =
      (formData as Record<string, any>)?.dataset_ids ?? legacyKbIds ?? [];

    return omit(
      { ...(formData as Record<string, any>), dataset_ids: datasetIds },
      'top_k',
    );
  }, [defaultValues, node?.data?.form]);

  return values;
}
