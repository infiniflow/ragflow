import classNames from 'classnames';
import Markdown from 'react-markdown';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import rehypeRaw from 'rehype-raw';
import remarkGfm from 'remark-gfm';

import styles from './index.less';

const HightLightMarkdown = ({
  children,
}: {
  children: string | null | undefined;
}) => {
  return (
    <Markdown
      remarkPlugins={[remarkGfm]}
      rehypePlugins={[rehypeRaw]}
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
                // style={dark}
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
      {children}
    </Markdown>
  );
};

export default HightLightMarkdown;
