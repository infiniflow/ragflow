import * as pdfjs from 'pdfjs-dist';
import React, { memo, useEffect, useRef, useState } from 'react';
import {
  AreaHighlight,
  Highlight,
  IHighlight,
  PdfHighlighter,
  PdfLoader,
  Popup,
} from 'react-pdf-highlighter';

pdfjs.GlobalWorkerOptions.workerSrc = '/pdfjs-dist/pdf.worker.min.js';

import { useCatchDocumentError } from '@/components/pdf-previewer/hooks';
import { Spin } from '@/components/ui/spin';
// import FileError from '@/pages/document-viewer/file-error';
import { Authorization } from '@/constants/authorization';
import FileError from '@/pages/document-viewer/file-error';
import { getAuthorization } from '@/utils/authorization-util';
import styles from './index.module.less';
type PdfLoaderProps = React.ComponentProps<typeof PdfLoader> & {
  httpHeaders?: Record<string, string>;
};

const Loader = PdfLoader as React.ComponentType<PdfLoaderProps>;
export interface IProps {
  highlights?: IHighlight[];
  setWidthAndHeight?: (width: number, height: number) => void;
  url: string;
  className?: string;
}
const HighlightPopup = ({
  comment,
}: {
  comment: { text: string; emoji: string };
}) =>
  comment.text ? (
    <div className="Highlight__popup">
      {comment.emoji} {comment.text}
    </div>
  ) : null;

// TODO: merge with DocumentPreviewer
const PdfPreview = ({
  highlights: state,
  setWidthAndHeight,
  url,
  className,
}: IProps) => {
  // const url = useGetDocumentUrl();

  const ref = useRef<(highlight: IHighlight) => void>(() => {});
  const [loaded, setLoaded] = useState(false);
  const error = useCatchDocumentError(url);

  const resetHash = () => {};

  useEffect(() => {
    if (state?.length && state?.length > 0 && loaded) {
      ref?.current(state[0]);
    }
  }, [state, loaded]);

  const httpHeaders = {
    [Authorization]: getAuthorization(),
  };

  const isUrlValid =
    !!url && !url.endsWith('undefined') && !url.endsWith('/get/');

  return (
    <div
      className={`${styles.documentContainer} rounded-[10px] overflow-hidden	${className}`}
    >
      {isUrlValid && (
        <Loader
          url={url}
          httpHeaders={httpHeaders}
          beforeLoad={
            <div className="absolute inset-0 flex items-center justify-center">
              <Spin />
            </div>
          }
          errorMessage={<FileError>{error}</FileError>}
        >
          {(pdfDocument) => {
            pdfDocument.getPage(1).then((page) => {
              const viewport = page.getViewport({ scale: 1 });
              const width = viewport.width;
              const height = viewport.height;
              setWidthAndHeight?.(width, height);
            });

            return (
              <PdfHighlighter
                pdfDocument={pdfDocument}
                enableAreaSelection={(event) => event.altKey}
                onScrollChange={resetHash}
                scrollRef={(scrollTo) => {
                  ref.current = scrollTo;
                  setLoaded(true);
                }}
                onSelectionFinished={() => null}
                highlightTransform={(
                  highlight,
                  index,
                  setTip,
                  hideTip,
                  viewportToScaled,
                  screenshot,
                  isScrolledTo,
                ) => {
                  const isTextHighlight = !(
                    highlight.content && highlight.content.image
                  );

                  const component = isTextHighlight ? (
                    <Highlight
                      isScrolledTo={isScrolledTo}
                      position={highlight.position}
                      comment={highlight.comment}
                    />
                  ) : (
                    <AreaHighlight
                      isScrolledTo={isScrolledTo}
                      highlight={highlight}
                      onChange={() => {}}
                    />
                  );

                  return (
                    <Popup
                      popupContent={<HighlightPopup {...highlight} />}
                      onMouseOver={(popupContent) =>
                        setTip(highlight, () => popupContent)
                      }
                      onMouseOut={hideTip}
                      key={index}
                    >
                      {component}
                    </Popup>
                  );
                }}
                highlights={loaded ? state || [] : []}
              />
            );
          }}
        </Loader>
      )}
    </div>
  );
};
export default memo(PdfPreview);
