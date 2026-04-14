import Image from '@/components/image';
import SvgIcon from '@/components/svg-icon';
import { IReferenceChunk, IReferenceObject } from '@/interfaces/database/chat';
import { getExtension } from '@/utils/document-util';
import { downloadFileFromBlob } from '@/utils/file-util';
import request from '@/utils/request';
import DOMPurify from 'dompurify';
import { memo, useCallback, useEffect, useMemo, useState } from 'react';
import Markdown from 'react-markdown';
import SyntaxHighlighter from 'react-syntax-highlighter';
import rehypeKatex from 'rehype-katex';
import rehypeRaw from 'rehype-raw';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';
import { visitParents } from 'unist-util-visit-parents';

import { useTranslation } from 'react-i18next';

import 'katex/dist/katex.min.css'; // `rehype-katex` does not import the CSS for you

import {
  currentReg,
  parseCitationIndex,
  preprocessLaTeX,
  replaceTextByOldReg,
  replaceThinkToSection,
} from '@/utils/chat';
import { citationMarkerReg } from '@/utils/citation-utils';
import { getDirAttribute } from '@/utils/text-direction';

import { useFetchDocumentThumbnailsByIds } from '@/hooks/use-document-request';
import { cn } from '@/lib/utils';
import classNames from 'classnames';
import { omit } from 'lodash';
import { pipe } from 'lodash/fp';
import reactStringReplace from 'react-string-replace';
import { Button } from '../ui/button';
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '../ui/hover-card';
import message from '../ui/message';
import styles from './index.module.less';

const getChunkIndex = (match: string) => parseCitationIndex(match);

const isArtifactUrl = (url?: string) =>
  Boolean(url && url.includes('/document/artifact/'));

const fetchArtifactBlob = async (url: string): Promise<Blob> => {
  const response = await request(url, {
    method: 'GET',
    responseType: 'blob',
  });

  return response.data as Blob;
};

const getArtifactName = (url?: string, fallback?: string) =>
  fallback || url?.split('/').pop()?.split('?')[0] || 'artifact';

function ArtifactLink({
  href,
  className,
  children,
}: {
  href: string;
  className?: string;
  children: React.ReactNode;
}) {
  const handleClick = useCallback(
    async (e: React.MouseEvent<HTMLAnchorElement>) => {
      e.preventDefault();
      try {
        const blob = await fetchArtifactBlob(href);
        const objectUrl = URL.createObjectURL(blob);
        window.open(objectUrl, '_blank', 'noopener,noreferrer');
        window.setTimeout(() => URL.revokeObjectURL(objectUrl), 60 * 1000);
      } catch {
        message.error('Failed to open artifact');
      }
    },
    [href],
  );

  return (
    <a href={href} className={className} onClick={handleClick}>
      {children}
    </a>
  );
}

function ArtifactImage({
  src,
  alt,
  downloadLabel,
}: {
  src: string;
  alt?: string;
  downloadLabel: string;
}) {
  const [imageSrc, setImageSrc] = useState('');

  useEffect(() => {
    let objectUrl = '';
    let active = true;

    const load = async () => {
      try {
        const blob = await fetchArtifactBlob(src);
        objectUrl = URL.createObjectURL(blob);
        if (active) {
          setImageSrc(objectUrl);
        }
      } catch {
        message.error('Failed to load artifact image');
      }
    };

    load();

    return () => {
      active = false;
      if (objectUrl) {
        URL.revokeObjectURL(objectUrl);
      }
    };
  }, [alt, src]);

  const handleDownload = useCallback(async () => {
    try {
      const blob = await fetchArtifactBlob(src);
      downloadFileFromBlob(blob, getArtifactName(src, alt));
    } catch {
      message.error('Failed to download artifact');
    }
  }, [alt, src]);

  return (
    <span className={styles.artifactImageWrapper}>
      {imageSrc ? (
        <img src={imageSrc} alt={alt || ''} className={styles.artifactImage} />
      ) : (
        <span className={styles.artifactImage} />
      )}
      <button
        type="button"
        className={styles.artifactDownload}
        onClick={handleDownload}
      >
        {downloadLabel}
      </button>
    </span>
  );
}
// TODO: The display of the table is inconsistent with the display previously placed in the MessageItem.
function MarkdownContent({
  reference,
  clickDocumentButton,
  content,
}: {
  content: string;
  loading: boolean;
  reference?: IReferenceObject;
  clickDocumentButton?: (documentId: string, chunk: IReferenceChunk) => void;
}) {
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
      const chunks = reference?.chunks ?? {};
      const chunkItem = chunks[chunkIndex];

      const documentList = Object.values(reference?.doc_aggs ?? {});
      const document = documentList.find(
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

  const renderPopoverContent = useCallback(
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
                  className={cn(styles.referenceImagePreview)}
                ></Image>
              </HoverCardContent>
            </HoverCard>
          )}
          <div className={'space-y-2 max-w-[40vw] w-full'}>
            <div
              dangerouslySetInnerHTML={{
                __html: DOMPurify.sanitize(chunkItem?.content ?? ''),
              }}
              className={classNames(styles.chunkContentText, 'w-full')}
              dir="auto"
            ></div>
            {documentId && (
              <div className="flex gap-1">
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
                  onClick={handleDocumentButtonClick(
                    documentId,
                    chunkItem,
                    fileExtension === 'pdf',
                    documentUrl,
                  )}
                  className="text-ellipsis text-wrap"
                >
                  {document?.doc_name}
                </Button>
              </div>
            )}
          </div>
        </div>
      );
    },
    [getReferenceInfo, handleDocumentButtonClick],
  );

  const renderReference = useCallback(
    (text: string) => {
      const replacedText = reactStringReplace(text, currentReg, (match, i) => {
        const chunkIndex = getChunkIndex(match);

        return (
          <HoverCard key={i}>
            <HoverCardTrigger>
              <bdi className="text-text-secondary bg-bg-card rounded-2xl px-1 mx-1 text-nowrap inline-block">
                Fig. {chunkIndex + 1}
              </bdi>
            </HoverCardTrigger>
            <HoverCardContent className="max-w-3xl">
              {renderPopoverContent(chunkIndex)}
            </HoverCardContent>
          </HoverCard>
        );
      });

      return replacedText;
    },
    [renderPopoverContent],
  );

  const dir = getDirAttribute(content.replace(citationMarkerReg, ''));

  return (
    <div dir={dir} className={styles.markdownContentWrapper}>
      <Markdown
        rehypePlugins={[rehypeWrapReference, rehypeKatex, rehypeRaw]}
        remarkPlugins={[remarkGfm, remarkMath]}
        components={
          {
            p: ({ children, ...props }: any) => <p {...props}>{children}</p>,
            'custom-typography': ({ children }: { children: string }) =>
              renderReference(children),
            a({ href, children, ...props }: any) {
              if (isArtifactUrl(href)) {
                return (
                  <ArtifactLink href={href} className={styles.artifactDownload}>
                    {children}
                  </ArtifactLink>
                );
              }
              return (
                <a href={href} {...omit(props, 'node')}>
                  {children}
                </a>
              );
            },
            img({ src, alt, ...props }: any) {
              if (isArtifactUrl(src)) {
                return (
                  <ArtifactImage
                    src={src}
                    alt={alt || ''}
                    downloadLabel={t('common.download')}
                  />
                );
              }
              return (
                <span className={styles.artifactImageWrapper}>
                  <img
                    src={src}
                    alt={alt || ''}
                    className={styles.artifactImage}
                    {...omit(props, 'node')}
                  />
                </span>
              );
            },
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
    </div>
  );
}

export default memo(MarkdownContent);
