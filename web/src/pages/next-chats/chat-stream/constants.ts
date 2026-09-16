// Maximum number of idle chat sessions kept in the streaming store. Sessions
// that are streaming, or the one currently being acted on, are never evicted.
export const MaxCachedChatSessions = 10;

// Stable id prefix for the seeded prologue message, so re-seeding the same
// conversation is idempotent under React StrictMode's double effects.
export const PrologueMessageIdPrefix = 'prologue_';

// Minimum gap between two store writes while a stream is running. SSE chunks
// arrive far faster than markdown can be re-rendered, and every write copies the
// message list and re-renders the message item, so coalescing them cuts the cost
// without any visible change to how the answer grows. The final chunk always
// flushes immediately.
export const AnswerFlushIntervalMs = 60;

// Redux DevTools keeps a full state snapshot per action. Streaming dispatches one
// action per chunk, each carrying the whole accumulated answer, so recording it
// makes a long answer quadratically expensive to produce — and it keeps costing
// after the chat page unmounts, which is what makes navigating away mid-answer
// feel slow. Flip to true only while debugging the store.
export const EnableChatStreamDevtools = false;
