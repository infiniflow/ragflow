import message from '@/components/ui/message';
import { Authorization } from '@/constants/authorization';
import { ResponseType } from '@/interfaces/database/base';
import { IReferenceObject } from '@/interfaces/database/chat';
import { BeginQuery } from '@/pages/agent/interface';
import { getAuthorization } from '@/utils/authorization-util';
import { EventSourceParserStream } from 'eventsource-parser/stream';
import { useCallback, useRef, useState } from 'react';

export enum MessageEventType {
  WorkflowStarted = 'workflow_started',
  NodeStarted = 'node_started',
  NodeFinished = 'node_finished',
  Message = 'message',
  MessageEnd = 'message_end',
  WorkflowFinished = 'workflow_finished',
  UserInputs = 'user_inputs',
  WaitingForUser = 'waiting_for_user',
  NodeLogs = 'node_logs',
}

export interface IAnswerEvent<T> {
  event: MessageEventType;
  message_id: string;
  session_id: string;
  created_at: number;
  task_id: string;
  data: T;
}

export interface INodeData {
  inputs: Record<string, any>;
  outputs: Record<string, any>;
  component_id: string;
  component_name: string;
  component_type: string;
  error: null | string;
  elapsed_time: number;
  created_at: number;
  thoughts: string;
}

export interface IInputData {
  content: string;
  inputs: Record<string, BeginQuery>;
  tips: string;
}
export interface IAttachment {
  doc_id: string;
  format: string;
  file_name: string;
}
export interface IMessageData {
  content: string;
  audio_binary: string;
  outputs: any;
  start_to_think?: boolean;
  end_to_think?: boolean;
}

export interface IMessageEndData {
  reference: IReferenceObject;
}

export interface ILogData extends INodeData {
  logs: {
    name: string;
    result: string;
    args: {
      query: string;
      topic: string;
    };
  };
}

export type INodeEvent = IAnswerEvent<INodeData>;

export type IMessageEvent = IAnswerEvent<IMessageData>;

export type IMessageEndEvent = IAnswerEvent<IMessageEndData>;

export type IInputEvent = IAnswerEvent<IInputData>;

export type ILogEvent = IAnswerEvent<ILogData>;

export type IChatEvent = INodeEvent | IMessageEvent | IMessageEndEvent;

export type IEventList = Array<IChatEvent>;

const parseAgentEventData = (data: any) => {
  if (typeof data !== 'string') return data;

  try {
    return JSON.parse(data);
  } catch {
    return data;
  }
};

const normalizeAgentEvent = (value: any) => {
  if (value?.event === MessageEventType.WaitingForUser) {
    return {
      ...value,
      event: MessageEventType.UserInputs,
      data: parseAgentEventData(value.data),
    };
  }

  return value;
};

export const useSendMessageBySSE = (url: string) => {
  const [answerList, setAnswerList] = useState<IEventList>([]);
  const [done, setDone] = useState(true);
  const timer = useRef<any>();
  const sseRef = useRef<AbortController>();

  const initializeSseRef = useCallback(() => {
    sseRef.current = new AbortController();
  }, []);

  const resetAnswerList = useCallback(() => {
    if (timer.current) {
      clearTimeout(timer.current);
    }
    timer.current = setTimeout(() => {
      setAnswerList([]);
      clearTimeout(timer.current);
    }, 1000);
  }, []);

  const send = useCallback(
    async (
      body: any,
      controller?: AbortController,
    ): Promise<{ response: Response; data: ResponseType } | undefined> => {
      initializeSseRef();
      try {
        setDone(false);
        const response = await fetch(url, {
          method: 'POST',
          headers: {
            [Authorization]: getAuthorization(),
            'Content-Type': 'application/json',
          },
          body: JSON.stringify(body),
          signal: controller?.signal || sseRef.current?.signal,
        });
        // SSE streams (text/event-stream) emit `data: {...}\n\n` frames, not
        // a single JSON document. The .clone().json() call below is kept
        // for non-streaming callers (lastEventData will be set from the
        // per-frame parser below when the response IS SSE); for SSE
        // bodies the JSON parse rejects — swallow it silently instead
        // of console.warn'ing `SyntaxError: Unexpected token 'd', "data:
        // {"ev"... is not valid JSON` on every chat completion.
        const responseDataPromise: Promise<ResponseType | undefined> = response
          .clone()
          .json()
          .then((data: ResponseType) => data)
          .catch(() => undefined);
        if (!response.ok) {
          let errorMessage = response.statusText || 'Request failed';
          try {
            const errorBody = (await response
              .clone()
              .json()) as Partial<ResponseType>;
            if (typeof errorBody?.message === 'string' && errorBody.message) {
              errorMessage = errorBody.message;
            }
          } catch {
            // Non-JSON error body; fall back to the HTTP status text.
          }
          return {
            response,
            data: {
              code: response.status,
              data: null,
              message: errorMessage,
              status: response.status,
            },
          };
        }

        const reader = response?.body
          ?.pipeThrough(new TextDecoderStream())
          .pipeThrough(new EventSourceParserStream())
          .getReader();
        let lastEventData: ResponseType | undefined;

        try {
          // eslint-disable-next-line no-constant-condition
          while (true) {
            const x = await reader?.read();
            if (!x) {
              break;
            }
            const { done, value } = x;
            if (done) {
              console.log('agent chat sse reader done');
              break;
            }

            try {
              const raw = (value?.data ?? '').trim();
              // SSE end-of-stream sentinel — no payload, skip without
              // surfacing a JSON.parse error to the console.
              if (!raw) {
                continue;
              }
              // Some upstreams double-wrap the body in a `data:` prefix;
              // strip one layer so JSON.parse sees a real object.
              const payload = raw.startsWith('data:')
                ? raw.slice(5).trimStart()
                : raw;
              // Check the sentinel after prefix stripping so a
              // `data: [DONE]` payload is caught and the stream
              // loop is terminated.
              if (payload === '[DONE]') {
                console.log('agent chat sse done sentinel');
                break;
              }
              const val = normalizeAgentEvent(JSON.parse(payload));
              console.log('agent chat sse event', val);

              if (typeof val?.code === 'number' && val.code !== 0) {
                message.error(val.message);
              }
              lastEventData = val as ResponseType;

              setAnswerList((list) => {
                const nextList = [...list];
                nextList.push(val);
                return nextList;
              });
            } catch (e) {
              console.warn(e);
            }
          }
        } catch (e) {
          if (e instanceof DOMException && e.name === 'AbortError') {
            console.log('Request was aborted by user or logic.');
          } else {
            throw e;
          }
        }

        const responseData = await responseDataPromise;
        return {
          response,
          data:
            responseData ??
            lastEventData ??
            ({
              code: 0,
              data: true,
              message: 'success',
              status: response.status,
            } as ResponseType),
        };
      } catch (e) {
        console.warn(e);
        return undefined;
      } finally {
        setDone(true);
        resetAnswerList();
      }
    },
    [initializeSseRef, url, resetAnswerList],
  );

  const stopOutputMessage = useCallback(() => {
    sseRef.current?.abort();
  }, []);

  return {
    send,
    answerList,
    done,
    setDone,
    resetAnswerList,
    stopOutputMessage,
  };
};
