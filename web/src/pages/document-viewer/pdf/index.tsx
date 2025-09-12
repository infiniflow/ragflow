import { Authorization } from '@/constants/authorization';
import { getAuthorization } from '@/utils/authorization-util';
import { Skeleton } from 'antd';
import { PdfHighlighter, PdfLoader } from 'react-pdf-highlighter';
import FileError from '../file-error';
import { useCatchError } from '../hooks';
type PdfLoaderProps = React.ComponentProps<typeof PdfLoader> & {
  httpHeaders?: Record<string, string>;
};

const Loader = PdfLoader as React.ComponentType<PdfLoaderProps>;

interface IProps {
  url: string;
}

const PdfPreviewer = ({ url }: IProps) => {
  const { error } = useCatchError(url);
  const resetHash = () => {};
  const httpHeaders = {
    [Authorization]: getAuthorization(),
  };
  return (
    <div style={{ width: '100%', height: '100%' }}>
      <Loader
        url={url}
        httpHeaders={httpHeaders}
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
      </Loader>
    </div>
  );
};

export default PdfPreviewer;
