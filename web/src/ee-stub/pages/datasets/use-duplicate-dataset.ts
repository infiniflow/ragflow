import { IDataset } from '@/interfaces/database/dataset';

/**
 * Enterprise seam for the dataset duplicate action. Open source has no
 * duplicate dialog, so the hook reports a permanently closed modal.
 */
export default function useDuplicateDataset() {
  return {
    isModalVisible: false,
    showModal: (_record: IDataset) => {},
    hideModal: () => {},
    initialName: '',
    onOk: async (_name: string) => {},
    loading: false,
  };
}

export type DuplicateDatasetReturnType = ReturnType<typeof useDuplicateDataset>;
