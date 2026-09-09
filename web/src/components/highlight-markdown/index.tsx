/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
import { RehypeSanitizeAssistantMarkdown } from '@/constants/markdown-rehype-plugins';

import 'katex/dist/katex.min.css'; // `rehype-katex` does not import the CSS for you

import { preprocessLaTeX } from '@/utils/chat';
import { citationMarkerReg } from '@/utils/citation-utils';
import { getDirAttribute } from '@/utils/text-direction';
import { omit } from 'lodash';
import { useIsDarkTheme } from '../theme-provider';
import { SafeImg } from '@/components/safe-img';
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
        rehypePlugins={[
          rehypeRaw,
          RehypeSanitizeAssistantMarkdown,
          rehypeKatex,
        ]}
        components={
          {
            p: ({ children, ...props }: any) => (
              <p {...omit(props, 'node')}>{children}</p>
            ),
            img: SafeImg,
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
