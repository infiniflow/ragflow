import { CardSineLineContainer } from '@/components/card-singleline-container';
import { HomeIcon } from '@/components/svg-icon';
import { Segmented, SegmentedValue } from '@/components/ui/segmented';
import { Routes } from '@/routes';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'umi';
import { Agents } from './agent-list';
import { SeeAllAppCard } from './application-card';
import { ChatList } from './chat-list';
import { SearchList } from './search-list';

const IconMap = {
  [Routes.Chats]: 'chats',
  [Routes.Searches]: 'searches',
  [Routes.Agents]: 'agents',
};

export function Applications() {
  const [val, setVal] = useState(Routes.Chats);
  const { t } = useTranslation();
  const navigate = useNavigate();

  const handleNavigate = useCallback(() => {
    navigate(val);
  }, [navigate, val]);

  const options = useMemo(
    () => [
      { value: Routes.Chats, label: t('chat.chatApps') },
      { value: Routes.Searches, label: t('search.searchApps') },
      { value: Routes.Agents, label: t('header.flow') },
    ],
    [t],
  );

  const handleChange = (path: SegmentedValue) => {
    setVal(path as Routes);
  };

  return (
    <section className="mt-12">
      <div className="flex justify-between items-center mb-5">
        <h2 className="text-2xl font-semibold flex gap-2.5">
          {/* <IconFont
            name={IconMap[val as keyof typeof IconMap]}
            className="size-8"
          ></IconFont> */}
          <HomeIcon
            name={`${IconMap[val as keyof typeof IconMap]}`}
            width={'32'}
          />
          {options.find((x) => x.value === val)?.label}
        </h2>
        <Segmented
          options={options}
          value={val}
          onChange={handleChange}
          buttonSize="xl"
          // className="bg-bg-card border border-border-button rounded-lg"
          // activeClassName="bg-text-primary border-none rounded-lg"
        ></Segmented>
      </div>
      {/* <div className="flex flex-wrap gap-4"> */}
      <CardSineLineContainer>
        {val === Routes.Agents && <Agents></Agents>}
        {val === Routes.Chats && <ChatList></ChatList>}
        {val === Routes.Searches && <SearchList></SearchList>}
        {<SeeAllAppCard click={handleNavigate}></SeeAllAppCard>}
      </CardSineLineContainer>
      {/* </div> */}
    </section>
  );
}
