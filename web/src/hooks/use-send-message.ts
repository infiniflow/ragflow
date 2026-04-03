import message from '@/components/ui/message';
import { Authorization } from '@/constants/authorization';
import { IReferenceObject } from '@/interfaces/database/chat';
import { BeginQuery } from '@/pages/agent/interface';
import api from '@/utils/api';
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

export const useSendMessageBySSE = (url: string = api.completeConversation) => {
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

        const res = response.clone().json();

        const reader = response?.body
          ?.pipeThrough(new TextDecoderStream())
          .pipeThrough(new EventSourceParserStream())
          .getReader();

        while (true) {
          try {
            const x = await reader?.read();
            if (x) {
              const { done, value } = x;
              if (done) {
                console.info('done');
                resetAnswerList();
                break;
              }
              try {
                const val = JSON.parse(value?.data || '');

                console.info('data:', val);
                if (val.code === 500) {
                  message.error(val.message);
                }

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
              break;
            }
          }
        }
        console.info('done?');
        setDone(true);
        resetAnswerList();
        return { data: await res, response };
      } catch (e) {
        setDone(true);
        resetAnswerList();

        console.warn(e);
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
