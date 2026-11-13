import EmbedDialog from '@/components/embed-dialog';
import { useShowEmbedModal } from '@/components/embed-dialog/use-show-embed-dialog';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { SharedFrom } from '@/constants/chat';
import { useSetModalState } from '@/hooks/common-hooks';
import {
  useFetchSessionManually,
  useGetChatSearchParams,
} from '@/hooks/use-chat-request';
import { IClientConversation } from '@/interfaces/database/chat';
import { RootLayoutContainer } from '@/layouts/root-layout';
import { isEmpty } from 'lodash';
import { LucideSend } from 'lucide-react';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router';
import { useHandleClickConversationCard } from '../hooks/use-click-card';
import { ChatSettings } from './app-settings/chat-settings';
import { MultipleChatBox } from './chat-box/next-multiple-chat-box';
import { SingleChatBox } from './chat-box/single-chat-box';
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

  const { isDebugMode, enterDebugMode, exitDebugMode } = useSwitchDebugMode();
  const { removeChatBox, addChatBox, chatBoxIds } = useAddChatBox(isDebugMode);
  const {
    visible: settingsVisible,
    showModal: showSettings,
    hideModal: hideSettings,
  } = useSetModalState();

  const { conversationId, isNew } = useGetChatSearchParams();
  const { id } = useParams();
  const { showEmbedModal, hideEmbedModal, embedVisible, beta } =
    useShowEmbedModal();

  const handleOpenSettings = useCallback(() => {
    exitDebugMode();
    showSettings();
  }, [exitDebugMode, showSettings]);

  const handleOpenMultipleModels = useCallback(() => {
    hideSettings();
    enterDebugMode();
  }, [enterDebugMode, hideSettings]);

  const handleShowSingleChat = useCallback(() => {
    hideSettings();
    exitDebugMode();
  }, [exitDebugMode, hideSettings]);

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

  return (
    <RootLayoutContainer>
      <section className="h-full flex flex-col" data-testid="chat-detail">
        <article className="flex flex-1 min-h-0 pb-9">
          <Sessions
            handleConversationCardClick={handleConversationCardClick}
            settingsActive={settingsVisible}
            multipleModelsActive={isDebugMode}
            onOpenSettings={handleOpenSettings}
            onOpenMultipleModels={handleOpenMultipleModels}
            onShowSingleChat={handleShowSingleChat}
          ></Sessions>

          <section className="flex-1 min-w-0 min-h-0 flex flex-col">
            <header className="p-5 flex justify-end">
              <Button
                variant="ghost"
                onClick={showEmbedModal}
                data-testid="chat-detail-embed-open"
              >
                <LucideSend />
                {t('common.embedIntoSite')}
              </Button>
            </header>

            <Card
              className="flex flex-1 min-w-0 min-h-0 flex-col overflow-hidden bg-transparent mr-4"
              data-testid="chat-detail-content-frame"
            >
              {isDebugMode ? (
                <section
                  className="pt-5 pb-14 h-full flex flex-col"
                  data-testid="chat-detail-multimodel-root"
                >
                  <header className="px-10 pb-5">
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
              ) : settingsVisible ? (
                <ChatSettings />
              ) : (
                <CardContent className="flex-1 p-0 min-h-0">
                  <SingleChatBox conversation={currentConversation} />
                </CardContent>
              )}
            </Card>
          </section>
        </article>

        <EmbedDialog
          visible={embedVisible}
          hideModal={hideEmbedModal}
          token={id!}
          from={SharedFrom.Chat}
          beta={beta}
          isAgent={false}
        />
      </section>
    </RootLayoutContainer>
  );
}
