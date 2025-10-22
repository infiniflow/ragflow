import { useTestRetrieval } from '@/hooks/use-knowledge-request';
import { IModalProps } from '@/interfaces/common';
import TestingForm from '@/pages/dataset/testing/testing-form';
import { TestingResult } from '@/pages/dataset/testing/testing-result';
import { Modal } from 'antd';
import { useTranslation } from 'react-i18next';

type Props = IModalProps<any> & {
  open: boolean;
  onClose: () => void;
};

export default function RiskRetrievalModal({ open, onClose }: Props) {
  const { t } = useTranslation();

  const {
    loading,
    setValues,
    refetch,
    data,
    onPaginationChange,
    page,
    pageSize,
    handleFilterSubmit,
    filterValue,
  } = useTestRetrieval();

  return (
    <Modal
      open={open}
      onCancel={onClose}
      footer={null}
      width={1100}
      destroyOnClose
      title={t('knowledgeDetails.retrievalTesting')}
    >
      <section className="flex divide-x h-[70vh]">
        <div className="p-4 flex-1 overflow-auto">
          <TestingForm
            loading={loading}
            setValues={setValues}
            refetch={refetch}
          />
        </div>
        <div className="flex-1 overflow-auto">
          <TestingResult
            data={data}
            page={page}
            loading={loading}
            pageSize={pageSize}
            filterValue={filterValue}
            handleFilterSubmit={handleFilterSubmit}
            onPaginationChange={onPaginationChange}
          />
        </div>
      </section>
    </Modal>
  );
}
