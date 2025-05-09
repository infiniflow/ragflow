import { RAGFlowAvatar } from '@/components/ragflow-avatar';
import { useTheme } from '@/components/theme-provider';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Segmented, SegmentedValue } from '@/components/ui/segmented';
import { LanguageList, LanguageMap } from '@/constants/common';
import { useChangeLanguage } from '@/hooks/logic-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useNavigateWithFromState } from '@/hooks/route-hook';
import { useFetchUserInfo, useListTenant } from '@/hooks/user-setting-hooks';
import { TenantRole } from '@/pages/user-setting/constants';
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
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation } from 'umi';

const handleDocHelpCLick = () => {
  window.open('https://ragflow.io/docs/dev/category/guides', 'target');
};

export function Header() {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const navigate = useNavigateWithFromState();
  const { navigateToProfile } = useNavigatePage();

  const changeLanguage = useChangeLanguage();
  const { setTheme, theme } = useTheme();

  const {
    data: { language = 'English', avatar, nickname },
  } = useFetchUserInfo();

  const handleItemClick = (key: string) => () => {
    changeLanguage(key);
  };

  const { data } = useListTenant();

  const showBell = useMemo(
    () => data.some((x) => x.role === TenantRole.Invite),
    [data],
  );

  /* nav definitions */
  const tagsData = useMemo(
    () => [
      { path: Routes.Home, name: t('header.home'), icon: House },
      { path: Routes.Datasets, name: t('header.knowledgeBase'), icon: Library },
      { path: Routes.Chats, name: t('header.chat'), icon: MessageSquareText },
      {
        path: Routes.Searches,
        name: t('header.search'),
        icon: Search,
        disabled: true,
      }, // 🔒 disabled
      {
        path: Routes.Agents,
        name: t('header.flow'),
        icon: Cpu,
        disabled: true,
      },
      {
        path: Routes.Files,
        name: t('header.fileManager'),
        icon: File,
        disabled: true,
      },
    ],
    [t],
  );

  /** Segmented control options */
  const options = useMemo(
    () =>
      tagsData.map((tag) => {
        const Icon = tag.icon;
        return {
          label:
            tag.path === Routes.Home ? (
              <Icon className="size-6" />
            ) : (
              <span>{tag.name}</span>
            ),
          value: tag.path,
          disabled: tag.disabled,
        };
      }),
    [tagsData],
  );

  const currentPath = useMemo(
    () =>
      tagsData.find((x) => pathname.startsWith(x.path))?.path || Routes.Home,
    [pathname, tagsData],
  );

  /** handle nav change */
  const handleChange = (path: SegmentedValue) => {
    if (path === Routes.Searches) return; // ignore disabled tab
    navigate(path as Routes);
  };

  const handleLogoClick = useCallback(() => navigate(Routes.Home), [navigate]);

  return (
    <section className="p-5 pr-14 flex justify-between items-center ">
      {/* logo + stars */}
      <div className="flex items-center gap-4">
        <img
          src="/logo.svg"
          alt="logo"
          className="size-10 mr-[12]"
          onClick={handleLogoClick}
        />
        <div className="flex items-center gap-1.5 text-text-sub-title">
          <Github className="size-3.5" />
          <span className="text-base">21.5k stars</span>
        </div>
      </div>

      {/* segmented nav */}
      <Segmented
        options={options}
        value={currentPath}
        onChange={handleChange}
      />

      {/* right-side controls */}
      <div className="flex items-center gap-5 text-text-badge">
        <DropdownMenu>
          <DropdownMenuTrigger>
            <div className="flex items-center gap-1">
              {t(`common.${camelCase(language)}`)}
              <ChevronDown className="size-4" />
            </div>
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            {LanguageList.map((x) => (
              <DropdownMenuItem key={x} onClick={handleItemClick(x)}>
                {LanguageMap[x as keyof typeof LanguageMap]}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>

        <Button variant="ghost" onClick={handleDocHelpCLick}>
          <CircleHelp />
        </Button>
        <Button
          variant="ghost"
          onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
        >
          {theme === 'light' ? <Sun /> : <Moon />}
        </Button>

        <div className="relative">
          <RAGFlowAvatar
            name={nickname}
            avatar={avatar}
            className="size-8 cursor-pointer"
            onClick={navigateToProfile}
          />
          <Badge className="h-5 w-8 absolute font-normal p-0 justify-center -right-8 -top-2 text-text-title-invert bg-gradient-to-l from-[#42D7E7] to-[#478AF5]">
            Pro
          </Badge>
        </div>
      </div>
    </section>
  );
}
