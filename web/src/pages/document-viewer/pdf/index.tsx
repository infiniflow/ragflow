import { Skeleton } from 'antd';
import { PdfHighlighter, PdfLoader } from 'react-pdf-highlighter';
import FileError from '../file-error';
import { useCatchError } from '../hooks';

interface IProps {
  url: string;
}

const PdfPreviewer = ({ url }: IProps) => {
  const { error } = useCatchError(url);
  const resetHash = () => {};

  return (
    <div style={{ width: '100%', height: '100%' }}>
      <PdfLoader
        url={url}
        beforeLoad={<Skeleton active />}
        workerSrc="/pdfjs-dist/pdf.worker.min.js"
        errorMessage={<FileError>{error}</FileError>}
        onError={(e) => {
          console.warn(e);
        }}
      >
        {(pdfDocument) => {
          return (
            <PdfHighlighter
              pdfDocument={pdfDocument}
              enableAreaSelection={(event) => event.altKey}
              onScrollChange={resetHash}
              scrollRef={() => {}}
              onSelectionFinished={() => null}
              highlightTransform={() => {
                return <div></div>;
              }}
              highlights={[]}
            />
          );
        }}
      </PdfLoader>
    </div>
  );
};

export default PdfPreviewer;
