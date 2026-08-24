/**
 * Chat completion SSE streaming.
 *
 * NOTE on the network layering convention (see web/CLAUDE.md): this file
 * deliberately uses the native `fetch` API instead of the shared axios instance
 * in `@/utils/next-request`. Streaming requires access to the raw
 * `Response.body` `ReadableStream`, which axios cannot expose in the browser.
 * This mirrors the pre-existing SSE approach in `useSendMessageWithSse`
 * (`src/hooks/logic-hooks.ts`), but keeps `api.completionUrl` and the parsing
 * loop out of the hook layer.
 */
import { Authorization } from '@/constants/authorization';
import { ResponseType } from '@/interfaces/database/base';
import { IMessage, Variable } from '@/interfaces/database/chat';
import api from '@/utils/api';
import { getAuthorization } from '@/utils/authorization-util';
import { EventSourceParserStream } from 'eventsource-parser/stream';

export type ChatCompletionStreamParams = {
  chatId?: string;
  sessionId: string;
  messages: IMessage[];
  enableThinking?: string;
  enableInternet?: boolean;
  llmSetting?: Variable;
};

export type CompletionChunk = {
  answer?: string;
  final?: boolean;
  start_to_think?: boolean;
  end_to_think?: boolean;
  [key: string]: any;
};

function normalizeThinkingForRequest(thinking?: Variable['thinking']) {
  if (thinking === 'enabled' || thinking === 'disabled') return thinking;
  return undefined;
}

export function requestChatCompletionStream(
  {
    chatId,
    sessionId,
    messages,
    enableThinking,
    enableInternet,
    llmSetting,
  }: ChatCompletionStreamParams,
  signal: AbortSignal,
) {
  const {
    temperature,
    top_p,
    frequency_penalty,
    presence_penalty,
    max_tokens,
    thinking,
  } = llmSetting ?? {};
  const requestThinking = normalizeThinkingForRequest(thinking);

  return fetch(api.completionUrl, {
    method: 'POST',
    headers: {
      [Authorization]: getAuthorization(),
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      chat_id: chatId,
      session_id: sessionId,
      messages,
      pass_all_history_messages: true,
      reasoning: Number(enableThinking),
      internet: enableInternet,
      ...(temperature === undefined ? {} : { temperature }),
      ...(top_p === undefined ? {} : { top_p }),
      ...(frequency_penalty === undefined ? {} : { frequency_penalty }),
      ...(presence_penalty === undefined ? {} : { presence_penalty }),
      ...(max_tokens === undefined ? {} : { max_tokens }),
      ...(requestThinking === undefined ? {} : { thinking: requestThinking }),
    }),
    signal,
  });
}

/**
 * Yields each parsed SSE payload's `data` field. Terminates on stream end and
 * on any reader error; an AbortError is rethrown so the caller can distinguish
 * user cancellation from a genuine failure.
 */
export async function* parseCompletionEventStream(
  response: Response,
): AsyncGenerator<CompletionChunk> {
  const reader = response.body
    ?.pipeThrough(new TextDecoderStream())
    .pipeThrough(new EventSourceParserStream())
    .getReader();

  if (!reader) return;

  // oxlint-disable-next-line no-constant-condition
  while (true) {
    let chunk: Awaited<ReturnType<typeof reader.read>>;
    try {
      chunk = await reader.read();
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') {
        throw error;
      }
      // Any other reader failure means the stream is unusable. Unlike the
      // legacy loop in logic-hooks.ts, break out instead of spinning forever.
      break;
    }

    if (chunk.done) break;

    let payload: any;
    try {
      payload = JSON.parse(chunk.value?.data || '');
    } catch {
      // Swallow malformed SSE payloads, matching existing behaviour.
      continue;
    }

    const data = payload?.data;
    // The backend signals termination with a boolean `data` field.
    if (typeof data === 'boolean') continue;

    yield data as CompletionChunk;
  }
}

/** Reads the response body as JSON without throwing on a non-JSON body. */
export async function readJsonSafely(
  response: Response,
): Promise<ResponseType | undefined> {
  try {
    return await response.json();
  } catch {
    return undefined;
  }
}
