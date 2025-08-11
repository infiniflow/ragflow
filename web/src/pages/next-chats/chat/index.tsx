import { PageHeader } from '@/components/page-header';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { useSetModalState } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useFetchDialog } from '@/hooks/use-chat-request';
import { useTranslation } from 'react-i18next';
import { useHandleClickConversationCard } from '../hooks/use-click-card';
import { ChatSettings } from './app-settings/chat-settings';
import { ChatBox } from './chat-box';
import { Sessions } from './sessions';

export default function Chat() {
  const { navigateToChatList } = useNavigatePage();
  const { data } = useFetchDialog();
  const { t } = useTranslation();
  const { handleConversationCardClick, controller } =
    useHandleClickConversationCard();
  const { visible: settingVisible, switchVisible: switchSettingVisible } =
    useSetModalState(true);

  return (
    <section className="h-full flex flex-col">
      <PageHeader>
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink onClick={navigateToChatList}>
                {t('chat.chat')}
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{data.name}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      </PageHeader>
      <div className="flex flex-1 min-h-0">
        <div className="flex flex-1 min-w-0">
          <Sessions
            handleConversationCardClick={handleConversationCardClick}
            switchSettingVisible={switchSettingVisible}
          ></Sessions>
          <ChatBox controller={controller}></ChatBox>
        </div>
        {settingVisible && (
          <ChatSettings
            switchSettingVisible={switchSettingVisible}
          ></ChatSettings>
        )}
      </div>
    </section>
  );
}
