import React from 'react';

interface IProps extends React.PropsWithChildren {
  link: string;
  preventDefault?: boolean;
  color?: string;
}

const NewDocumentLink = ({
  children,
  link,
  preventDefault = false,
  color = 'rgb(15, 79, 170)',
}: IProps) => {
  return (
    <a
      target="_blank"
      onClick={!preventDefault ? undefined : (e) => e.preventDefault()}
      href={link}
      rel="noreferrer"
      style={{ color, wordBreak: 'break-all' }}
    >
      {children}
    </a>
  );
};

export default NewDocumentLink;
