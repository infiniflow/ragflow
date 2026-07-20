import { useCallback, useState } from 'react';

export const useFieldModal = () => {
  const [addFieldModalOpen, setAddFieldModalOpen] = useState(false);
  const [editingFieldIndex, setEditingFieldIndex] = useState<number | null>(
    null,
  );

  const handleModalOpenChange = useCallback(
    (open: boolean) => {
      setAddFieldModalOpen(open);
      if (!open) setEditingFieldIndex(null);
    },
    [setAddFieldModalOpen],
  );

  const handleOpenAddField = useCallback(() => {
    setEditingFieldIndex(null);
    setAddFieldModalOpen(true);
  }, []);

  const handleOpenEditField = useCallback((index: number) => {
    setEditingFieldIndex(index);
    setAddFieldModalOpen(true);
  }, []);

  return {
    addFieldModalOpen,
    editingFieldIndex,
    setEditingFieldIndex,
    handleModalOpenChange,
    handleOpenAddField,
    handleOpenEditField,
  };
};
