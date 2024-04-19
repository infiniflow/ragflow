import Markdown from 'react-markdown';
import SyntaxHighlighter from 'react-syntax-highlighter';
import remarkGfm from 'remark-gfm';

const HightLightMarkdown = ({
  children,
}: {
  children: string | null | undefined;
}) => {
  return (
    <Markdown
      remarkPlugins={[remarkGfm]}
      components={
        {
          code(props: any) {
            const { children, className, node, ...rest } = props;
            const match = /language-(\w+)/.exec(className || '');
            return match ? (
              <SyntaxHighlighter {...rest} PreTag="div" language={match[1]}>
                {String(children).replace(/\n$/, '')}
              </SyntaxHighlighter>
            ) : (
              <code {...rest} className={className}>
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
