import FileStatusBadge from '@/components/file-status-badge';
import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import { useTranslate } from '@/hooks/common-hooks';
import React from 'react';
import reactStringReplace from 'react-string-replace';

interface ProcessLogModalProps {
  visible: boolean;
  onCancel: () => void;
  taskInfo: {
    taskId: string;
    fileName: string;
    fileSize: string;
    source: string;
    task: string;
    state: 'Running' | 'Success' | 'Failed' | 'Pending';
    startTime: string;
    endTime?: string;
    duration?: string;
    details: string;
  };
}

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
  const replaceText = (text: string) => {
    // Remove duplicate \n
    const nextText = text.replace(/(\n)\1+/g, '$1');

    const replacedText = reactStringReplace(
      nextText,
      /(\[ERROR\].+\s)/g,
      (match, i) => {
        return (
          <span key={i} className={'text-red-600'}>
            {match}
          </span>
        );
      },
    );

    return replacedText;
  };
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
          </div>

          {/* Right Column */}
          <div className="space-y-4">
            <div className="flex flex-col">
              <span className="text-text-secondary text-sm">Status</span>
              <div className="mt-1">
                <FileStatusBadge status={taskInfo.state} />
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
        {/* <InfoItem label="Details" value={taskInfo.details} /> */}
        <div>
          <div>Details</div>
          <div>
            <ul className="space-y-2">
              <div className={'w-full  whitespace-pre-line text-wrap '}>
                {replaceText(taskInfo.details)}
              </div>
            </ul>
          </div>
        </div>
      </div>
    </Modal>
  );
};

export default ProcessLogModal;
