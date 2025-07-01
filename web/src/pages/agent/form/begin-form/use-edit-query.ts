import { useSetModalState } from '@/hooks/common-hooks';
import { useSetSelectedRecord } from '@/hooks/logic-hooks';
import { useCallback, useMemo, useState } from 'react';
import { BeginQuery, INextOperatorForm } from '../../interface';

export const useEditQueryRecord = ({ form, node }: INextOperatorForm) => {
  const { setRecord, currentRecord } = useSetSelectedRecord<BeginQuery>();
  const { visible, hideModal, showModal } = useSetModalState();
  const [index, setIndex] = useState(-1);

  const otherThanCurrentQuery = useMemo(() => {
    const inputs: BeginQuery[] = form?.getValues('inputs') || [];
    return inputs.filter((item, idx) => idx !== index);
  }, [form, index]);

  const handleEditRecord = useCallback(
    (record: BeginQuery) => {
      const inputs: BeginQuery[] = form?.getValues('inputs') || [];
      console.log('🚀 ~ useEditQueryRecord ~ inputs:', inputs);

      const nextQuery: BeginQuery[] =
        index > -1 ? inputs.toSpliced(index, 1, record) : [...inputs, record];

      form.setValue('inputs', nextQuery, {
        shouldDirty: true,
        shouldTouch: true,
      });

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

  const handleDeleteRecord = useCallback(
    (idx: number) => {
      const inputs = form?.getValues('inputs') || [];
      const nextQuery = inputs.filter(
        (item: BeginQuery, index: number) => index !== idx,
      );

      form.setValue('inputs', nextQuery, { shouldDirty: true });
    },
    [form],
  );

  return {
    ok: handleEditRecord,
    currentRecord,
    setRecord,
    visible,
    hideModal,
    showModal: handleShowModal,
    otherThanCurrentQuery,
    handleDeleteRecord,
  };
};
