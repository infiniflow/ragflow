import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import api from '@/utils/api';
import { getAuthorization } from '@/utils/authorization-util';
import DOMPurify from 'dompurify';
import {
  BookOpen,
  ExternalLink,
  Loader2,
  MessageCircleQuestion,
  Send,
  X,
} from 'lucide-react';
import { memo, useCallback, useEffect, useRef, useState } from 'react';
import Markdown from 'react-markdown';

interface DocMessage {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  references?: DocReference[];
  timestamp: number;
}

interface DocReference {
  source: string;
  heading: string;
  url: string;
}

const SUGGESTED_QUESTIONS = [
  'How do I get started with RAGFlow?',
  'How do I configure an embedding model?',
  'How do I create a knowledge base?',
  'How do I deploy RAGFlow with Docker?',
];

function DocAssistantWidget() {
  const [isOpen, setIsOpen] = useState(false);
  const [isReady, setIsReady] = useState(false);
  const [messages, setMessages] = useState<DocMessage[]>([
    {
      id: 'welcome',
      role: 'assistant',
      content:
        "Hi! I'm the RAGFlow Documentation Assistant. Ask me anything about RAGFlow configuration, features, or troubleshooting.",
      timestamp: Date.now(),
    },
  ]);
  const [input, setInput] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const abortRef = useRef<AbortController | null>(null);

  useEffect(() => {
    let cancelled = false;
    const checkStatus = async () => {
      try {
        const res = await fetch(api.docAssistantStatus, {
          headers: { Authorization: getAuthorization() },
        });
        const json = await res.json();
        if (!cancelled && json.code === 0 && json.data?.enabled) {
          setIsReady(true);
        }
      } catch {
        // assistant unavailable — keep hidden
      }
    };
    checkStatus();
    return () => {
      cancelled = true;
    };
  }, []);

  const scrollToBottom = useCallback(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, []);

  useEffect(() => {
    scrollToBottom();
  }, [messages, scrollToBottom]);

  useEffect(() => {
    if (isOpen && inputRef.current) {
      inputRef.current.focus();
    }
  }, [isOpen]);

  const sendMessage = useCallback(
    async (question: string) => {
      if (!question.trim() || isLoading) return;

      const userMsg: DocMessage = {
        id: `user-${Date.now()}`,
        role: 'user',
        content: question.trim(),
        timestamp: Date.now(),
      };
      setMessages((prev) => [...prev, userMsg]);
      setInput('');
      setIsLoading(true);

      const history = messages
        .filter((m) => m.id !== 'welcome')
        .map((m) => ({ role: m.role, content: m.content }));

      const assistantMsgId = `assistant-${Date.now()}`;
      setMessages((prev) => [
        ...prev,
        {
          id: assistantMsgId,
          role: 'assistant',
          content: '',
          references: [],
          timestamp: Date.now(),
        },
      ]);

      try {
        abortRef.current = new AbortController();
        const response = await fetch(api.docAssistantAsk, {
          method: 'POST',
          headers: {
            Authorization: getAuthorization(),
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            question: question.trim(),
            history,
            stream: true,
          }),
          signal: abortRef.current.signal,
        });

        const reader = response.body?.getReader();
        const decoder = new TextDecoder();
        let fullAnswer = '';
        let references: DocReference[] = [];

        if (reader) {
          let buffer = '';
          let reading = true;
          while (reading) {
            const { done, value } = await reader.read();
            if (done) {
              reading = false;
              continue;
            }
            buffer += decoder.decode(value, { stream: true });

            const lines = buffer.split('\n');
            buffer = lines.pop() || '';

            for (const line of lines) {
              const trimmed = line.trim();
              if (!trimmed.startsWith('data:')) continue;
              const jsonStr = trimmed.slice(5);
              try {
                const parsed = JSON.parse(jsonStr);
                if (parsed.data === true) continue;
                if (parsed.code !== 0) {
                  fullAnswer += parsed.data?.answer || parsed.message || '';
                  continue;
                }
                const chunk = parsed.data;
                if (chunk?.answer) {
                  fullAnswer = chunk.answer;
                }
                if (chunk?.references?.length) {
                  references = chunk.references;
                }
              } catch {
                // skip malformed SSE lines
              }
            }

            setMessages((prev) =>
              prev.map((m) =>
                m.id === assistantMsgId
                  ? { ...m, content: fullAnswer, references }
                  : m,
              ),
            );
          }
        }

        if (!fullAnswer) {
          setMessages((prev) =>
            prev.map((m) =>
              m.id === assistantMsgId
                ? { ...m, content: 'Sorry, I could not find an answer.' }
                : m,
            ),
          );
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === 'AbortError') return;
        setMessages((prev) =>
          prev.map((m) =>
            m.id === assistantMsgId
              ? {
                  ...m,
                  content:
                    'Failed to connect to the documentation assistant. Please check your network and try again.',
                }
              : m,
          ),
        );
      } finally {
        abortRef.current = null;
        setIsLoading(false);
      }
    },
    [isLoading, messages],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        sendMessage(input);
      }
    },
    [input, sendMessage],
  );

  const handleSuggestedQuestion = useCallback(
    (question: string) => {
      sendMessage(question);
    },
    [sendMessage],
  );

  if (!isReady) return null;

  return (
    <>
      {/* Floating trigger button */}
      <button
        onClick={() => setIsOpen((prev) => !prev)}
        className={cn(
          'fixed bottom-6 right-6 z-50 flex items-center justify-center',
          'size-12 rounded-full shadow-lg transition-all duration-300',
          'bg-colors-background-inverse-standard text-white',
          'hover:scale-110 hover:shadow-xl',
          isOpen && 'rotate-0',
        )}
        aria-label="Toggle Documentation Assistant"
      >
        {isOpen ? (
          <X className="size-5" />
        ) : (
          <MessageCircleQuestion className="size-5" />
        )}
      </button>

      {/* Chat panel */}
      {isOpen && (
        <div
          className={cn(
            'fixed bottom-20 right-4 sm:right-6 z-50',
            'w-[calc(100vw-2rem)] sm:w-[400px] h-[560px] max-h-[80vh]',
            'rounded-2xl shadow-2xl border border-border-default',
            'bg-bg-body flex flex-col overflow-hidden',
            'animate-in slide-in-from-bottom-4 fade-in duration-300',
          )}
        >
          {/* Header */}
          <div className="flex items-center gap-3 px-4 py-3 border-b border-border-default bg-colors-background-inverse-standard text-white">
            <BookOpen className="size-5 shrink-0" />
            <div className="flex-1 min-w-0">
              <h3 className="text-sm font-semibold leading-tight">
                Documentation Assistant
              </h3>
              <p className="text-xs opacity-80 truncate">
                Ask anything about RAGFlow
              </p>
            </div>
            <button
              onClick={() => setIsOpen(false)}
              className="p-1 rounded hover:bg-white/20 transition-colors"
              aria-label="Close"
            >
              <X className="size-4" />
            </button>
          </div>

          {/* Messages area */}
          <div
            ref={scrollRef}
            className="flex-1 overflow-y-auto p-4 space-y-4 scrollbar-auto"
          >
            {messages.map((msg) => (
              <div
                key={msg.id}
                className={cn(
                  'flex',
                  msg.role === 'user' ? 'justify-end' : 'justify-start',
                )}
              >
                <div
                  className={cn(
                    'max-w-[85%] rounded-2xl px-3.5 py-2.5 text-sm',
                    msg.role === 'user'
                      ? 'bg-colors-background-inverse-standard text-white rounded-br-sm'
                      : 'bg-colors-background-neutral-standard text-text-title rounded-bl-sm',
                  )}
                >
                  {msg.role === 'assistant' ? (
                    <div className="space-y-2">
                      <Markdown
                        className="prose prose-sm max-w-none dark:prose-invert [&_p]:mb-1 [&_p]:mt-0 [&_ul]:my-1 [&_ol]:my-1 [&_li]:my-0 [&_pre]:my-1"
                        components={{
                          a: ({ href, children, ...props }) => (
                            <a
                              href={href}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="text-blue-400 hover:underline"
                              {...props}
                            >
                              {children}
                            </a>
                          ),
                          code: ({ children, className, ...props }) => {
                            const isInline = !className;
                            return isInline ? (
                              <code
                                className="bg-black/10 dark:bg-white/10 rounded px-1 py-0.5 text-xs"
                                {...props}
                              >
                                {children}
                              </code>
                            ) : (
                              <code
                                className={cn(className, 'text-xs')}
                                {...props}
                              >
                                {children}
                              </code>
                            );
                          },
                        }}
                      >
                        {DOMPurify.sanitize(msg.content)}
                      </Markdown>

                      {/* References */}
                      {msg.references && msg.references.length > 0 && (
                        <div className="mt-2 pt-2 border-t border-border-default/30">
                          <p className="text-xs font-medium opacity-70 mb-1">
                            Sources:
                          </p>
                          <div className="space-y-1">
                            {msg.references.map((ref, idx) => (
                              <a
                                key={idx}
                                href={ref.url}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="flex items-center gap-1.5 text-xs text-blue-500 hover:text-blue-600 hover:underline"
                              >
                                <ExternalLink className="size-3 shrink-0" />
                                <span className="truncate">{ref.heading}</span>
                              </a>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  ) : (
                    <p className="whitespace-pre-wrap">{msg.content}</p>
                  )}
                </div>
              </div>
            ))}

            {/* Loading indicator */}
            {isLoading && (
              <div className="flex justify-start">
                <div className="bg-colors-background-neutral-standard rounded-2xl rounded-bl-sm px-4 py-3">
                  <div className="flex items-center gap-2 text-sm text-text-sub-title">
                    <Loader2 className="size-4 animate-spin" />
                    <span>Searching docs...</span>
                  </div>
                </div>
              </div>
            )}

            {/* Suggested questions (only show initially) */}
            {messages.length === 1 && !isLoading && (
              <div className="space-y-2">
                <p className="text-xs text-text-sub-title">Try asking:</p>
                {SUGGESTED_QUESTIONS.map((q) => (
                  <button
                    key={q}
                    onClick={() => handleSuggestedQuestion(q)}
                    className={cn(
                      'block w-full text-left text-xs px-3 py-2 rounded-lg',
                      'border border-border-default hover:bg-colors-background-neutral-standard',
                      'transition-colors text-text-title',
                    )}
                  >
                    {q}
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* Input area */}
          <div className="p-3 border-t border-border-default">
            <div className="flex items-end gap-2">
              <textarea
                ref={inputRef}
                value={input}
                onChange={(e) => setInput(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder="Ask about RAGFlow..."
                rows={1}
                className={cn(
                  'flex-1 resize-none rounded-xl border border-border-default',
                  'bg-colors-background-neutral-standard px-3 py-2 text-sm',
                  'placeholder:text-text-sub-title',
                  'focus:outline-none focus:ring-2 focus:ring-ring',
                  'max-h-24 scrollbar-auto',
                )}
                disabled={isLoading}
              />
              <Button
                size="icon"
                className="size-9 rounded-xl shrink-0"
                disabled={!input.trim() || isLoading}
                onClick={() => sendMessage(input)}
              >
                <Send className="size-4" />
              </Button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}

export default memo(DocAssistantWidget);
