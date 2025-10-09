import { useEffect, useRef, useState } from 'react';
import { IJsonContainerProps, IObjContainerProps } from './interface';

export const useParserInit = ({
  initialValue,
}: {
  initialValue:
    | IJsonContainerProps['initialValue']
    | IObjContainerProps['initialValue'];
}) => {
  const [content, setContent] = useState(initialValue);

  useEffect(() => {
    setContent(initialValue);
    console.log('initialValue json parse', initialValue);
  }, [initialValue]);

  const [activeEditIndex, setActiveEditIndex] = useState<number | undefined>(
    undefined,
  );
  const editDivRef = useRef<HTMLDivElement>(null);

  return {
    content,
    setContent,
    activeEditIndex,
    setActiveEditIndex,
    editDivRef,
  };
};
