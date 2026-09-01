/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import Image, { AuthenticatedImg } from '@/components/image';
import SvgIcon from '@/components/svg-icon';
import { MarkdownRemarkPlugins } from '@/constants/markdown-remark-plugins';
import { IReference, IReferenceChunk } from '@/interfaces/database/chat';
import { citationMarkerReg } from '@/utils/citation-utils';
import { getExtension } from '@/utils/document-util';
import { supportsSourceLocate } from '@/utils/source-locate';
import { getDirAttribute } from '@/utils/text-direction';
import DOMPurify from 'dompurify';
import { memo, useCallback, useEffect, useMemo } from 'react';
import Markdown from 'react-markdown';
import SyntaxHighlighter from 'react-syntax-highlighter';
import rehypeKatex from 'rehype-katex';
import rehypeRaw from 'rehype-raw';
import { RehypeSanitizeAssistantMarkdown } from '@/constants/markdown-rehype-plugins';
import { visitParents } from 'unist-util-visit-parents';

import { useTranslation } from 'react-i18next';

import 'katex/dist/katex.min.css'; // `rehype-katex` does not import the CSS for you

import { useFetchDocumentThumbnailsByIds } from '@/hooks/use-document-request';
import { useLoadingPause } from '@/hooks/use-loading-pause';
import {
  currentReg,
  escapeUnmatchedAngleBrackets,
  parseCitationIndex,
  preprocessLaTeX,
  replaceRetrievingToSection,
  replaceTextByOldReg,
  replaceThinkToSection,
  unescapeAngleBrackets,
} from '@/utils/chat';
import classNames from 'classnames';
import { omit } from 'lodash';
import pipe from 'lodash/fp/pipe';
import reactStringReplace from 'react-string-replace';
import { LoadingDots } from '../loading-dots';
import { Button } from '../ui/button';
import {
  HoverCard,
  HoverCardContent,
  HoverCardTrigger,
} from '../ui/hover-card';
import styles from './index.module.less';
import { sanitizeHtmlWithImagesAsText } from '@/utils/dom-util';
import { SafeImg } from '@/components/safe-img';

const getChunkIndex = (match: string) => parseCitationIndex(match);

// Wraps every text node so citation markers can be replaced by React elements.
// Defined at module scope: react-markdown rebuilds its whole processor whenever
// a plugin's identity changes, and while an answer streams this component
// re-renders many times a second.
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

const MarkdownRehypePlugins = [
  rehypeRaw,
  RehypeSanitizeAssistantMarkdown,
  rehypeWrapReference,
  rehypeKatex,
];

const MarkdownParagraph = ({ children, ...props }: any) => (
  <p {...props}>{children}</p>
);

const MarkdownCode = (props: any) => {
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
    <code {...restProps} className={classNames(className, 'text-wrap')}>
      {children}
    </code>
  );
};

const formatMetadataValue = (value: unknown) => {
  if (Array.isArray(value)) return value.join(', ');
  if (value === null || value === undefined) return '';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
};

// TODO: The display of the table is inconsistent with the display previously placed in the MessageItem.
const MarkdownContent = ({
  reference,
  clickDocumentButton,
  content,
  loading,
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
    // Escape standalone < and > outside matched <...> tags
    // so DOMPurify doesn't strip them as HTML.
    const safeContent = escapeUnmatchedAngleBrackets(content);

    let text = DOMPurify.sanitize(safeContent, {
      ADD_TAGS: ['think', 'section', 'details', 'summary', 'retrieving'],
      ADD_ATTR: ['class'],
    });

    // let text = content;
    if (text === '' && loading) {
      text = t('chat.searching');
    }
    const nextText = replaceTextByOldReg(text);
    const thinkSummary = loading
      ? `${t('chat.thinking')}...`
      : t('chat.thought');
    return unescapeAngleBrackets(
      pipe(
        (value: string) => replaceThinkToSection(value, thinkSummary),
        replaceRetrievingToSection,
        preprocessLaTeX,
      )(nextText),
    );
  }, [content, loading, t]);

  const documentIds = useMemo(() => {
    const docAggs = reference?.doc_aggs;
    return Array.isArray(docAggs) ? docAggs.map((x) => x.doc_id) : [];
  }, [reference?.doc_aggs]);

  // Skipping the empty case matters: this component is mounted once per message,
  // and setting a fresh empty array would re-render (and re-parse the markdown
  // of) every message that carries no reference at all.
  useEffect(() => {
    if (documentIds.length === 0) return;
    setDocumentIds(documentIds);
  }, [documentIds, setDocumentIds]);

  const handleDocumentButtonClick = useCallback(
    (
      documentId: string,
      chunk: IReferenceChunk,
      fileExtension: string,
      documentUrl?: string,
    ) =>
      () => {
        if (supportsSourceLocate(fileExtension) && clickDocumentButton) {
          clickDocumentButton(documentId, chunk);
          return;
        }
        if (!documentUrl) return;
        window.open(
          `/document/${documentId}?ext=${fileExtension}&resource=${'document'}`,
          '_blank',
        );
      },
    [clickDocumentButton],
  );

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
                __html: sanitizeHtmlWithImagesAsText(chunkItem?.content ?? ''),
              }}
              className={classNames(styles.chunkContentText)}
              dir="auto"
            ></div>
            {chunkItem?.document_metadata &&
              Object.keys(chunkItem.document_metadata).length > 0 && (
                <section className="space-y-1 border border-border-default rounded p-2">
                  {Object.entries(chunkItem.document_metadata).map(
                    ([key, value]) => (
                      <div key={key} className="text-xs">
                        <span className="text-text-secondary">{key}:</span>{' '}
                        <span className="text-text-primary">
                          {formatMetadataValue(value)}
                        </span>
                      </div>
                    ),
                  )}
                </section>
              )}
            {documentId && (
              <section className="flex gap-1">
                {fileThumbnail ? (
                  <AuthenticatedImg
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
                  className={'text-wrap p-0 flex-1 h-auto'}
                  onClick={handleDocumentButtonClick(
                    documentId,
                    chunkItem,
                    fileExtension,
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
      const replacedText = reactStringReplace(text, currentReg, (match, i) => {
        const chunkIndex = getChunkIndex(match);

        return (
          <HoverCard key={i}>
            <HoverCardTrigger>
              <bdi className="text-text-secondary bg-bg-card rounded-2xl px-1 mx-1 text-nowrap inline-block">
                {t('common.figure')} {chunkIndex + 1}
              </bdi>
            </HoverCardTrigger>
            <HoverCardContent className="max-w-3xl">
              {getPopoverContent(chunkIndex)}
            </HoverCardContent>
          </HoverCard>
        );
      });

      return replacedText;
    },
    [getPopoverContent, t],
  );

  const dir = getDirAttribute(content.replace(citationMarkerReg, ''));
  const showLoadingDots = useLoadingPause(loading, content);

  const markdownComponents = useMemo(
    () =>
      ({
        p: MarkdownParagraph,
        'custom-typography': ({ children }: { children: string }) =>
          renderReference(children),
        img: SafeImg,
        code: MarkdownCode,
      }) as any,
    [renderReference],
  );

  return (
    <div dir={dir} className={styles.markdownContentWrapper}>
      <Markdown
        rehypePlugins={MarkdownRehypePlugins}
        remarkPlugins={MarkdownRemarkPlugins}
        components={markdownComponents}
      >
        {contentWithCursor}
      </Markdown>
      {showLoadingDots && (
        <LoadingDots className="ml-1 inline-block text-text-secondary" />
      )}
    </div>
  );
};

export default memo(MarkdownContent);
