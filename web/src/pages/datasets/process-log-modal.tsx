import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import { useTranslate } from '@/hooks/common-hooks';
import React from 'react';

interface ProcessLogModalProps {
  visible: boolean;
  onCancel: () => void;
  taskInfo: {
    taskId: string;
    fileName: string;
    fileSize: string;
    source: string;
    task: string;
    state: 'Running' | 'Completed' | 'Failed' | 'Pending';
    startTime: string;
    endTime?: string;
    duration?: string;
    details: string;
  };
}

const StatusTag: React.FC<{ state: string }> = ({ state }) => {
  const getTagStyle = () => {
    switch (state) {
      case 'Running':
        return 'bg-green-500 text-green-100';
      case 'Completed':
        return 'bg-blue-500 text-blue-100';
      case 'Failed':
        return 'bg-red-500 text-red-100';
      case 'Pending':
        return 'bg-yellow-500 text-yellow-100';
      default:
        return 'bg-gray-500 text-gray-100';
    }
  };

  return (
    <span
      className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${getTagStyle()}`}
    >
      <span className="w-1.5 h-1.5 rounded-full mr-1 bg-current"></span>
      {state}
    </span>
  );
};

const InfoItem: React.FC<{
  label: string;
  value: string | React.ReactNode;
  className?: string;
}> = ({ label, value, className = '' }) => {
  return (
    <div className={`flex flex-col mb-4 ${className}`}>
      <span className="text-text-secondary text-sm">{label}</span>
      <span className="text-text-primary mt-1">{value}</span>
    </div>
  );
};

const ProcessLogModal: React.FC<ProcessLogModalProps> = ({
  visible,
  onCancel,
  taskInfo,
}) => {
  const { t } = useTranslate('knowledgeDetails');
  return (
    <Modal
      title={t('processLog')}
      open={visible}
      onCancel={onCancel}
      footer={
        <div className="flex justify-end">
          <Button onClick={onCancel}>{t('close')}</Button>
        </div>
      }
      className="process-log-modal"
    >
      <div className="p-6 rounded-lg">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          {/* Left Column */}
          <div className="space-y-4">
            <InfoItem label="Task ID" value={taskInfo.taskId} />
            <InfoItem label="File Name" value={taskInfo.fileName} />
            <InfoItem label="File Size" value={taskInfo.fileSize} />
            <InfoItem label="Source" value={taskInfo.source} />
            <InfoItem label="Task" value={taskInfo.task} />
            <InfoItem label="Details" value={taskInfo.details} />
          </div>

          {/* Right Column */}
          <div className="space-y-4">
            <div className="flex flex-col">
              <span className="text-text-secondary text-sm">States</span>
              <div className="mt-1">
                <StatusTag state={taskInfo.state} />
              </div>
            </div>

            <InfoItem label="Start Time" value={taskInfo.startTime} />

            <InfoItem label="End Time" value={taskInfo.endTime || '-'} />

            <InfoItem
              label="Duration"
              value={taskInfo.duration ? `${taskInfo.duration}s` : '-'}
            />
          </div>
        </div>
      </div>
    </Modal>
  );
};

export default ProcessLogModal;
