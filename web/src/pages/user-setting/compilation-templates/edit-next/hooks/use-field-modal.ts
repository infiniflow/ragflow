/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
