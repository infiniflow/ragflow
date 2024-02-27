import { api_host } from '@/utils/api';
import React from 'react';

interface IProps extends React.PropsWithChildren {
  documentId: string;
}

const NewDocumentLink = ({ children, documentId }: IProps) => {
  return (
    <a
      target="_blank"
      href={`${api_host}/document/get/${documentId}`}
      rel="noreferrer"
    >
      {children}
    </a>
  );
};

export default NewDocumentLink;
