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

import { FilterValue } from '@/components/list-filter-bar/interface';
import { hasActiveFilter } from '@/components/list-filter-bar/utils';
import message from '@/components/ui/message';
import { Authorization } from '@/constants/authorization';
import { MessageType } from '@/constants/chat';
import { FormInstance } from '@/interfaces/antd-compat';
import { Pagination } from '@/interfaces/common';
import { ResponseType } from '@/interfaces/database/base';
import {
  IAnswer,
  IClientConversation,
  IMessage,
  Message,
} from '@/interfaces/database/chat';
import { IKnowledgeFile } from '@/interfaces/database/dataset';
import { changeLanguageAsync } from '@/locales/config';
import api from '@/utils/api';
import { getAuthorization } from '@/utils/authorization-util';
import { buildMessageUuid } from '@/utils/chat';
import {
  consumeListDeletionMarker,
  discardListDeletionMarker,
} from '@/utils/list-deletion-util';
import axios from 'axios';
import { EventSourceParserStream } from 'eventsource-parser/stream';
import { has, isEmpty, omit } from 'lodash';
import {
  ChangeEventHandler,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useTranslate } from './common-hooks';
import { useSetPaginationParams } from './route-hook';
import { useSaveSetting } from './use-user-setting-request';

export function usePrevious<T>(value: T) {
  const ref = useRef<T>();
  useEffect(() => {
    ref.current = value;
  }, [value]);
  return ref.current;
}

export const useSetSelectedRecord = <T = IKnowledgeFile>() => {
  const [currentRecord, setCurrentRecord] = useState<T>({} as T);

  const setRecord = (record: T) => {
    setCurrentRecord(record);
  };

  return { currentRecord, setRecord };
};

export const useChangeLanguage = () => {
  const { saveSetting } = useSaveSetting();

  const changeLanguage = useCallback(
    (lng: string) => {
      changeLanguageAsync(lng);
      saveSetting({ language: lng });
    },
    [saveSetting],
  );

  return changeLanguage;
};

export const useGetPaginationWithRouter = () => {
  const { t } = useTranslate('common');
  const {
    setPaginationParams,
    page,
    size: pageSize,
  } = useSetPaginationParams();

  const onPageChange: Pagination['onChange'] = useCallback(
    (pageNumber: number, size?: number) => {
      if (size !== pageSize) {
        setPaginationParams(1, size);
      } else {
        setPaginationParams(pageNumber, size);
      }
    },
    [setPaginationParams, pageSize],
  );

  const setCurrentPagination = useCallback(
    (pagination: { page: number; pageSize?: number }) => {
      if (pagination.pageSize !== pageSize) {
        pagination.page = 1; // Reset to first page if pageSize changes
      }
      setPaginationParams(pagination.page, pagination.pageSize);
    },
    [setPaginationParams, pageSize],
  );

  const pagination: Pagination = useMemo(() => {
    return {
      showQuickJumper: true,
      total: 0,
      showSizeChanger: true,
      current: page,
      pageSize: pageSize,
      pageSizeOptions: [1, 2, 10, 20, 50, 100],
      onChange: onPageChange,
      showTotal: (total: number) => `${t('total')} ${total}`,
    };
  }, [t, onPageChange, page, pageSize]);

  return {
    pagination,
    setPagination: setCurrentPagination,
  };
};

// When the current page becomes empty (e.g. after deleting the last card on
// the last page), navigate back to the previous page automatically. When the
// empty page was caused by a deletion (recorded via markListItemsDeleted) and
// a search or filter is active, clear them and jump to the first page of the
// unfiltered list instead — the filtered result set no longer exists, so the
// previous page of it would be meaningless.
export const useGoToPreviousPageOnEmpty = (
  listLength: number | undefined,
  loading: boolean = false,
  options?: {
    deletionKey?: string;
    searchString?: string;
    setSearchString?: (value: string) => void;
    filterValue?: FilterValue;
    setFilterValue?: (value: FilterValue) => void;
  },
) => {
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const optionsRef = useRef(options);
  optionsRef.current = options;

  useEffect(() => {
    if (loading || listLength !== 0 || pagination.current <= 1) {
      return;
    }

    const {
      deletionKey,
      searchString,
      setSearchString,
      filterValue,
      setFilterValue,
    } = optionsRef.current ?? {};
    const clearedByDeletion =
      deletionKey &&
      (Boolean(searchString) || hasActiveFilter(filterValue)) &&
      consumeListDeletionMarker(deletionKey);

    if (clearedByDeletion) {
      setSearchString?.('');
      setFilterValue?.({});
      setPagination({ page: 1, pageSize: pagination.pageSize });
    } else {
      if (deletionKey) {
        // The empty page was not caused by a deletion (e.g. a search with no
        // matches); drop any stale marker so it cannot fire later.
        discardListDeletionMarker(deletionKey);
      }
      setPagination({
        page: pagination.current - 1,
        pageSize: pagination.pageSize,
      });
    }
  }, [listLength, loading, pagination, setPagination]);
};

