import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader } from '@/components/ui/card';
import {
  useFetchSessionList,
  useFetchSessionManually,
  useGetChatSearchParams,
} from '@/hooks/use-chat-request';
import { IClientConversation } from '@/interfaces/database/chat';
import { RootLayoutContainer } from '@/layouts/root-layout';
import { cn } from '@/lib/utils';
import { isEmpty } from 'lodash';
import { LucideArrowBigLeft, LucideArrowUpRight } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import { useHandleClickConversationCard } from '../hooks/use-click-card';
import { ChatSettings } from './app-settings/chat-settings';
import { MultipleChatBox } from './chat-box/next-multiple-chat-box';
import { SingleChatBox } from './chat-box/single-chat-box';
import { ConversationHeader } from './conversation-header';
import { Sessions } from './sessions';
import { useAddChatBox } from './use-add-box';
import { useSwitchDebugMode } from './use-switch-debug-mode';

export default function Chat() {
  const { t } = useTranslation();
  const [currentConversation, setCurrentConversation] =
    useState<IClientConversation>({} as IClientConversation);

  const { fetchSessionManually } = useFetchSessionManually();

  const { handleConversationCardClick, controller, stopOutputMessage } =
    useHandleClickConversationCard();

  const { isDebugMode, switchDebugMode } = useSwitchDebugMode();
  const { removeChatBox, addChatBox, chatBoxIds, hasSingleChatBox } =
    useAddChatBox(isDebugMode);

  const { conversationId, isNew } = useGetChatSearchParams();
  const { id: chatId } = useParams();

  const { data: dialogList } = useFetchSessionList();

  // Lifted out of `Sessions` so the header can mirror it: while the conversation
  // list is open the header drops its title (the list already highlights the
  // active conversation), and it reappears once the list is collapsed.
  const [sessionsVisible, setSessionsVisible] = useState(true);

  const handleExpandSessions = useCallback(() => {
    setSessionsVisible(true);
  }, []);

  const currentConversationName = useMemo(() => {
    return (
      dialogList.find((x) => x.id === conversationId)?.name ||
      t('chat.newConversation')
    );
  }, [conversationId, dialogList, t]);

  // The URL is the single source of truth for which conversation is open:
  // card clicks, "+" and the temp→real id swap after the first send all land
  // here. Clear first so the previous conversation's messages and references
  // can never leak into the newly opened one while the fetch is in flight.
  useEffect(() => {
    setCurrentConversation((previous) =>
      isEmpty(previous) ? previous : ({} as IClientConversation),
    );
    if (!conversationId || isNew === 'true') return;

    let cancelled = false;
    fetchSessionManually(conversationId).then((conversation) => {
      if (!cancelled && !isEmpty(conversation)) {
        setCurrentConversation(conversation);
      }
    });
    return () => {
      cancelled = true;
    };
  }, [conversationId, isNew, fetchSessionManually]);

  if (isDebugMode) {
    return (
      <section
        className="pt-5 pb-14 h-[100vh] flex flex-col"
        data-testid="chat-detail-multimodel-root"
      >
        <header className="px-10 pb-5">
          <div className="mb-5">
            <Button
              variant="outline"
              onClick={switchDebugMode}
              data-testid="chat-detail-multimodel-back"
            >
              <LucideArrowBigLeft />
              <span>{t('common.back')}</span>
            </Button>
          </div>

          <span className="text-2xl">
            {t('chat.multipleModels')} ({chatBoxIds.length}/3)
          </span>
        </header>

        <MultipleChatBox
          chatBoxIds={chatBoxIds}
          controller={controller}
          removeChatBox={removeChatBox}
          addChatBox={addChatBox}
          stopOutputMessage={stopOutputMessage}
          conversation={currentConversation}
        ></MultipleChatBox>
      </section>
    );
  }

  return (
    <RootLayoutContainer>
      <section className="h-full flex flex-col" data-testid="chat-detail">
        <article className="flex flex-1 min-h-0 pb-9">
          <Sessions
            handleConversationCardClick={handleConversationCardClick}
            visible={sessionsVisible}
            onVisibleChange={setSessionsVisible}
          ></Sessions>

          <Card className="flex-1 min-w-0 bg-transparent border-none shadow-none h-full">
            <CardContent className="flex p-0 h-full">
              <Card className="flex flex-col flex-1 bg-transparent min-w-0 overflow-hidden">
                {/* Rendered only while the conversation list is collapsed: an
                    expanded list already names the active conversation, and an
                    empty header bar would still cost its own height. */}
                {!sessionsVisible && (
                  <CardHeader
                    className={cn('px-5 py-3', {
                      'border-b-0.5 border-cable-border': hasSingleChatBox,
                    })}
                  >
                    <ConversationHeader
                      chatId={chatId}
                      sessionId={conversationId}
                      title={currentConversationName}
                      summarizable={isNew !== 'true' && !isEmpty(conversationId)}
                      onExpandSessions={handleExpandSessions}
                    >
                      <Button
                        variant="ghost"
                        className="h-8 shrink-0 gap-1.5 rounded-lg px-2 text-cable-brand hover:bg-cable-brand-soft"
                        onClick={switchDebugMode}
                        data-testid="chat-detail-multimodel-toggle"
                      >
                        <LucideArrowUpRight className="size-4" />
                        {t('chat.multipleModels')}
                      </Button>
                    </ConversationHeader>
                  </CardHeader>
                )}
                <CardContent className="flex-1 p-0 min-h-0">
                  <SingleChatBox conversation={currentConversation} />
                </CardContent>
              </Card>

              <ChatSettings hasSingleChatBox={hasSingleChatBox}></ChatSettings>
            </CardContent>
          </Card>
        </article>
      </section>
    </RootLayoutContainer>
  );
}
