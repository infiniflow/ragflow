import { useRef } from 'react';
import { useScrollToBottom } from '../hooks/logic-hooks';

export function ChatWindow({ messages }: { messages: string[] }) {
  const containerRef = useRef<HTMLDivElement>(null);
  const { scrollRef, isAtBottom } = useScrollToBottom(messages, containerRef);

  return (
    <div
      ref={containerRef}
      style={{
        height: 300,
        overflowY: 'auto',
        border: '1px solid #ccc',
        position: 'relative',
        paddingBottom: 40,
      }}
    >
      {messages.map((msg, idx) => (
        <div key={idx} style={{ padding: 8 }}>
          {msg}
        </div>
      ))}
      <div ref={scrollRef} />
      {!isAtBottom && (
        <button
          style={{
            position: 'absolute',
            bottom: 8,
            right: 8,
            zIndex: 10,
            padding: '8px 16px',
            background: '#007bff',
            color: '#fff',
            border: 'none',
            borderRadius: 4,
            cursor: 'pointer',
          }}
          onClick={() => {
            scrollRef.current?.scrollIntoView({ behavior: 'smooth' });
          }}
        >
          Scroll Down
        </button>
      )}
    </div>
  );
}
