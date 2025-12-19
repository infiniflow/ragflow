import Image from '@/components/image';
import SvgIcon from '@/components/svg-icon';
import { IReference, IReferenceChunk } from '@/interfaces/database/chat';
import { getExtension } from '@/utils/document-util';
import DOMPurify from 'dompurify';
import { useCallback, useEffect, useMemo } from 'react';
import Markdown from 'react-markdown';
import SyntaxHighlighter from 'react-syntax-highlighter';
import rehypeKatex from 'rehype-katex';
import rehypeRaw from 'rehype-raw';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';
import { visitParents } from 'unist-util-visit-parents';

import { useTranslation } from 'react-i18next';

import 'katex/dist/katex.min.css'; // `rehype-katex` does not import the CSS for you

import { useFetchDocumentThumbnailsByIds } from '@/hooks/use-document-request';
import {
  preprocessLaTeX,
  replaceTextByOldReg,
  replaceThinkToSection,
  showImage,
} from '@/utils/chat';
import classNames from 'classnames';
import { omit } from 'lodash';
import { pipe } from 'lodash/fp';
import { CircleAlert } from 'lucide-react';
import { Button } from '../ui/button';
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '../ui/hover-card';
import { ImageCarousel } from './image-carousel';
import styles from './index.less';
import {
  groupConsecutiveReferences,
  shouldShowCarousel,
} from './reference-utils';

const getChunkIndex = (match: string) => Number(match);

