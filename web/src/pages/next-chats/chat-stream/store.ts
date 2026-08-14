/**
 * Module-level store for the single-chat SSE stream.
 *
 * The message list, the streaming flag and the AbortController live here rather
 * than in component state so an in-flight stream survives unmounting
 * `/chat/:id`. The reader loop (see run-stream.ts) writes straight into this
 * store, so navigating away and back re-subscribes to a stream that never
 * stopped.
 */
import { IAnswer, IMessage } from '@/interfaces/database/chat';
import { MessageType } from '@/constants/chat';
import { create } from 'zustand';
import { devtools } from 'zustand/middleware';
import { MaxCachedChatSessions, EnableChatStreamDevtools } from './constants';
import {
  buildAssistantMessageFromAnswer,
  buildPrologueMessageId,
  buildQuestionAndPlaceholder,
  mergeLocalFiles,
} from './utils';

export type ChatStreamSession = {
  chatId: string;
  messages: IMessage[];
  isStreaming: boolean;
  abortController?: AbortController;
  // Text of a question whose stream failed. Held per session because the
  // failure can land while the user is on another conversation or page, where
  // restoring the input box directly would either be a no-op or type into the
  // wrong session's input.
  pendingInput?: string;
  lastActiveAt: number;
};

export type ChatStreamState = {
  sessions: Record<string, ChatStreamSession>;

  ensureSession(conversationId: string, chatId: string): void;
  seedPrologue(conversationId: string, chatId: string, prologue: string): void;
  hydrateFromServer(conversationId: string, messages: IMessage[]): void;
  appendQuestion(conversationId: string, message: IMessage): void;
  beginStream(conversationId: string, controller: AbortController): boolean;
  applyAnswer(conversationId: string, answer: IAnswer): void;
  failStream(conversationId: string, pendingInput: string): void;
  consumePendingInput(conversationId: string): void;
  removeMessageById(conversationId: string, messageId: string): void;
  endStream(conversationId: string): void;
  stopStream(conversationId: string): void;
  removeSessions(conversationIds: string[]): void;
};

function createSession(chatId: string): ChatStreamSession {
  return {
    chatId,
    messages: [],
    isStreaming: false,
    lastActiveAt: Date.now(),
  };
}

/**
 * Drops the least recently used idle sessions once the cache exceeds
 * MaxCachedChatSessions. Never evicts a streaming session or the session being
 * acted on. Only called from actions that can create a new entry — never from
 * the per-chunk hot path.
 */
function evictIdleSessions(
  sessions: Record<string, ChatStreamSession>,
  protectedId: string,
): Record<string, ChatStreamSession> {
  const ids = Object.keys(sessions);
  if (ids.length <= MaxCachedChatSessions) return sessions;

  const evictable = ids
    .filter((id) => id !== protectedId && !sessions[id].isStreaming)
    .sort((a, b) => sessions[a].lastActiveAt - sessions[b].lastActiveAt);

  const removeCount = ids.length - MaxCachedChatSessions;
  const next = { ...sessions };
  evictable.slice(0, removeCount).forEach((id) => {
    delete next[id];
  });
  return next;
}

