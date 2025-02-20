import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { Container } from '@/components/ui/container';
import { Segmented, SegmentedValue } from '@/components/ui/segmented';
import { useTranslate } from '@/hooks/common-hooks';
import { useNavigatePage } from '@/hooks/logic-hooks/navigate-hooks';
import { useNavigateWithFromState } from '@/hooks/route-hook';
import { cn } from '@/lib/utils';
import { Routes } from '@/routes';
import {
  ChevronDown,
  Cpu,
  File,
  Github,
  House,
  Library,
  MessageSquareText,
  Search,
  Star,
  Zap,
} from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useLocation } from 'umi';

export function Header() {
  const { t } = useTranslate('header');
  const { pathname } = useLocation();
  const navigate = useNavigateWithFromState();
  const { navigateToHome, navigateToProfile } = useNavigatePage();

  const tagsData = useMemo(
    () => [
      { path: Routes.Datasets, name: t('knowledgeBase'), icon: Library },
      { path: Routes.Chats, name: t('chat'), icon: MessageSquareText },
      { path: Routes.Searches, name: t('search'), icon: Search },
      { path: Routes.Agents, name: t('flow'), icon: Cpu },
      { path: Routes.Files, name: t('fileManager'), icon: File },
    ],
    [t],
  );

  const options = useMemo(() => {
    return tagsData.map((tag) => {
      const HeaderIcon = tag.icon;

      return {
        label: (
          <div className="flex items-center gap-1">
            <HeaderIcon className="size-5"></HeaderIcon>
            <span>{tag.name}</span>
          </div>
        ),
        value: tag.path,
      };
    });
  }, [tagsData]);

  const currentPath = useMemo(() => {
    return (
      tagsData.find((x) => pathname.startsWith(x.path))?.path || Routes.Home
    );
  }, [pathname, tagsData]);

  const isHome = Routes.Home === currentPath;

  const handleChange = (path: SegmentedValue) => {
    navigate(path as Routes);
  };

  const handleLogoClick = useCallback(() => {
    navigate(Routes.Home);
  }, [navigate]);

  return (
    <section className="py-6 px-10 flex justify-between items-center border-b">
      <div className="flex items-center gap-4">
        <img
          src={'/logo.svg'}
          alt="logo"
          className="w-[100] h-[100] mr-[12]"
          onClick={handleLogoClick}
        />
        <Button
          variant="secondary"
          className="bg-colors-background-inverse-standard"
        >
          <Github />
          21.5k stars
          <Star />
        </Button>
      </div>
      <div className="flex gap-2 items-center">
        <Button
          variant={'icon'}
          size={'icon'}
          onClick={navigateToHome}
          className={cn({
            'bg-colors-background-inverse-strong': isHome,
          })}
        >
          <House
            className={cn({
              'text-colors-text-inverse-strong': isHome,
            })}
          />
        </Button>
        <div className="h-8 w-[1px] bg-colors-outline-neutral-strong"></div>
        <Segmented
          options={options}
          value={currentPath}
          onChange={handleChange}
          className="bg-colors-background-inverse-standard text-backgroundInverseStandard-foreground"
        ></Segmented>
      </div>
      <div className="flex items-center gap-4">
        <Container className="bg-colors-background-inverse-standard hidden xl:flex">
          V 0.13.0
          <Button variant="secondary" className="size-8">
            <ChevronDown />
          </Button>
        </Container>
        <Container className="px-3 py-2 bg-colors-background-inverse-standard">
          <Avatar
            className="w-[30px] h-[30px] cursor-pointer"
            onClick={navigateToProfile}
          >
            <AvatarImage src="https://github.com/shadcn.png" />
            <AvatarFallback>CN</AvatarFallback>
          </Avatar>
          <span className="max-w-14 truncate"> yifanwu92@gmail.com</span>
          <Button
            variant="destructive"
            className="py-[2px] px-[8px] h-[23px] rounded-[4px]"
          >
            <Zap />
            Pro
          </Button>
        </Container>
      </div>
    </section>
  );
}
