import Image from '@/components/image';
import SvgIcon from '@/components/svg-icon';
import {
  useFetchDocumentThumbnailsByIds,
  useGetDocumentUrl,
} from '@/hooks/document-hooks';
import { IReference, IReferenceChunk } from '@/interfaces/database/chat';
import {
  preprocessLaTeX,
  replaceThinkToSection,
  showImage,
} from '@/utils/chat';
import { getExtension } from '@/utils/document-util';
import { InfoCircleOutlined } from '@ant-design/icons';
import { Button, Flex, Popover, Tooltip } from 'antd';
import classNames from 'classnames';
import DOMPurify from 'dompurify';
import 'katex/dist/katex.min.css';
import { omit } from 'lodash';
import { pipe } from 'lodash/fp';
import { useCallback, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import Markdown from 'react-markdown';
import reactStringReplace from 'react-string-replace';
import SyntaxHighlighter from 'react-syntax-highlighter';
import {
  oneDark,
  oneLight,
} from 'react-syntax-highlighter/dist/esm/styles/prism';
import rehypeKatex from 'rehype-katex';
import rehypeRaw from 'rehype-raw';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';
import { visitParents } from 'unist-util-visit-parents';
import { currentReg, replaceTextByOldReg } from '../pages/next-chats/utils';
import styles from './floating-chat-widget-markdown.less';
import { useIsDarkTheme } from './theme-provider';

const getChunkIndex = (match: string) => Number(match.replace(/\[|\]/g, ''));

const FloatingChatWidgetMarkdown = ({
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
  const getDocumentUrl = useGetDocumentUrl();
  const isDarkTheme = useIsDarkTheme();

  const contentWithCursor = useMemo(() => {
    let text = content === '' ? t('chat.searching') : content;
    const nextText = replaceTextByOldReg(text);
    return pipe(replaceThinkToSection, preprocessLaTeX)(nextText);
  }, [content, t]);

  useEffect(() => {
    const docAggs = reference?.doc_aggs;
    const docList = Array.isArray(docAggs)
      ? docAggs
      : Object.values(docAggs ?? {});
    setDocumentIds(docList.map((x: any) => x.doc_id).filter(Boolean));
  }, [reference, setDocumentIds]);

  const handleDocumentButtonClick = useCallback(
    (
      documentId: string,
      chunk: IReferenceChunk,
      isPdf: boolean,
      documentUrl?: string,
    ) =>
      () => {
        if (!documentId) return;
        if (!isPdf && documentUrl) {
          window.open(documentUrl, '_blank');
        } else if (clickDocumentButton) {
          clickDocumentButton(documentId, chunk);
        }
      },
    [clickDocumentButton],
  );

  const rehypeWrapReference = () => (tree: any) => {
    visitParents(tree, 'text', (node, ancestors) => {
      const latestAncestor = ancestors[ancestors.length - 1];
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

  const getReferenceInfo = useCallback(
    (chunkIndex: number) => {
      const chunkItem = reference?.chunks?.[chunkIndex];
      if (!chunkItem) return null;
      const docAggsArray = Array.isArray(reference?.doc_aggs)
        ? reference.doc_aggs
        : Object.values(reference?.doc_aggs ?? {});
      const document = docAggsArray.find(
        (x: any) => x?.doc_id === chunkItem?.document_id,
      ) as any;
      const documentId = document?.doc_id;
      const documentUrl =
        document?.url ?? (documentId ? getDocumentUrl(documentId) : undefined);
      const fileThumbnail = documentId ? fileThumbnails[documentId] : '';
      const fileExtension = documentId
        ? getExtension(document?.doc_name ?? '')
        : '';
      return {
        documentUrl,
        fileThumbnail,
        fileExtension,
        imageId: chunkItem.image_id,
        chunkItem,
        documentId,
        document,
      };
    },
    [fileThumbnails, reference, getDocumentUrl],
  );

  const getPopoverContent = useCallback(
    (chunkIndex: number) => {
      const info = getReferenceInfo(chunkIndex);

      if (!info) {
        return (
          <div className="p-2 text-xs text-red-500">
            Error: Missing document information.
          </div>
        );
      }

      const {
        documentUrl,
        fileThumbnail,
        fileExtension,
        imageId,
        chunkItem,
        documentId,
        document,
      } = info;

      return (
        <div
          key={`popover-content-${chunkItem.id}`}
          className="flex gap-2 widget-citation-content"
        >
          {imageId && (
            <Popover
              placement="left"
              content={
                <Image
                  id={imageId}
                  className="max-w-[80vw] max-h-[60vh] rounded"
                />
              }
            >
              <Image
                id={imageId}
                className="w-24 h-24 object-contain rounded m-1 cursor-pointer"
              />
            </Popover>
          )}
          <div className="space-y-2 flex-1 min-w-0">
            <div
              dangerouslySetInnerHTML={{
                __html: DOMPurify.sanitize(chunkItem?.content ?? ''),
              }}
              className="max-h-[250px] overflow-y-auto text-xs leading-relaxed p-2 bg-gray-50 dark:bg-gray-800 rounded prose-sm"
            ></div>
            {documentId && (
              <Flex gap={'small'} align="center">
                {fileThumbnail ? (
                  <img
                    src={fileThumbnail}
                    alt={document?.doc_name}
                    className="w-6 h-6 rounded"
                  />
                ) : (
                  <SvgIcon name={`file-icon/${fileExtension}`} width={20} />
                )}
                <Tooltip
                  title={
                    !documentUrl && fileExtension !== 'pdf'
                      ? 'Document link unavailable'
                      : document.doc_name
                  }
                >
                  <Button
                    type="link"
                    size="small"
                    className="p-0 text-xs break-words h-auto text-left flex-1"
                    onClick={handleDocumentButtonClick(
                      documentId,
                      chunkItem,
                      fileExtension === 'pdf',
                      documentUrl,
                    )}
                    disabled={!documentUrl && fileExtension !== 'pdf'}
                    style={{ whiteSpace: 'normal' }}
                  >
                    <span className="truncate">
                      {document?.doc_name ?? 'Unnamed Document'}
                    </span>
                  </Button>
                </Tooltip>
              </Flex>
            )}
          </div>
        </div>
      );
    },
    [getReferenceInfo, handleDocumentButtonClick],
  );

  const renderReference = useCallback(
    (text: string) => {
      return reactStringReplace(text, currentReg, (match, i) => {
        const chunkIndex = getChunkIndex(match);
        const info = getReferenceInfo(chunkIndex);

        if (!info) {
          return (
            <Tooltip key={`err-tooltip-${i}`} title="Reference unavailable">
              <InfoCircleOutlined className={styles.referenceIcon} />
            </Tooltip>
          );
        }

        const { imageId, chunkItem, documentId, fileExtension, documentUrl } =
          info;

        if (showImage(chunkItem?.doc_type)) {
          return (
            <Image
              key={`img-${i}`}
              id={imageId}
              className="block object-contain max-w-full max-h-48 rounded my-2 cursor-pointer"
              onClick={handleDocumentButtonClick(
                documentId,
                chunkItem,
                fileExtension === 'pdf',
                documentUrl,
              )}
            />
          );
        }

        return (
          <Popover content={getPopoverContent(chunkIndex)} key={`popover-${i}`}>
            <InfoCircleOutlined className={styles.referenceIcon} />
          </Popover>
        );
      });
    },
    [getPopoverContent, getReferenceInfo, handleDocumentButtonClick],
  );

  return (
    <div className="floating-chat-widget">
      <Markdown
        rehypePlugins={[rehypeWrapReference, rehypeKatex, rehypeRaw]}
        remarkPlugins={[remarkGfm, remarkMath]}
        className="text-sm leading-relaxed space-y-2 prose-sm max-w-full"
        components={
          {
            'custom-typography': ({ children }: { children: string }) =>
              renderReference(children),
            code(props: any) {
              // eslint-disable-next-line @typescript-eslint/no-unused-vars
              const { children, className, node, ...rest } = props;
              const match = /language-(\w+)/.exec(className || '');
              return match ? (
                <SyntaxHighlighter
                  {...omit(rest, 'inline')}
                  PreTag="div"
                  language={match[1]}
                  style={isDarkTheme ? oneDark : oneLight}
                  wrapLongLines
                >
                  {String(children).replace(/\n$/, '')}
                </SyntaxHighlighter>
              ) : (
                <code
                  {...rest}
                  className={classNames(
                    className,
                    'text-wrap text-xs bg-gray-200 dark:bg-gray-700 px-1 py-0.5 rounded',
                  )}
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
    </div>
  );
};

export default FloatingChatWidgetMarkdown;
