import Markdown from 'react-markdown';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import rehypeKatex from 'rehype-katex';
import rehypeRaw from 'rehype-raw';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';

import 'katex/dist/katex.min.css'; // `rehype-katex` does not import the CSS for you

import { preprocessLaTeX } from '@/utils/chat';

const HightLightMarkdown = ({
  children,
}: {
  children: string | null | undefined;
}) => {
  return (
    <Markdown
      remarkPlugins={[remarkGfm, remarkMath]}
      rehypePlugins={[rehypeRaw, rehypeKatex]}
      className="text-text-primary text-sm"
      components={
        {
          code(props: any) {
            const { children, className, ...rest } = props;
            const match = /language-(\w+)/.exec(className || '');
            return match ? (
              <SyntaxHighlighter {...rest} PreTag="div" language={match[1]}>
                {String(children).replace(/\n$/, '')}
              </SyntaxHighlighter>
            ) : (
              <code
                {...rest}
                className={`${className} pt-1 px-2 pb-2 m-0 whitespace-break-spaces rounded text-text-primary text-sm`}
              >
                {children}
              </code>
            );
          },
        } as any
      }
    >
      {children ? preprocessLaTeX(children) : children}
    </Markdown>
  );
};

export default HightLightMarkdown;
