import '@js-preview/excel/lib/index.css';
import FileError from '../file-error';
import { useFetchExcel } from '../hooks';

const Excel = ({ filePath }: { filePath: string }) => {
  const { status, containerRef, error } = useFetchExcel(filePath);

  return (
    <div
      id="excel"
      ref={containerRef}
      style={{ height: '100%', width: '100%' }}
    >
      {status || <FileError>{error}</FileError>}
    </div>
  );
};

export default Excel;