// TODO: The display of the table is inconsistent with the display previously placed in the MessageItem.
const MarkdownContent = ({
  reference,
  clickDocumentButton,
  content,
}: {
  content: string;
  loading: boolean;
  reference: IReference;
  clickDocumentButton?: (documentId: string, chunk: IReferenceChunk) => void;
}) => {
  const { t } = useTranslation();
  const { setDocumentIds, data: fileThumbnails } =
    useFetchDocumentThumbnailsByIds();
  const contentWithCursor = useMemo(() => {
    let text = DOMPurify.sanitize(content, {
      ADD_TAGS: ['think', 'section'],
      ADD_ATTR: ['class'],
    });

    // let text = content;
    if (text === '') {
      text = t('chat.searching');
    }
    const nextText = replaceTextByOldReg(text);
    return pipe(replaceThinkToSection, preprocessLaTeX)(nextText);
  }, [content, t]);

  useEffect(() => {
    const docAggs = reference?.doc_aggs;
    setDocumentIds(Array.isArray(docAggs) ? docAggs.map((x) => x.doc_id) : []);
  }, [reference, setDocumentIds]);

  const handleDocumentButtonClick = useCallback(
    (
      documentId: string,
      chunk: IReferenceChunk,
      isPdf: boolean,
      documentUrl?: string,
    ) =>
      () => {
        if (!isPdf) {
          if (!documentUrl) {
            return;
          }
          window.open(documentUrl, '_blank');
        } else {
          clickDocumentButton?.(documentId, chunk);
        }
      },
    [clickDocumentButton],
  );

  const rehypeWrapReference = () => {
    return function wrapTextTransform(tree: any) {
      visitParents(tree, 'text', (node, ancestors) => {
        const latestAncestor = ancestors.at(-1);
        if (
          latestAncestor.tagName !== 'custom-typography' &&
          latestAncestor.tagName !== 'code'
        ) {
          node.type = 'element';
          node.tagName = 'custom-typography';
          node.properties = {};
          node.children = [{ type: 'text', value: node.value }];
        }
      });
    };
  };

  const getReferenceInfo = useCallback(
    (chunkIndex: number) => {
      const chunks = reference?.chunks ?? [];
      const chunkItem = chunks[chunkIndex];
      const document = reference?.doc_aggs?.find(
        (x) => x?.doc_id === chunkItem?.document_id,
      );
      const documentId = document?.doc_id;
      const documentUrl = document?.url;
      const fileThumbnail = documentId ? fileThumbnails[documentId] : '';
      const fileExtension = documentId ? getExtension(document?.doc_name) : '';
      const imageId = chunkItem?.image_id;

      return {
        documentUrl,
        fileThumbnail,
        fileExtension,
        imageId,
        chunkItem,
        documentId,
        document,
      };
    },
    [fileThumbnails, reference],
  );

  const getPopoverContent = useCallback(
    (chunkIndex: number) => {
      const {
        documentUrl,
        fileThumbnail,
        fileExtension,
        imageId,
        chunkItem,
        documentId,
        document,
      } = getReferenceInfo(chunkIndex);

      return (
        <div key={chunkItem?.id} className="flex gap-2">
          {imageId && (
            <HoverCard>
              <HoverCardTrigger>
                <Image
                  id={imageId}
                  className={styles.referenceChunkImage}
                ></Image>
              </HoverCardTrigger>
              <HoverCardContent>
                <Image
                  id={imageId}
                  className={styles.referenceImagePreview}
                ></Image>
              </HoverCardContent>
            </HoverCard>
          )}
          <div className={'space-y-2 max-w-[40vw]'}>
            <div
              dangerouslySetInnerHTML={{
                __html: DOMPurify.sanitize(chunkItem?.content ?? ''),
              }}
              className={classNames(styles.chunkContentText)}
            ></div>
            {documentId && (
              <section className="flex gap-1">
                {fileThumbnail ? (
                  <img
                    src={fileThumbnail}
                    alt=""
                    className={styles.fileThumbnail}
                  />
                ) : (
                  <SvgIcon
                    name={`file-icon/${fileExtension}`}
                    width={24}
                  ></SvgIcon>
                )}
                <Button
                  variant="link"
                  className={'text-wrap p-0'}
                  onClick={handleDocumentButtonClick(
                    documentId,
                    chunkItem,
                    fileExtension === 'pdf',
                    documentUrl,
                  )}
                >
                  {document?.doc_name}
                </Button>
              </section>
            )}
          </div>
        </div>
      );
    },
    [getReferenceInfo, handleDocumentButtonClick],
  );

  const renderReference = useCallback(
    (text: string) => {
      const groups = groupConsecutiveReferences(text);
      const elements = [];
      let lastIndex = 0;

      groups.forEach((group, groupIndex) => {
        if (group[0].start > lastIndex) {
          elements.push(text.substring(lastIndex, group[0].start));
        }

        if (shouldShowCarousel(group, reference)) {
          elements.push(
            <ImageCarousel
              key={`carousel-${groupIndex}`}
              group={group}
              reference={reference}
              fileThumbnails={fileThumbnails}
              onImageClick={handleDocumentButtonClick}
            />,
          );
        } else {
          group.forEach((ref) => {
            const chunkIndex = getChunkIndex(ref.id);
            const {
              documentUrl,
              fileExtension,
              imageId,
              chunkItem,
              documentId,
            } = getReferenceInfo(chunkIndex);
            const docType = chunkItem?.doc_type;

            if (showImage(docType)) {
              elements.push(
                <section key={ref.id}>
                  <Image
                    id={imageId}
                    className={styles.referenceInnerChunkImage}
                    onClick={
                      documentId
                        ? handleDocumentButtonClick(
                            documentId,
                            chunkItem,
                            fileExtension === 'pdf',
                            documentUrl,
                          )
                        : () => {}
                    }
                  />
                  <span className="text-accent-primary"> {imageId}</span>
                </section>,
              );
            } else {
              elements.push(
                <HoverCard key={ref.id}>
                  <HoverCardTrigger>
                    <CircleAlert className="size-4 inline-block" />
                  </HoverCardTrigger>
                  <HoverCardContent className="max-w-3xl">
                    {getPopoverContent(chunkIndex)}
                  </HoverCardContent>
                </HoverCard>,
              );
            }
          });
        }

        lastIndex = group[group.length - 1].end;
      });

      if (lastIndex < text.length) {
        elements.push(text.substring(lastIndex));
      }

      return elements;
    },
    [
      getPopoverContent,
      getReferenceInfo,
      handleDocumentButtonClick,
      reference,
      fileThumbnails,
    ],
  );

  return (
    <Markdown
      rehypePlugins={[rehypeWrapReference, rehypeKatex, rehypeRaw]}
      remarkPlugins={[remarkGfm, remarkMath]}
      className={styles.markdownContentWrapper}
      components={
        {
          'custom-typography': ({ children }: { children: string }) =>
            renderReference(children),
          code(props: any) {
            const { children, className, ...rest } = props;
            const restProps = omit(rest, 'node');
            const match = /language-(\w+)/.exec(className || '');
            return match ? (
              <SyntaxHighlighter
                {...restProps}
                PreTag="div"
                language={match[1]}
                wrapLongLines
              >
                {String(children).replace(/\n$/, '')}
              </SyntaxHighlighter>
            ) : (
              <code
                {...restProps}
                className={classNames(className, 'text-wrap')}
              >
                {children}
              </code>
            );
          },
        } as any
      }
    >
      {contentWithCursor}
    </Markdown>
  );
};

export default MarkdownContent;
