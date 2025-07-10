import { useFetchExcel } from '@/pages/document-viewer/hooks';
import classNames from 'classnames';
import { useGetDocumentUrl } from './hooks';

interface ExcelCsvPreviewerProps {
  className?: string;
}

export const ExcelCsvPreviewer: React.FC<ExcelCsvPreviewerProps> = ({
  className,
}) => {
  const url = useGetDocumentUrl();
  const { containerRef } = useFetchExcel(url);

  return (
    <div
      ref={containerRef}
      className={classNames(
        'relative w-full h-full p-4 bg-background-paper border border-border-normal rounded-md excel-csv-previewer',
        className,
      )}
    ></div>
  );
};
