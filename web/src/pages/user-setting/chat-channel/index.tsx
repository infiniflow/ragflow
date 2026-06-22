import { Button } from '@/components/ui/button';
import { Plus } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { ProfileSettingWrapperCard } from '../components/user-setting-header';
import AddChatChannelModal from './add-channel-modal';
import { AddedChannelCard } from './component/added-channel-card';
import { ChatChannelKey, useChatChannelInfo } from './constant';
import { useAddChatChannel, useListChatChannel } from './hooks';
import { IChatChannelInfo } from './interface';

const AvailableChannelCard = ({
  name,
  description,
  icon,
  onAdd,
}: IChatChannelInfo & { onAdd: () => void }) => {
  const { t } = useTranslation();

  return (
    <article
      className="
        size-full p-2.5 border-0.5 border-border-button rounded-lg relative group hover:bg-bg-card focus-within:bg-bg-card
        grid grid-cols-[auto_1fr] grid-rows-[auto_1fr] gap-x-2.5 gap-y-1"
      style={{ gridTemplateAreas: '"icon title" "icon description"' }}
      onClick={() => onAdd()}
    >
      <span className="w-6" style={{ gridArea: 'icon' }}>
        {icon}
      </span>

      <header className="flex items-center gap-2" style={{ gridArea: 'title' }}>
        <h3 className="text-base text-text-primary">{name}</h3>
        <Button
          size="auto"
          className="ml-auto px-1 py-0.5 gap-0.5 text-xs items-center opacity-0 transition-all group-hover:opacity-100 group-focus-within:opacity-100"
          onClick={(e: any) => {
            e.stopPropagation();
            onAdd();
          }}
        >
          <Plus className="size-[1em]" />
          {t('setting.add')}
        </Button>
      </header>

      <p
        style={{ gridArea: 'description' }}
        className="text-xs text-text-secondary"
      >
        {description}
      </p>
    </article>
  );
};

const ChatChannel = () => {
  const { t } = useTranslation();
  const { chatChannelInfo } = useChatChannelInfo();
  const channelTemplates: IChatChannelInfo[] = Object.values(ChatChannelKey)
    .filter(
      (id) =>
        [
          ChatChannelKey.DISCORD,
          ChatChannelKey.DINGTALK,
          ChatChannelKey.FEISHU,
          ChatChannelKey.TELEGRAM,
          ChatChannelKey.QQBOT,
          ChatChannelKey.WECOM,
        ].includes(id), // Show only selected chat channels
    )
    .map((id) => ({
      id,
      name: chatChannelInfo[id].name,
      description: chatChannelInfo[id].description,
      icon: chatChannelInfo[id].icon,
    }));

  const { categorizedList } = useListChatChannel();

  const {
    activeChannel,
    editingRecord,
    loading,
    modalVisible,
    hideModal,
    showAddingModal,
    showEditingModal,
    handleOk,
  } = useAddChatChannel();

  return (
    <ProfileSettingWrapperCard
      header={
        <header>
          <h2 className="text-2xl font-medium text-text-primary">
            {t('setting.chatChannels')}
          </h2>
          <p className="mt-1 text-sm text-text-secondary">
            {t('setting.chatChannelsDescription')}
          </p>
        </header>
      }
    >
      <div className="h-full p-5 overflow-x-hidden overflow-y-auto">
        <section className="flex flex-col gap-3">
          {categorizedList?.length <= 0 && (
            <div className="text-text-secondary w-full flex justify-center items-center h-20">
              {t('setting.channelEmptyTip')}
            </div>
          )}
          {categorizedList.map((item, index) => (
            <AddedChannelCard key={index} {...item} onEdit={showEditingModal} />
          ))}
        </section>

        <section className="mt-8">
          <header className="flex flex-row items-center justify-between space-y-0 p-0 pb-4">
            <h2 className="text-2xl font-medium">
              {t('setting.availableChannels')}
              <div className="text-sm text-text-secondary font-normal mt-1.5">
                {t('setting.availableChannelsDescription')}
              </div>
            </h2>
          </header>

          <ul className="@container grid sm:grid-cols-1 lg:grid-cols-2 xl:grid-cols-2 2xl:grid-cols-4 3xl:grid-cols-4 gap-4">
            {channelTemplates.map((item) => (
              <li key={item.id} className="h-full">
                <AvailableChannelCard
                  {...item}
                  onAdd={() => showAddingModal(item)}
                />
              </li>
            ))}
          </ul>
        </section>
      </div>

      {modalVisible && (
        <AddChatChannelModal
          visible
          loading={loading}
          hideModal={hideModal}
          onOk={handleOk}
          channel={activeChannel}
          record={editingRecord}
        />
      )}
    </ProfileSettingWrapperCard>
  );
};

export default ChatChannel;