export const useHandleSearchChange = () => {
  const [searchString, setSearchString] = useState('');
  const { pagination, setPagination } = useGetPaginationWithRouter();
  const handleInputChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
      const value = e.target.value;
      setSearchString(value);
      setPagination({ page: 1 });
    },
    [setPagination],
  );

  return {
    handleInputChange,
    searchString,
    setSearchString,
    pagination,
    setPagination,
  };
};

export const useGetPagination = (options?: { pageSize?: number }) => {
  const [pagination, setPagination] = useState({
    page: 1,
    pageSize: options?.pageSize ?? 10,
  });
  const { t } = useTranslate('common');

  const onPageChange: Pagination['onChange'] = useCallback(
    (pageNumber: number, pageSize: number) => {
      setPagination({ page: pageNumber, pageSize });
    },
    [],
  );

  const currentPagination: Pagination = useMemo(() => {
    return {
      showQuickJumper: true,
      total: 0,
      showSizeChanger: true,
      current: pagination.page,
      pageSize: pagination.pageSize,
      pageSizeOptions: [1, 2, 10, 20, 50, 100],
      onChange: onPageChange,
      showTotal: (total: number) => `${t('total')} ${total}`,
    };
  }, [t, onPageChange, pagination]);

  return {
    pagination: currentPagination,
    setPagination,
  };
};

export interface AppConf {
  appName: string;
}

export const useFetchAppConf = () => {
  const [appConf, setAppConf] = useState<AppConf>({} as AppConf);
  const fetchAppConf = useCallback(async () => {
    const ret = await axios.get('/conf.json');

    setAppConf(ret.data);
  }, []);

  useEffect(() => {
    fetchAppConf();
  }, [fetchAppConf]);

  return appConf;
};

function useSetDoneRecord() {
  const [doneRecord, setDoneRecord] = useState<Record<string, boolean>>({});

  const clearDoneRecord = useCallback(() => {
    setDoneRecord({});
  }, []);

  const setDoneRecordById = useCallback((id: string, val: boolean) => {
    setDoneRecord((prev) => ({ ...prev, [id]: val }));
  }, []);

  const allDone = useMemo(() => {
    return Object.values(doneRecord).every((val) => val);
  }, [doneRecord]);

  useEffect(() => {
    if (!isEmpty(doneRecord) && allDone) {
      clearDoneRecord();
    }
  }, [allDone, clearDoneRecord, doneRecord]);

  return {
    doneRecord,
    setDoneRecord,
    setDoneRecordById,
    clearDoneRecord,
    allDone,
  };
}

