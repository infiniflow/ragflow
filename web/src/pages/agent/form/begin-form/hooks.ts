import { useSetModalState } from '@/hooks/common-hooks';
import { useSetSelectedRecord } from '@/hooks/logic-hooks';
import { useCallback, useMemo, useState } from 'react';
import { BeginQuery, INextOperatorForm } from '../../interface';

export const useEditQueryRecord = ({ form }: INextOperatorForm) => {
  const { setRecord, currentRecord } = useSetSelectedRecord<BeginQuery>();
  const { visible, hideModal, showModal } = useSetModalState();
  const [index, setIndex] = useState(-1);

  const otherThanCurrentQuery = useMemo(() => {
    const query: BeginQuery[] = form?.getValues('query') || [];
    return query.filter((item, idx) => idx !== index);
  }, [form, index]);

  const handleEditRecord = useCallback(
    (record: BeginQuery) => {
      const query: BeginQuery[] = form?.getValues('query') || [];

      const nextQuery: BeginQuery[] =
        index > -1 ? query.toSpliced(index, 1, record) : [...query, record];

      // onValuesChange?.(
      //   { query: nextQuery },
      //   { query: nextQuery, prologue: form?.getFieldValue('prologue') },
      // );
      hideModal();
    },
    [form, hideModal, index],
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
