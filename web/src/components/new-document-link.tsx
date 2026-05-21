import {
  getExtension,
  isSupportedPreviewDocumentType,
} from '@/utils/document-util';
import React from 'react';

interface IProps extends React.PropsWithChildren {
  link?: string;
  preventDefault?: boolean;
  color?: string;
  documentName: string;
  documentId?: string;
  resource?: 'document' | 'files';
  className?: string;
}

const NewDocumentLink = ({
  children,
  link,
  preventDefault = false,
  color = 'rgb(15, 79, 170)',
  documentId,
  documentName,
  resource = 'document',
  className,
}: IProps) => {
  let nextLink = link;
  const extension = getExtension(documentName);
  if (!link) {
    nextLink = `/document/${documentId}?ext=${extension}&resource=${resource}`;
  }

  return (
    <a
      target="_blank"
      onClick={
        !preventDefault || isSupportedPreviewDocumentType(extension)
          ? undefined
          : (e) => e.preventDefault()
      }
      href={nextLink}
      rel="noreferrer"
      style={{ color: className ? '' : color, wordBreak: 'break-all' }}
      className={className}
    >
      {children}
    </a>
  );
};

export default NewDocumentLink;
