import { IconFont } from '@/components/icon-font';
import { Routes } from '@/routes';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'umi';
import { SeeAllAppCard } from './application-card';
import { ChatList } from './chat-list';
import { SearchList } from './search-list';

const IconMap = {
  [Routes.Chats]: 'chat',
  [Routes.Searches]: 'search',
  [Routes.Agents]: 'agent',
};

export function Applications() {
  // const [val, setVal] = useState(Routes.Chats);
  const { t } = useTranslation();
  const navigate = useNavigate();

  const handleNavigate = useCallback(
    (path: Routes) => {
      navigate(path);
    },
    [navigate],
  );

  const options = useMemo(
    () => [
      { value: Routes.Chats, label: t('chat.chatApps') },
      { value: Routes.Searches, label: t('search.searchApps') },
      // { value: Routes.Agents, label: t('header.flow') },
    ],
    [t],
  );

  // const handleChange = (path: SegmentedValue) => {
  //   setVal(path as string);
  // };

  return (
    <>
      {options.map((option) => (
        <section className="mt-12" key={option.value}>
          <div className="flex justify-between items-center mb-5">
            <h2 className="text-2xl font-bold flex gap-2.5">
              <IconFont
                name={IconMap[option.value as keyof typeof IconMap]}
                className="size-8"
              ></IconFont>
              {option.label}
            </h2>
            {/* <Segmented
          options={options}
          value={val}
          onChange={handleChange}
          className="bg-bg-card border border-border-button rounded-full"
          activeClassName="bg-text-primary border-none"
        ></Segmented> */}
          </div>
          <div className="flex flex-wrap gap-4">
            {/* {val === Routes.Agents && <Agents></Agents>} */}
            {option.value === Routes.Chats && <ChatList></ChatList>}
            {option.value === Routes.Searches && <SearchList></SearchList>}
            {
              <SeeAllAppCard
                click={() => handleNavigate(option.value as Routes)}
              ></SeeAllAppCard>
            }
          </div>
        </section>
      ))}
    </>
  );
}
