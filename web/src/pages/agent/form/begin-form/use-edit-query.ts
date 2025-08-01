import { useSetModalState } from '@/hooks/common-hooks';
import { useSetSelectedRecord } from '@/hooks/logic-hooks';
import { useCallback, useMemo, useState } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { BeginQuery, INextOperatorForm } from '../../interface';

export const useEditQueryRecord = ({
  form,
}: INextOperatorForm & { form: UseFormReturn }) => {
  const { setRecord, currentRecord } = useSetSelectedRecord<BeginQuery>();
  const { visible, hideModal, showModal } = useSetModalState();
  const [index, setIndex] = useState(-1);
  const inputs: BeginQuery[] = useWatch({
    control: form.control,
    name: 'inputs',
  });

  const otherThanCurrentQuery = useMemo(() => {
    return inputs.filter((item, idx) => idx !== index);
  }, [index, inputs]);

  const handleEditRecord = useCallback(
    (record: BeginQuery) => {
      const inputs: BeginQuery[] = form?.getValues('inputs') || [];

      const nextQuery: BeginQuery[] =
        index > -1 ? inputs.toSpliced(index, 1, record) : [...inputs, record];

      form.setValue('inputs', nextQuery);

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
      const nextInputs = inputs.filter(
        (item: BeginQuery, index: number) => index !== idx,
      );

      form.setValue('inputs', nextInputs);
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
