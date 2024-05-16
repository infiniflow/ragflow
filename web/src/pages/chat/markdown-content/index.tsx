import Image from '@/components/image';
import SvgIcon from '@/components/svg-icon';
import { useSelectFileThumbnails } from '@/hooks/knowledgeHook';
import { IReference } from '@/interfaces/database/chat';
import { IChunk } from '@/interfaces/database/knowledge';
import { getExtension } from '@/utils/documentUtils';
import { InfoCircleOutlined } from '@ant-design/icons';
import { Button, Flex, Popover, Space } from 'antd';
import { useCallback } from 'react';
import Markdown from 'react-markdown';
import reactStringReplace from 'react-string-replace';
import SyntaxHighlighter from 'react-syntax-highlighter';
import remarkGfm from 'remark-gfm';
import { visitParents } from 'unist-util-visit-parents';

import styles from './index.less';

const reg = /(#{2}\d+\${2})/g;
const curReg = /(~{2}\d+\${2})/g;

const getChunkIndex = (match: string) => Number(match.slice(2, -2));
// TODO: The display of the table is inconsistent with the display previously placed in the MessageItem.
const MarkdownContent = ({
  reference,
  clickDocumentButton,
  content,
}: {
  content: string;
  reference: IReference;
  clickDocumentButton: (documentId: string, chunk: IChunk) => void;
}) => {
  const fileThumbnails = useSelectFileThumbnails();

  const handleDocumentButtonClick = useCallback(
    (documentId: string, chunk: IChunk, isPdf: boolean) => () => {
      if (!isPdf) {
        return;
      }
      clickDocumentButton(documentId, chunk);
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

  const getPopoverContent = useCallback(
    (chunkIndex: number) => {
      const chunks = reference?.chunks ?? [];
      const chunkItem = chunks[chunkIndex];
      const document = reference?.doc_aggs?.find(
        (x) => x?.doc_id === chunkItem?.doc_id,
      );
      const documentId = document?.doc_id;
      const fileThumbnail = documentId ? fileThumbnails[documentId] : '';
      const fileExtension = documentId ? getExtension(document?.doc_name) : '';
      const imageId = chunkItem?.img_id;
      return (
        <Flex
          key={chunkItem?.chunk_id}
          gap={10}
          className={styles.referencePopoverWrapper}
        >
          {imageId && (
            <Popover
              placement="left"
              content={
                <Image
                  id={imageId}
                  className={styles.referenceImagePreview}
                ></Image>
              }
            >
              <Image
                id={imageId}
                className={styles.referenceChunkImage}
              ></Image>
            </Popover>
          )}
          <Space direction={'vertical'}>
            <div
              dangerouslySetInnerHTML={{
                __html: chunkItem?.content_with_weight,
              }}
              className={styles.chunkContentText}
            ></div>
            {documentId && (
              <Flex gap={'small'}>
                {fileThumbnail ? (
                  <img src={fileThumbnail} alt="" />
                ) : (
                  <SvgIcon
                    name={`file-icon/${fileExtension}`}
                    width={24}
                  ></SvgIcon>
                )}
                <Button
                  type="link"
                  className={styles.documentLink}
                  onClick={handleDocumentButtonClick(
                    documentId,
                    chunkItem,
                    fileExtension === 'pdf',
                  )}
                >
                  {document?.doc_name}
                </Button>
              </Flex>
            )}
          </Space>
        </Flex>
      );
    },
    [reference, fileThumbnails, handleDocumentButtonClick],
  );

  const renderReference = useCallback(
    (text: string) => {
      let replacedText = reactStringReplace(text, reg, (match, i) => {
        const chunkIndex = getChunkIndex(match);
        return (
          <Popover content={getPopoverContent(chunkIndex)}>
            <InfoCircleOutlined key={i} className={styles.referenceIcon} />
          </Popover>
        );
      });

      replacedText = reactStringReplace(replacedText, curReg, (match, i) => (
        <span className={styles.cursor} key={i}></span>
      ));

      return replacedText;
    },
    [getPopoverContent],
  );

  return (
    <Markdown
      rehypePlugins={[rehypeWrapReference]}
      remarkPlugins={[remarkGfm]}
      components={
        {
          'custom-typography': ({ children }: { children: string }) =>
            renderReference(children),
          code(props: any) {
            const { children, className, node, ...rest } = props;
            const match = /language-(\w+)/.exec(className || '');
            return match ? (
              <SyntaxHighlighter {...rest} PreTag="div" language={match[1]}>
                {String(children).replace(/\n$/, '')}
              </SyntaxHighlighter>
            ) : (
              <code {...rest} className={className}>
                {children}
              </code>
            );
          },
        } as any
      }
    >
      {content}
    </Markdown>
  );
};

export default MarkdownContent;