export const useChatStreamStore = create<ChatStreamState>()(
  devtools(
    (set, get) => ({
      sessions: {},

      ensureSession: (conversationId, chatId) => {
        if (!conversationId) return;
        if (get().sessions[conversationId]) return;
        set(
          (state) => ({
            sessions: evictIdleSessions(
              {
                ...state.sessions,
                [conversationId]: createSession(chatId),
              },
              conversationId,
            ),
          }),
          false,
          'ensureSession',
        );
      },

      seedPrologue: (conversationId, chatId, prologue) => {
        if (!conversationId) return;
        const session = get().sessions[conversationId];
        // Idempotent: a session that already holds messages (a seeded prologue
        // under StrictMode's double effect, or a real exchange) is left alone.
        if (session && session.messages.length > 0) return;

        const prologueMessage: IMessage = {
          role: MessageType.Assistant,
          content: prologue,
          id: buildPrologueMessageId(conversationId),
          conversationId,
        };

        set(
          (state) => ({
            sessions: evictIdleSessions(
              {
                ...state.sessions,
                [conversationId]: {
                  ...(state.sessions[conversationId] ?? createSession(chatId)),
                  messages: [prologueMessage],
                  lastActiveAt: Date.now(),
                },
              },
              conversationId,
            ),
          }),
          false,
          'seedPrologue',
        );
      },

      hydrateFromServer: (conversationId, messages) => {
        if (!conversationId) return;
        const session = get().sessions[conversationId];
        // A stream in flight owns the list: the answer it is building hasn't
        // been persisted yet, so anything the server returns is behind.
        if (session?.isStreaming) return;
        // Otherwise compare lengths instead of latching authority to the local
        // list forever. A shorter server list is a stale snapshot taken before
        // the latest exchange was persisted — accepting it would truncate a
        // just-finished answer (the pre-existing race). An equal or longer list
        // is at least as new as what we hold, and it carries the real message
        // ids, references and prompts that the SSE payload lacks, plus any
        // exchange added from another tab.
        if (session && messages.length < session.messages.length) return;

        set(
          (state) => {
            const previous = state.sessions[conversationId];
            return {
              sessions: evictIdleSessions(
                {
                  ...state.sessions,
                  [conversationId]: {
                    ...(previous ?? createSession('')),
                    messages: mergeLocalFiles(
                      messages,
                      previous?.messages ?? [],
                    ),
                    lastActiveAt: Date.now(),
                  },
                },
                conversationId,
              ),
            };
          },
          false,
          'hydrateFromServer',
        );
      },

      appendQuestion: (conversationId, message) => {
        if (!conversationId) return;
        set(
          (state) => {
            const previous =
              state.sessions[conversationId] ?? createSession('');
            return {
              sessions: evictIdleSessions(
                {
                  ...state.sessions,
                  [conversationId]: {
                    ...previous,
                    messages: [
                      ...previous.messages,
                      ...buildQuestionAndPlaceholder(message),
                    ],
                    lastActiveAt: Date.now(),
                  },
                },
                conversationId,
              ),
            };
          },
          false,
          'appendQuestion',
        );
      },

      beginStream: (conversationId, controller) => {
        if (!conversationId) return false;
        // Guard against a duplicate start (StrictMode, double submit).
        if (get().sessions[conversationId]?.isStreaming) return false;

        set(
          (state) => {
            const previous =
              state.sessions[conversationId] ?? createSession('');
            return {
              sessions: {
                ...state.sessions,
                [conversationId]: {
                  ...previous,
                  isStreaming: true,
                  abortController: controller,
                  lastActiveAt: Date.now(),
                },
              },
            };
          },
          false,
          'beginStream',
        );
        return true;
      },

      applyAnswer: (conversationId, answer) => {
        if (!conversationId) return;
        set(
          (state) => {
            const previous = state.sessions[conversationId];
            if (!previous) return state;
            // A session that is no longer streaming has no trailing placeholder
            // to fill in. The reader's final flush runs before endStream, so
            // this also guards the case where the in-flight question (and its
            // placeholder) were just deleted: without it, the flush would slice
            // off whatever message is now last.
            if (!previous.isStreaming) return state;
            return {
              sessions: {
                ...state.sessions,
                [conversationId]: {
                  ...previous,
                  messages: [
                    ...previous.messages.slice(0, -1),
                    buildAssistantMessageFromAnswer(answer),
                  ],
                  lastActiveAt: Date.now(),
                },
              },
            };
          },
          false,
          'applyAnswer',
        );
      },

      // Drops the failed question/placeholder pair and parks the question text
      // on the session so it can be handed back to the input box whenever the
      // user is (or returns to) this conversation.
      failStream: (conversationId, pendingInput) => {
        if (!conversationId) return;
        set(
          (state) => {
            const previous = state.sessions[conversationId];
            if (!previous) return state;
            return {
              sessions: {
                ...state.sessions,
                [conversationId]: {
                  ...previous,
                  messages: previous.messages.slice(0, -2),
                  pendingInput,
                },
              },
            };
          },
          false,
          'failStream',
        );
      },

      consumePendingInput: (conversationId) => {
        if (!conversationId) return;
        set(
          (state) => {
            const previous = state.sessions[conversationId];
            if (!previous?.pendingInput) return state;
            return {
              sessions: {
                ...state.sessions,
                [conversationId]: { ...previous, pendingInput: undefined },
              },
            };
          },
          false,
          'consumePendingInput',
        );
      },

      removeMessageById: (conversationId, messageId) => {
        if (!conversationId) return;

        // Deleting the question that is currently being answered has to stop
        // that answer too: the stream lives at module level, so it would
        // otherwise keep producing tokens for a question that is gone. The
        // question and its assistant placeholder are appended as a pair (and
        // share an id), so the in-flight question sits second to last.
        const session = get().sessions[conversationId];
        const removingStreamedQuestion = Boolean(
          session?.isStreaming && session.messages.at(-2)?.id === messageId,
        );
        if (removingStreamedQuestion) {
          session?.abortController?.abort();
        }

        set(
          (state) => {
            const previous = state.sessions[conversationId];
            if (!previous) return state;
            return {
              sessions: {
                ...state.sessions,
                [conversationId]: {
                  ...previous,
                  messages: previous.messages.filter((x) => x.id !== messageId),
                  // Close the session right here instead of waiting for the
                  // reader's endStream, so the abort's trailing flush can't
                  // write the deleted answer back into the list.
                  isStreaming: removingStreamedQuestion
                    ? false
                    : previous.isStreaming,
                  abortController: removingStreamedQuestion
                    ? undefined
                    : previous.abortController,
                },
              },
            };
          },
          false,
          'removeMessageById',
        );
      },

      endStream: (conversationId) => {
        if (!conversationId) return;
        set(
          (state) => {
            const previous = state.sessions[conversationId];
            if (!previous) return state;
            return {
              sessions: {
                ...state.sessions,
                [conversationId]: {
                  ...previous,
                  isStreaming: false,
                  abortController: undefined,
                },
              },
            };
          },
          false,
          'endStream',
        );
      },

      stopStream: (conversationId) => {
        if (!conversationId) return;
        get().sessions[conversationId]?.abortController?.abort();
      },

      removeSessions: (conversationIds) => {
        if (conversationIds.length === 0) return;
        set(
          (state) => {
            const sessions = { ...state.sessions };
            conversationIds.forEach((id) => {
              sessions[id]?.abortController?.abort();
              delete sessions[id];
            });
            return { sessions };
          },
          false,
          'removeSessions',
        );
      },
    }),
    { name: 'chat-stream', enabled: EnableChatStreamDevtools },
  ),
);

// Shared empty array so a session-less selector keeps returning the same
// reference and doesn't trigger re-renders.
const EmptyMessages: IMessage[] = [];

export function useChatStreamMessages(conversationId: string) {
  return useChatStreamStore(
    (state) => state.sessions[conversationId]?.messages ?? EmptyMessages,
  );
}

export function useIsChatStreaming(conversationId: string) {
  return useChatStreamStore(
    (state) => state.sessions[conversationId]?.isStreaming ?? false,
  );
}

export function useChatPendingInput(conversationId: string) {
  return useChatStreamStore(
    (state) => state.sessions[conversationId]?.pendingInput,
  );
}
