import { MarkdownRemarkPlugins } from '@/constants/markdown-remark-plugins';
import classNames from 'classnames';
import Markdown from 'react-markdown';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import {
  oneDark,
  oneLight,
} from 'react-syntax-highlighter/dist/esm/styles/prism';
import rehypeKatex from 'rehype-katex';
import rehypeRaw from 'rehype-raw';

import 'katex/dist/katex.min.css'; // `rehype-katex` does not import the CSS for you

import { preprocessLaTeX } from '@/utils/chat';
import { citationMarkerReg } from '@/utils/citation-utils';
import { getDirAttribute } from '@/utils/text-direction';
import { omit } from 'lodash';
import { useIsDarkTheme } from '../theme-provider';
import styles from './index.module.less';

const HighLightMarkdown = ({
  children,
}: {
  className?: string;
  children: string | null | undefined;
}) => {
  const isDarkTheme = useIsDarkTheme();
  // IMPORTANT: preprocessLaTeX() decodes &lt;/&gt;/&amp; back to raw HTML before
  // rehypeRaw parses the markdown. Sanitizing children *before* preprocessLaTeX
  // would let entity-encoded payloads bypass DOMPurify and inject HTML.
  // Sanitize the *post*-processed string instead. (Coderabbit CRITICAL #3486038798)
  const processed = children ? preprocessLaTeX(children) : children;
  const dir = children
    ? getDirAttribute(children.replace(citationMarkerReg, ''))
    : undefined;

  return (
    <div dir={dir} className={classNames(styles.text)}>
      <Markdown
        remarkPlugins={MarkdownRemarkPlugins}
        rehypePlugins={[rehypeRaw, rehypeKatex]}
        components={
          {
            p: ({ children, ...props }: any) => (
              <p {...omit(props, 'node')}>{children}</p>
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
        {processed}
      </Markdown>
    </div>
  );
};

export default HighLightMarkdown;
