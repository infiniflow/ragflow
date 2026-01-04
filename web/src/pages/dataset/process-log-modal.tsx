import FileStatusBadge from '@/components/file-status-badge';
import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { RunningStatusMap } from '@/constants/knowledge';
import { useTranslate } from '@/hooks/common-hooks';
import React, { useMemo } from 'react';
import reactStringReplace from 'react-string-replace';
import { RunningStatus } from './dataset/constant';
export interface ILogInfo {
  fileType?: string;
  uploadedBy?: string;
  uploadDate?: string;
  processBeginAt?: string;
  chunkNumber?: number;

  taskId?: string;
  fileName: string;
  fileSize?: string;
  source?: string;
  task?: string;
  status?: RunningStatus;
  startTime?: string;
  endTime?: string;
  duration?: string;
  details: string;
}

interface ProcessLogModalProps {
  visible: boolean;
  onCancel: () => void;
  logInfo: ILogInfo;
  title: string;
}

const InfoItem: React.FC<{
  overflowTip?: boolean;
  label: string;
  value: string | React.ReactNode;
  className?: string;
}> = ({ label, value, className = '', overflowTip = false }) => {
  return (
    <div className={`flex flex-col mb-4 ${className}`}>
      <span className="text-text-secondary text-sm">{label}</span>
      {overflowTip && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="text-text-primary mt-1 truncate w-full">
              {value}
            </span>
          </TooltipTrigger>
          <TooltipContent>{value}</TooltipContent>
        </Tooltip>
      )}
      {!overflowTip && (
        <span className="text-text-primary mt-1 truncate w-full">{value}</span>
      )}
    </div>
  );
};
export const replaceText = (text: string) => {
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
const ProcessLogModal: React.FC<ProcessLogModalProps> = ({
  visible,
  onCancel,
  logInfo: initData,
  title,
}) => {
  const { t } = useTranslate('knowledgeDetails');
  const blackKeyList = [''];
  const logInfo = useMemo(() => {
    return initData;
  }, [initData]);

  return (
    <Modal
      title={title || 'log'}
      open={visible}
      onCancel={onCancel}
      footer={
        <div className="flex justify-end">
          <Button onClick={onCancel}>{t('close')}</Button>
        </div>
      }
      className="process-log-modal"
    >
      <div className=" rounded-lg">
        <div className="flex flex-wrap ">
          {Object?.keys(logInfo).map((key) => {
            if (
              blackKeyList.includes(key) ||
              !logInfo[key as keyof typeof logInfo]
            ) {
              return null;
            }
            if (key === 'details') {
              return (
                <div className="w-full  mt-2" key={key}>
                  <InfoItem
                    label={t(key)}
                    value={
                      <div className="w-full  whitespace-pre-line text-wrap bg-bg-card rounded-lg h-fit max-h-[350px] overflow-y-auto scrollbar-auto p-2.5">
                        {replaceText(logInfo.details)}
                      </div>
                    }
                  />
                </div>
              );
            }
            if (key === 'status') {
              return (
                <div className="flex flex-col w-1/2" key={key}>
                  <span className="text-text-secondary text-sm">
                    {t('status')}
                  </span>
                  <div className="mt-1">
                    <FileStatusBadge
                      status={logInfo.status as RunningStatus}
                      name={RunningStatusMap[logInfo.status as RunningStatus]}
                    />
                  </div>
                </div>
              );
            }
            return (
              <div className="w-1/2" key={key}>
                <InfoItem
                  overflowTip={true}
                  label={t(key)}
                  value={logInfo[key as keyof typeof logInfo]}
                />
              </div>
            );
          })}
        </div>
        {/* <InfoItem label="Details" value={logInfo.details} /> */}
        {/* <div>
          <div>Details</div>
          <div>
            <ul className="space-y-2">
              <div className={'w-full  whitespace-pre-line text-wrap '}>
                {replaceText(logInfo.details)}
              </div>
            </ul>
          </div>
        </div> */}
      </div>
    </Modal>
  );
};

export default ProcessLogModal;
