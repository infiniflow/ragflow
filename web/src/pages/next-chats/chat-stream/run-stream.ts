/**
 * Module-level SSE driver. Deliberately not a hook: it must keep running (and
 * keep writing into the store) after the chat page unmounts, so that navigating
 * away mid-answer and back shows the stream still in progress.
 */
import { IMessage, Variable } from '@/interfaces/database/chat';
import {
  CompletionChunk,
  parseCompletionEventStream,
  readJsonSafely,
  requestChatCompletionStream,
} from '@/services/chat-completion-stream';
import { AnswerFlushIntervalMs } from './constants';
import { useChatStreamStore } from './store';
import { mergeAnswerChunk } from './utils';

export type RunChatCompletionStreamParams = {
  conversationId: string;
  chatId?: string;
  messages: IMessage[];
  enableThinking?: string;
  enableInternet?: boolean;
  llmSetting?: Variable;
};

export type RunChatCompletionStreamResult = {
  ok: boolean;
  aborted: boolean;
};

export async function runChatCompletionStream({
  conversationId,
  chatId,
  messages,
  enableThinking,
  enableInternet,
  llmSetting,
}: RunChatCompletionStreamParams): Promise<RunChatCompletionStreamResult> {
  const { beginStream, applyAnswer, endStream } = useChatStreamStore.getState();

  const controller = new AbortController();
  if (!beginStream(conversationId, controller)) {
    return { ok: false, aborted: false };
  }

  // Kept local rather than read back from the store: it must stay immune to
  // concurrent mutations of the message list, and mergeAnswerChunk's
  // `startsWith` / think-marker behaviour depends on the raw accumulated text
  // rather than on the rendered message content.
  let accumulatedAnswer = '';
  let aborted = false;
  let ok = true;

  // Chunks arrive faster than the UI can use them, so they're coalesced into a
  // single store write per AnswerFlushIntervalMs. Fields are merged rather than
  // replaced so a `reference` or `prompt` that arrived on a skipped chunk isn't
  // lost; the buffer resets on each flush, matching the legacy "newest chunk
  // wins" semantics across flush windows.
  let pendingChunk: CompletionChunk | undefined;
  let lastFlushAt = 0;

  const flushAnswer = () => {
    if (!pendingChunk) return;
    applyAnswer(conversationId, {
      ...pendingChunk,
      answer: accumulatedAnswer,
      conversationId,
    });
    pendingChunk = undefined;
    lastFlushAt = Date.now();
  };

  try {
    const response = await requestChatCompletionStream(
      {
        chatId,
        sessionId: conversationId,
        messages,
        enableThinking,
        enableInternet,
        llmSetting,
      },
      controller.signal,
    );

    // Clone before the body is consumed: an error response carries a JSON
    // body, which is how a failed completion is detected. On a successful SSE
    // response this parse simply fails and yields undefined.
    const errorBodyPromise = readJsonSafely(response.clone());

    try {
      for await (const chunk of parseCompletionEventStream(response)) {
        accumulatedAnswer = mergeAnswerChunk(accumulatedAnswer, chunk);
        pendingChunk = pendingChunk ? { ...pendingChunk, ...chunk } : chunk;

        // The first chunk flushes immediately (lastFlushAt is 0), so the empty
        // placeholder is replaced without waiting out an interval.
        if (chunk.final || Date.now() - lastFlushAt >= AnswerFlushIntervalMs) {
          flushAnswer();
        }
      }
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') {
        aborted = true;
      } else {
        throw error;
      }
    }

    // A healthy stream body isn't valid JSON, so errorBody stays undefined and
    // the request counts as successful. A parseable body means the endpoint
    // answered with an error envelope instead of a stream.
    const errorBody = await errorBodyPromise;
    if (
      !aborted &&
      errorBody !== undefined &&
      (response.status !== 200 || errorBody.code !== 0)
    ) {
      ok = false;
    }
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') {
      aborted = true;
    } else {
      ok = false;
    }
  } finally {
    // Whatever was buffered must land, including on abort: a stopped answer
    // keeps the text produced up to that point.
    flushAnswer();
    endStream(conversationId);
  }

  return { ok, aborted };
}
