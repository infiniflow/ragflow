import { api_host } from '@/utils/api';
import FileViewer from 'react-file-viewer';
import { useParams, useSearchParams } from 'umi';
import Excel from './excel';

const DocumentViewer = () => {
  const { id: documentId } = useParams();
  const api = `${api_host}/file/get/${documentId}`;
  const [currentQueryParameters] = useSearchParams();
  const ext = currentQueryParameters.get('ext');

  const onError = (e: any) => {
    console.error(e, 'error in file-viewer');
  };

  return (
    <section style={{ width: '100%' }}>
      {ext === 'xlsx' && <Excel filePath={api}></Excel>}
      {ext !== 'xlsx' && (
        <FileViewer fileType={ext} filePath={api} onError={onError} />
      )}
    </section>
  );
};

export default DocumentViewer;
