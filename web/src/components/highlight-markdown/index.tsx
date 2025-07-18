import classNames from 'classnames';
import Markdown from 'react-markdown';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism';
import rehypeKatex from 'rehype-katex';
import rehypeRaw from 'rehype-raw';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';

import 'katex/dist/katex.min.css'; // `rehype-katex` does not import the CSS for you

import { preprocessLaTeX } from '@/utils/chat';
import styles from './index.less';

const HightLightMarkdown = ({
  children,
  dark = false,
}: {
  children: string | null | undefined;
  dark?: boolean;
}) => {
  return (
    <Markdown
      remarkPlugins={[remarkGfm, remarkMath]}
      rehypePlugins={[rehypeRaw, rehypeKatex]}
      className={classNames(styles.text)}
      components={
        {
          code(props: any) {
            const { children, className, node, ...rest } = props;
            const match = /language-(\w+)/.exec(className || '');
            return match ? (
              <SyntaxHighlighter
                {...rest}
                PreTag="div"
                language={match[1]}
                style={dark && oneDark}
              >
                {String(children).replace(/\n$/, '')}
              </SyntaxHighlighter>
            ) : (
              <code {...rest} className={`${className} ${styles.code}`}>
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
