import { useSetModalState } from '@/hooks/common-hooks';
import { useSetSelectedRecord } from '@/hooks/logic-hooks';
import { useCallback, useMemo, useState } from 'react';
import { BeginQuery, IOperatorForm } from '../../interface';

export const useEditQueryRecord = ({ form, onValuesChange }: IOperatorForm) => {
  const { setRecord, currentRecord } = useSetSelectedRecord<BeginQuery>();
  const { visible, hideModal, showModal } = useSetModalState();
  const [index, setIndex] = useState(-1);

  const otherThanCurrentQuery = useMemo(() => {
    const query: BeginQuery[] = form?.getFieldValue('query') || [];
    return query.filter((item, idx) => idx !== index);
  }, [form, index]);

  const handleEditRecord = useCallback(
    (record: BeginQuery) => {
      const query: BeginQuery[] = form?.getFieldValue('query') || [];

      const nextQuery: BeginQuery[] =
        index > -1 ? query.toSpliced(index, 1, record) : [...query, record];

      onValuesChange?.(
        { query: nextQuery },
        { query: nextQuery, prologue: form?.getFieldValue('prologue') },
      );
      hideModal();
    },
    [form, hideModal, index, onValuesChange],
  );

  const handleShowModal = useCallback(
    (idx?: number, record?: BeginQuery) => {
      setIndex(idx ?? -1);
      setRecord(record ?? ({} as BeginQuery));
      showModal();
    },
    [setRecord, showModal],
  );

  return {
    ok: handleEditRecord,
    currentRecord,
    setRecord,
    visible,
    hideModal,
    showModal: handleShowModal,
    otherThanCurrentQuery,
  };
};