export const useSendMessageWithSse = () => {
  const [answer, setAnswer] = useState<IAnswer>({} as IAnswer);
  const [done, setDone] = useState(true);
  const { doneRecord, clearDoneRecord, setDoneRecordById, allDone } =
    useSetDoneRecord();
  const timer = useRef<any>();
  const sseRef = useRef<AbortController>();

  const initializeSseRef = useCallback(() => {
    sseRef.current = new AbortController();
  }, []);

  const resetAnswer = useCallback(() => {
    if (timer.current) {
      clearTimeout(timer.current);
    }
    timer.current = setTimeout(() => {
      setAnswer({} as IAnswer);
      clearTimeout(timer.current);
    }, 1000);
  }, []);

  const setDoneValue = useCallback(
    (body: any, value: boolean) => {
      if (has(body, 'chatBoxId')) {
        setDoneRecordById(body.chatBoxId, value);
      } else {
        setDone(value);
      }
    },
    [setDoneRecordById],
  );

  const send = useCallback(
    async (
      url: string,
      body: any,
      controller?: AbortController,
    ): Promise<{ response: Response; data: ResponseType } | undefined> => {
      initializeSseRef();
      try {
        setDoneValue(body, false);
        const response = await fetch(url, {
          method: 'POST',
          headers: {
            [Authorization]: getAuthorization(),
            'Content-Type': 'application/json',
          },
          body: JSON.stringify(omit(body, 'chatBoxId')),
          signal: controller?.signal || sseRef.current?.signal,
        });

        const res = response.clone().json();

        const reader = response?.body
          ?.pipeThrough(new TextDecoderStream())
          .pipeThrough(new EventSourceParserStream())
          .getReader();

        // oxlint-disable-next-line no-constant-condition
        while (true) {
          try {
            const x = await reader?.read();
            if (x) {
              const { done, value } = x;
              if (done) {
                resetAnswer();
                break;
              }
              try {
                const val = JSON.parse(value?.data || '');
                const d = val?.data;
                if (typeof d !== 'boolean') {
                  setAnswer((prev) => {
                    const prevAnswer = prev.answer || '';
                    // Skip final-chunk answer only when prior stream chunks exist (avoids duplicate).
                    // Empty-response and other single-shot answers arrive with final=true only.
                    // const currentAnswer = d.final ? '' : d.answer || '';
                    const currentAnswer =
                      d.final && prevAnswer ? '' : d.answer || '';

                    let newAnswer: string;
                    if (prevAnswer && currentAnswer.startsWith(prevAnswer)) {
                      newAnswer = currentAnswer;
                    } else {
                      newAnswer = prevAnswer + currentAnswer;
                    }

                    if (d.start_to_think === true) {
                      newAnswer = newAnswer + '<think>';
                    }

                    if (d.end_to_think === true) {
                      newAnswer = newAnswer + '</think>';
                    }

                    return {
                      ...d,
                      answer: newAnswer,
                      conversationId: body?.session_id ?? body?.conversation_id,
                      chatBoxId: body.chatBoxId,
                    };
                  });
                }
              } catch {
                // Swallow parse errors silently
              }
            }
          } catch (error) {
            if (error instanceof DOMException && error.name === 'AbortError') {
              break;
            }
          }
        }
        setDoneValue(body, true);
        resetAnswer();
        return { data: await res, response };
      } catch {
        setDoneValue(body, true);

        resetAnswer();
        // Swallow fetch errors silently
      }
    },
    [initializeSseRef, setDoneValue, resetAnswer],
  );

  const stopOutputMessage = useCallback(() => {
    sseRef.current?.abort();
  }, []);

  return {
    send,
    answer,
    done,
    doneRecord,
    allDone,
    setDone,
    resetAnswer,
    stopOutputMessage,
    clearDoneRecord,
  };
};

