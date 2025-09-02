import { useSetModalState } from '@/hooks/common-hooks';
import { useSetSelectedRecord } from '@/hooks/logic-hooks';
import { useCallback, useMemo, useState } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { INextOperatorForm } from '../../interface';
import { FormSchemaType, VariableFormSchemaType } from './schema';

export const useEditVariableRecord = ({
  form,
}: INextOperatorForm & { form: UseFormReturn<FormSchemaType> }) => {
  const { setRecord, currentRecord } =
    useSetSelectedRecord<VariableFormSchemaType>();

  const { visible, hideModal, showModal } = useSetModalState();
  const [index, setIndex] = useState(-1);
  const variables = useWatch({
    control: form.control,
    name: 'variables',
  });

  const otherThanCurrentQuery = useMemo(() => {
    return variables.filter((item, idx) => idx !== index);
  }, [index, variables]);

  const handleEditRecord = useCallback(
    (record: VariableFormSchemaType) => {
      const variables = form?.getValues('variables') || [];

      const nextVaribales =
        index > -1
          ? variables.toSpliced(index, 1, record)
          : [...variables, record];

      form.setValue('variables', nextVaribales);

      hideModal();
    },
    [form, hideModal, index],
  );

  const handleShowModal = useCallback(
    (idx?: number, record?: VariableFormSchemaType) => {
      setIndex(idx ?? -1);
      setRecord(record ?? ({} as VariableFormSchemaType));
      showModal();
    },
    [setRecord, showModal],
  );

  const handleDeleteRecord = useCallback(
    (idx: number) => {
      const variables = form?.getValues('variables') || [];
      const nextVariables = variables.filter((item, index) => index !== idx);

      form.setValue('variables', nextVariables);
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
