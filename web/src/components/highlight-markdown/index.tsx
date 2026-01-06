import classNames from 'classnames';
import Markdown from 'react-markdown';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import {
  oneDark,
  oneLight,
} from 'react-syntax-highlighter/dist/esm/styles/prism';
import rehypeKatex from 'rehype-katex';
import rehypeRaw from 'rehype-raw';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';

import 'katex/dist/katex.min.css'; // `rehype-katex` does not import the CSS for you

import { preprocessLaTeX } from '@/utils/chat';
import { useIsDarkTheme } from '../theme-provider';
import styles from './index.module.less';

const HighLightMarkdown = ({
  children,
}: {
  children: string | null | undefined;
}) => {
  const isDarkTheme = useIsDarkTheme();

  return (
    <Markdown
      remarkPlugins={[remarkGfm, remarkMath]}
      rehypePlugins={[rehypeRaw, rehypeKatex]}
      className={classNames(styles.text)}
      components={
        {
          code(props: any) {
            const { children, className, ...rest } = props;
            const match = /language-(\w+)/.exec(className || '');
            return match ? (
              <SyntaxHighlighter
                {...rest}
                PreTag="div"
                language={match[1]}
                style={isDarkTheme ? oneDark : oneLight}
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

export default HighLightMarkdown;
