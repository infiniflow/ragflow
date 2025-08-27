import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { useTheme } from '@/components/theme-provider';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Segmented, SegmentedValue } from '@/components/ui/segmented';
import { LanguageList, LanguageMap, ThemeEnum } from '@/constants/common';
import { useChangeLanguage } from '@/hooks/logic-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useNavigateWithFromState } from '@/hooks/route-hook';
import { useFetchUserInfo } from '@/hooks/user-setting-hooks';
import { Routes } from '@/routes';
import { camelCase } from 'lodash';
import {
  ChevronDown,
  CircleHelp,
  Cpu,
  File,
  Github,
  House,
  Library,
  MessageSquareText,
  Moon,
  Search,
  Sun,
} from 'lucide-react';
import React, { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation } from 'umi';
import { BellButton } from './bell-button';

const handleDocHelpCLick = () => {
  window.open('https://ragflow.io/docs/dev/category/guides', 'target');
};

export function Header() {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const navigate = useNavigateWithFromState();
  const { navigateToOldProfile } = useNavigatePage();

  const changeLanguage = useChangeLanguage();
  const { setTheme, theme } = useTheme();

  const {
    data: { language = 'English', avatar, nickname },
  } = useFetchUserInfo();

  const handleItemClick = (key: string) => () => {
    changeLanguage(key);
  };

  const items = LanguageList.map((x) => ({
    key: x,
    label: <span>{LanguageMap[x as keyof typeof LanguageMap]}</span>,
  }));

  const onThemeClick = React.useCallback(() => {
    setTheme(theme === ThemeEnum.Dark ? ThemeEnum.Light : ThemeEnum.Dark);
  }, [setTheme, theme]);

  const tagsData = useMemo(
    () => [
      { path: Routes.Root, name: t('header.Root'), icon: House },
      { path: Routes.Datasets, name: t('header.dataset'), icon: Library },
      { path: Routes.Chats, name: t('header.chat'), icon: MessageSquareText },
      { path: Routes.Searches, name: t('header.search'), icon: Search },
      { path: Routes.Agents, name: t('header.flow'), icon: Cpu },
      { path: Routes.Files, name: t('header.fileManager'), icon: File },
    ],
    [t],
  );

  const options = useMemo(() => {
    return tagsData.map((tag) => {
      const HeaderIcon = tag.icon;

      return {
        label:
          tag.path === Routes.Root ? (
            <HeaderIcon className="size-6"></HeaderIcon>
          ) : (
            <span>{tag.name}</span>
          ),
        value: tag.path,
      };
    });
  }, [tagsData]);

  // const currentPath = useMemo(() => {
  //   return (
  //     tagsData.find((x) => pathname.startsWith(x.path))?.path || Routes.Root
  //   );
  // }, [pathname, tagsData]);

  const handleChange = (path: SegmentedValue) => {
    navigate(path as Routes);
  };

  const handleLogoClick = useCallback(() => {
    navigate(Routes.Root);
  }, [navigate]);

  return (
    <section className="p-5 pr-14 flex justify-between items-center ">
      <div className="flex items-center gap-4">
        <img
          src={'/logo.svg'}
          alt="logo"
          className="size-10 mr-[12]"
          onClick={handleLogoClick}
        />
        <a
          className="flex items-center gap-1.5 text-text-secondary"
          target="_blank"
          href="https://github.com/infiniflow/ragflow"
          rel="noreferrer"
        >
          <Github className="size-4" />
          {/* <span className=" text-base">21.5k stars</span> */}
        </a>
      </div>
      <Segmented
        options={options}
        value={pathname}
        onChange={handleChange}
      ></Segmented>
      <div className="flex items-center gap-5 text-text-badge">
        <DropdownMenu>
          <DropdownMenuTrigger>
            <div className="flex items-center gap-1">
              {t(`common.${camelCase(language)}`)}
              <ChevronDown className="size-4" />
            </div>
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            {items.map((x) => (
              <DropdownMenuItem key={x.key} onClick={handleItemClick(x.key)}>
                {x.label}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
        <Button variant={'ghost'} onClick={handleDocHelpCLick}>
          <CircleHelp />
        </Button>
        <Button variant={'ghost'} onClick={onThemeClick}>
          {theme === 'light' ? <Sun /> : <Moon />}
        </Button>
        <BellButton></BellButton>
        <div className="relative">
          <RAGFlowAvatar
            name={nickname}
            avatar={avatar}
            className="size-8 cursor-pointer"
            onClick={navigateToOldProfile}
          ></RAGFlowAvatar>
          {/* Temporarily hidden */}
          {/* <Badge className="h-5 w-8 absolute font-normal p-0 justify-center -right-8 -top-2 text-bg-base bg-gradient-to-l from-[#42D7E7] to-[#478AF5]">
            Pro
          </Badge> */}
        </div>
      </div>
    </section>
  );
}
