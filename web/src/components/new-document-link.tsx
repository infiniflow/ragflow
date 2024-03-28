import { api_host } from '@/utils/api';
import React from 'react';

interface IProps extends React.PropsWithChildren {
  documentId: string;
  preventDefault?: boolean;
}

const NewDocumentLink = ({
  children,
  documentId,
  preventDefault = false,
}: IProps) => {
  return (
    <a
      target="_blank"
      onClick={!preventDefault ? undefined : (e) => e.preventDefault()}
      href={`${api_host}/document/get/${documentId}`}
      rel="noreferrer"
    >
      {children}
    </a>
  );
};

export default NewDocumentLink;