export const useSpeechWithSse = (url: string = api.chatsTts) => {
  const read = useCallback(
    async (body: any) => {
      const response = await fetch(url, {
        method: 'POST',
        headers: {
          [Authorization]: getAuthorization(),
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(body),
      });
      try {
        const res = await response.clone().json();
        if (res?.code !== 0) {
          message.error(res?.message);
        }
      } catch {
        // Swallow errors silently
      }
      return response;
    },
    [url],
  );

  return { read };
};

//#region chat hooks

// Firefox reports a fractional `scrollTop`, while `scrollHeight` / `clientHeight`
// are rounded. `scrollToBottom` therefore lands a sub-pixel *below* the position
// a native clamp had produced, which reads as a decreasing `scrollTop`. Require a
// real gesture's worth of movement so that jitter is not mistaken for one.
const UserScrollUpThreshold = 2;

export const useScrollToBottom = (
  messages?: unknown,
  containerRef?: React.RefObject<HTMLDivElement>,
) => {
  const ref = useRef<HTMLDivElement>(null);
  const [isAtBottom, setIsAtBottom] = useState(true);
  const isAtBottomRef = useRef(true);
  // `null` means "no baseline yet", so the very first measurement has to fall
  // back to the distance check instead of guessing a scroll direction.
  const lastScrollTopRef = useRef<number | null>(null);

  useEffect(() => {
    isAtBottomRef.current = isAtBottom;
  }, [isAtBottom]);

  // We pin the transcript to the bottom ourselves, so browser scroll anchoring is
  // pure interference: when a streamed answer re-lays out (markdown turning a
  // paragraph into a code block, a line re-wrapping), Firefox shifts `scrollTop`
  // to hold its anchor node still. That shift is indistinguishable from a user
  // scrolling up in the handler below, so it latched auto-follow off mid-answer.
  // Chrome suppresses the adjustment while pinned to the bottom, which is why
  // only Firefox drifted away from the bottom.
  useEffect(() => {
    if (!containerRef?.current) return;
    containerRef.current.style.overflowAnchor = 'none';
  }, [containerRef]);

  const checkIfNearBottom = useCallback(() => {
    if (!containerRef?.current) return true;
    const { scrollTop, scrollHeight, clientHeight } = containerRef.current;
    // Content shorter than the viewport has nothing to scroll, so it is
    // trivially "at the bottom". Returning false here would latch auto-follow
    // off when a stream starts from a short transcript: growing content does
    // not fire a `scroll` event, so no later check would ever re-arm the flag
    // and the view would never track the incoming message.
    if (scrollHeight <= clientHeight) return true;
    return Math.abs(scrollTop + clientHeight - scrollHeight) < 60;
  }, [containerRef]);

  useEffect(() => {
    if (!containerRef?.current) return;
    const container = containerRef.current;

    const handleScroll = () => {
      const previousScrollTop = lastScrollTopRef.current;
      const { scrollTop } = container;
      lastScrollTopRef.current = scrollTop;

      const nearBottom = checkIfNearBottom();
      let atBottom: boolean;
      if (nearBottom) {
        atBottom = true;
      } else if (
        previousScrollTop === null ||
        scrollTop < previousScrollTop - UserScrollUpThreshold
      ) {
        // With scroll anchoring off, only a user gesture (wheel, drag, keys,
        // touch) can shrink `scrollTop` by a meaningful amount, so this is the
        // one reliable signal that they want to leave the bottom.
        atBottom = false;
      } else {
        // We are far from the bottom yet `scrollTop` did not move: the gap comes
        // from content that grew after `scrollToBottom` ran but before this
        // event was dispatched. Disarming here would strand auto-follow forever,
        // because growing content never fires another `scroll` event to re-arm
        // it. Keep whatever the user last asked for.
        atBottom = isAtBottomRef.current;
      }

      // Write the ref here rather than relying on the effect that mirrors
      // `isAtBottom`: that effect only runs after the next render, and while the
      // main thread is busy rendering a streaming answer an already scheduled
      // auto-scroll would still see the stale `true`.
      isAtBottomRef.current = atBottom;
      setIsAtBottom(atBottom);
    };

    container.addEventListener('scroll', handleScroll);
    handleScroll();
    return () => container.removeEventListener('scroll', handleScroll);
  }, [containerRef, checkIfNearBottom]);

  // Imperative scroll function
  const scrollToBottom = useCallback(() => {
    if (containerRef?.current) {
      const container = containerRef.current;
      // Overshoot and let the browser clamp. `scrollHeight - clientHeight` is a
      // difference of two rounded values, so in Firefox — where the real maximum
      // is fractional — it can land just short of the bottom.
      container.scrollTo({
        top: container.scrollHeight,
        behavior: 'auto',
      });
    }
  }, [containerRef]);

  // Streaming replaces `messages` many times a second. The previous
  // rAF + setTimeout(100) chain always had several scrolls queued, and they read
  // `isAtBottomRef` long after the user had scrolled up — yanking the view back
  // down. One cancellable frame per change, gated on the latest position, keeps
  // auto-follow without fighting the user.
  useEffect(() => {
    if (!messages) return;
    if (!containerRef?.current) return;
    if (!isAtBottomRef.current) return;

    const frame = requestAnimationFrame(() => {
      if (isAtBottomRef.current) {
        scrollToBottom();
      }
    });
    return () => cancelAnimationFrame(frame);
  }, [messages, containerRef, scrollToBottom]);

  return { scrollRef: ref, isAtBottom, scrollToBottom };
};

export const useHandleMessageInputChange = () => {
  const [value, setValue] = useState('');

  const handleInputChange: ChangeEventHandler<HTMLTextAreaElement> = (e) => {
    const value = e.target.value;
    const nextValue = value.replaceAll('\\n', '\n').replaceAll('\\t', '\t');
    setValue(nextValue);
  };

  return {
    handleInputChange,
    value,
    setValue,
  };
};

export const useSelectDerivedMessages = () => {
  const [derivedMessages, setDerivedMessages] = useState<IMessage[]>([]);

  const messageContainerRef = useRef<HTMLDivElement>(null);

  const { scrollRef, scrollToBottom } = useScrollToBottom(
    derivedMessages,
    messageContainerRef,
  );

  const addNewestQuestion = useCallback(
    (message: IMessage, answer: string = '') => {
      setDerivedMessages((pre) => {
        return [
          ...pre,
          {
            ...message,
            id: buildMessageUuid(message), // The message id is generated on the front end,
            // and the message id returned by the back end is the same as the question id,
            //  so that the pair of messages can be deleted together when deleting the message
          },
          {
            role: MessageType.Assistant,
            content: answer,
            conversationId: message.conversationId,
            id: buildMessageUuid({ ...message, role: MessageType.Assistant }),
          },
        ];
      });
    },
    [],
  );

  const addNewestOneQuestion = useCallback((message: Message) => {
    setDerivedMessages((pre) => {
      return [
        ...pre,
        {
          ...message,
          id: buildMessageUuid(message), // The message id is generated on the front end,
          // and the message id returned by the back end is the same as the question id,
          //  so that the pair of messages can be deleted together when deleting the message
        },
      ];
    });
  }, []);

  // Add the streaming message to the last item in the message list
  const addNewestAnswer = useCallback((answer: IAnswer) => {
    setDerivedMessages((pre) => {
      return [
        ...(pre?.slice(0, -1) ?? []),
        {
          role: MessageType.Assistant,
          content: answer.answer,
          reference: answer.reference,
          id: buildMessageUuid({
            id: answer.id,
            role: MessageType.Assistant,
          }),
          prompt: answer.prompt,
          audio_binary: answer.audio_binary,
          ...omit(answer, 'reference'),
        },
      ];
    });
  }, []);

  // Add the streaming message to the last item in the message list
  const addNewestOneAnswer = useCallback((answer: IAnswer) => {
    setDerivedMessages((pre) => {
      const idx = pre.findIndex((x) => x.id === answer.id);

      if (idx !== -1) {
        return pre.map((x) => {
          if (x.id === answer.id) {
            return { ...x, ...answer, content: answer.answer };
          }
          return x;
        });
      }

      return [
        ...(pre ?? []),
        {
          role: MessageType.Assistant,
          content: answer.answer,
          reference: answer.reference,
          id: buildMessageUuid({
            id: answer.id,
            role: MessageType.Assistant,
          }),
          prompt: answer.prompt,
          audio_binary: answer.audio_binary,
          ...omit(answer, 'reference'),
        },
      ];
    });
  }, []);

  const addPrologue = useCallback((prologue: string) => {
    setDerivedMessages((pre) => {
      if (pre.length > 0) {
        return [
          {
            ...pre[0],
            content: prologue,
          },
          ...pre.slice(1),
        ];
      }

      return [
        {
          role: MessageType.Assistant,
          content: prologue,
          id: buildMessageUuid({
            role: MessageType.Assistant,
          }),
        },
      ];
    });
  }, []);

  const removeLatestMessage = useCallback(() => {
    setDerivedMessages((pre) => {
      const nextMessages = pre?.slice(0, -2) ?? [];
      return nextMessages;
    });
  }, []);

  const removeMessageById = useCallback(
    (messageId: string) => {
      setDerivedMessages((pre) => {
        const nextMessages = pre?.filter((x) => x.id !== messageId) ?? [];
        return nextMessages;
      });
    },
    [setDerivedMessages],
  );

  const removeMessagesAfterCurrentMessage = useCallback(
    (messageId: string) => {
      setDerivedMessages((pre) => {
        const index = pre.findIndex((x) => x.id === messageId);
        if (index !== -1) {
          let nextMessages = pre.slice(0, index + 2) ?? [];
          const latestMessage = nextMessages.at(-1);
          nextMessages = latestMessage
            ? [
                ...nextMessages.slice(0, -1),
                {
                  ...latestMessage,
                  content: '',
                  reference: undefined,
                  prompt: undefined,
                },
              ]
            : nextMessages;
          return nextMessages;
        }
        return pre;
      });
    },
    [setDerivedMessages],
  );

  const removeAllMessages = useCallback(() => {
    setDerivedMessages([]);
  }, [setDerivedMessages]);

  const removeAllMessagesExceptFirst = useCallback(() => {
    setDerivedMessages((list) => {
      if (list.length <= 1) {
        return list;
      }
      return list.slice(0, 1);
    });
  }, [setDerivedMessages]);

  return {
    scrollRef,
    messageContainerRef,
    derivedMessages,
    setDerivedMessages,
    addNewestQuestion,
    addNewestAnswer,
    removeLatestMessage,
    removeMessageById,
    addNewestOneQuestion,
    addNewestOneAnswer,
    removeMessagesAfterCurrentMessage,
    removeAllMessages,
    scrollToBottom,
    removeAllMessagesExceptFirst,
    addPrologue,
  };
};

export interface IRemoveMessageById {
  removeMessageById(messageId: string): void;
}

export const useRemoveMessagesAfterCurrentMessage = (
  setCurrentConversation: (
    callback: (state: IClientConversation) => IClientConversation,
  ) => void,
) => {
  const removeMessagesAfterCurrentMessage = useCallback(
    (messageId: string) => {
      setCurrentConversation((pre) => {
        const index = pre.message?.findIndex((x) => x.id === messageId);
        if (index !== -1) {
          let nextMessages = pre.message?.slice(0, index + 2) ?? [];
          const latestMessage = nextMessages.at(-1);
          nextMessages = latestMessage
            ? [
                ...nextMessages.slice(0, -1),
                {
                  ...latestMessage,
                  content: '',
                  reference: undefined,
                  prompt: undefined,
                },
              ]
            : nextMessages;
          return {
            ...pre,
            message: nextMessages,
          };
        }
        return pre;
      });
    },
    [setCurrentConversation],
  );

  return { removeMessagesAfterCurrentMessage };
};

export interface IRegenerateMessage {
  regenerateMessage?: (message: Message) => void;
}

export const useRegenerateMessage = ({
  removeMessagesAfterCurrentMessage,
  sendMessage,
  messages,
}: {
  removeMessagesAfterCurrentMessage(messageId: string): void;
  sendMessage({
    message,
  }: {
    message: Message;
    messages?: Message[];
  }): void | Promise<any>;
  messages: Message[];
}) => {
  const regenerateMessage = useCallback(
    async (message: Message) => {
      if (message.id) {
        removeMessagesAfterCurrentMessage(message.id);
        const index = messages.findIndex((x) => x.id === message.id);
        // Always pass the truncated history explicitly, even when it is
        // empty (regenerating the first question), so the backend can
        // overwrite the session with it via pass_all_history_messages.
        const nextMessages = index !== -1 ? messages.slice(0, index) : [];
        sendMessage({
          // Keep the original id so the question/answer pair id stays
          // consistent between local state and the persisted session.
          message: { ...message },
          messages: nextMessages,
        });
      }
    },
    [removeMessagesAfterCurrentMessage, sendMessage, messages],
  );

  return { regenerateMessage };
};

// #endregion

/**
 *
 * @param defaultId
 * used to switch between different items, similar to radio
 * @returns
 */
export const useSelectItem = (defaultId?: string) => {
  const [selectedId, setSelectedId] = useState('');

  const handleItemClick = useCallback(
    (id: string) => () => {
      setSelectedId(id);
    },
    [],
  );

  useEffect(() => {
    if (defaultId) {
      setSelectedId(defaultId);
    }
  }, [defaultId]);

  return { selectedId, handleItemClick };
};

const ChunkTokenNumMap = {
  naive: 128,
  knowledge_graph: 8192,
};

export const useHandleChunkMethodSelectChange = (form: FormInstance) => {
  // const form = Form.useFormInstance();
  const handleChange = useCallback(
    (value: string) => {
      if (value in ChunkTokenNumMap) {
        form.setFieldValue(
          ['parser_config', 'chunk_token_num'],
          ChunkTokenNumMap[value as keyof typeof ChunkTokenNumMap],
        );
      }
    },
    [form],
  );

  return handleChange;
};

// reset form fields when modal is form, closed
export const useResetFormOnCloseModal = ({
  form,
  visible,
}: {
  form: FormInstance;
  visible?: boolean;
}) => {
  const prevOpenRef = useRef<boolean>();
  useEffect(() => {
    prevOpenRef.current = visible;
  }, [visible]);
  const prevOpen = prevOpenRef.current;

  useEffect(() => {
    if (!visible && prevOpen) {
      form.resetFields();
    }
  }, [form, prevOpen, visible]);
};
