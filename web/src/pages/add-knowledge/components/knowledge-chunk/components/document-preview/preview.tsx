import { Spin } from 'antd';
import { useRef, useState } from 'react';
import type { NewHighlight } from 'react-pdf-highlighter';
import {
  AreaHighlight,
  Highlight,
  PdfHighlighter,
  PdfLoader,
  Popup,
  Tip,
} from 'react-pdf-highlighter';
import { useGetSelectedChunk } from '../../hooks';
import { testHighlights } from './hightlights';
import { useGetDocumentUrl } from './hooks';

import styles from './index.less';

interface IProps {
  selectedChunkId: string;
}

const getNextId = () => String(Math.random()).slice(2);

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

const Preview = ({ selectedChunkId }: IProps) => {
  const url = useGetDocumentUrl();
  const selectedChunk = useGetSelectedChunk(selectedChunkId);

  const [state, setState] = useState<any>(testHighlights);
  const ref = useRef((highlight: any) => {});

  const parseIdFromHash = () =>
    document.location.hash.slice('#highlight-'.length);

  const resetHash = () => {
    document.location.hash = '';
  };

  const getHighlightById = (id: string) => {
    const highlights = state;

    return highlights.find((highlight: any) => highlight.id === id);
  };

  //   let scrollViewerTo = (highlight: any) => {};

  let scrollToHighlightFromHash = () => {
    const highlight = getHighlightById(parseIdFromHash());

    if (highlight) {
      ref.current(highlight);
    }
  };

  const addHighlight = (highlight: NewHighlight) => {
    const highlights = state;

    console.log('Saving highlight', highlight);

    setState([{ ...highlight, id: getNextId() }, ...highlights]);
  };

  const updateHighlight = (
    highlightId: string,
    position: Object,
    content: Object,
  ) => {
    console.log('Updating highlight', highlightId, position, content);

    setState(
      state.map((h: any) => {
        const {
          id,
          position: originalPosition,
          content: originalContent,
          ...rest
        } = h;
        return id === highlightId
          ? {
              id,
              position: { ...originalPosition, ...position },
              content: { ...originalContent, ...content },
              ...rest,
            }
          : h;
      }),
    );
  };

  // useEffect(() => {
  //   ref.current(testHighlights[0]);
  // }, [selectedChunk]);

  return (
    <div className={styles.documentContainer}>
      <PdfLoader url={url} beforeLoad={<Spin />}>
        {(pdfDocument) => (
          <PdfHighlighter
            pdfDocument={pdfDocument}
            enableAreaSelection={(event) => event.altKey}
            onScrollChange={resetHash}
            // pdfScaleValue="page-width"

            scrollRef={(scrollTo) => {
              //   scrollViewerTo = scrollTo;
              ref.current = scrollTo;

              scrollToHighlightFromHash();
            }}
            onSelectionFinished={(
              position,
              content,
              hideTipAndSelection,
              transformSelection,
            ) => (
              <Tip
                onOpen={transformSelection}
                onConfirm={(comment) => {
                  addHighlight({ content, position, comment });

                  hideTipAndSelection();
                }}
              />
            )}
            highlightTransform={(
              highlight,
              index,
              setTip,
              hideTip,
              viewportToScaled,
              screenshot,
              isScrolledTo,
            ) => {
              const isTextHighlight = !Boolean(
                highlight.content && highlight.content.image,
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
                  onChange={(boundingRect) => {
                    updateHighlight(
                      highlight.id,
                      { boundingRect: viewportToScaled(boundingRect) },
                      { image: screenshot(boundingRect) },
                    );
                  }}
                />
              );

              return (
                <Popup
                  popupContent={<HighlightPopup {...highlight} />}
                  onMouseOver={(popupContent) =>
                    setTip(highlight, (highlight: any) => popupContent)
                  }
                  onMouseOut={hideTip}
                  key={index}
                >
                  {component}
                </Popup>
              );
            }}
            highlights={state}
          />
        )}
      </PdfLoader>
    </div>
  );
};

export default Preview;
