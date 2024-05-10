import { Skeleton } from 'antd';
import { PdfHighlighter, PdfLoader } from 'react-pdf-highlighter';

interface IProps {
  url: string;
}

const DocumentPreviewer = ({ url }: IProps) => {
  const resetHash = () => {};

  return (
    <div style={{ width: '100%' }}>
      <PdfLoader
        url={url}
        beforeLoad={<Skeleton active />}
        workerSrc="/pdfjs-dist/pdf.worker.min.js"
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

export default DocumentPreviewer;
