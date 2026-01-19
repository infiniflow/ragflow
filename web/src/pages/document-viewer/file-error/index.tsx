import { useTranslate } from '@/hooks/common-hooks';
import React from 'react';

const FileError = ({ children }: React.PropsWithChildren) => {
  const { t } = useTranslate('fileManager');
  return (
    <div className="flex items-center justify-center min-h-screen">
      <div className="bg-state-error-5 border border-state-error rounded-lg p-4 shadow-sm">
        <div className="flex ml-3">
          <div className="text-white font-medium">
            {children || t('fileError')}
          </div>
        </div>
      </div>
    </div>
  );
};

export default FileError;
