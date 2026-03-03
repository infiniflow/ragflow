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
import { citationMarkerReg } from '@/utils/citation-utils';
import { getDirAttribute } from '@/utils/text-direction';
import { useIsDarkTheme } from '../theme-provider';
import styles from './index.module.less';

const HighLightMarkdown = ({
  children,
}: {
  children: string | null | undefined;
}) => {
  const isDarkTheme = useIsDarkTheme();
  const dir = children
    ? getDirAttribute(children.replace(citationMarkerReg, ''))
    : undefined;

  return (
    <div dir={dir} className={classNames(styles.text)}>
      <Markdown
        remarkPlugins={[remarkGfm, remarkMath]}
        rehypePlugins={[rehypeRaw, rehypeKatex]}
        components={
          {
            p: ({ children, node, ...props }: any) => (
              <p {...props}>{children}</p>
            ),
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
    </div>
  );
};

export default HighLightMarkdown;
