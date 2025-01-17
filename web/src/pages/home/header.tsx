import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar';
import { Button } from '@/components/ui/button';
import { Container } from '@/components/ui/container';
import { Segmented, SegmentedValue } from '@/components/ui/segmented';
import { useTranslate } from '@/hooks/common-hooks';
import { useNavigateWithFromState } from '@/hooks/route-hook';
import {
  ChevronDown,
  Cpu,
  Github,
  Library,
  MessageSquareText,
  Search,
  Star,
  Zap,
} from 'lucide-react';
import { useCallback, useMemo, useState } from 'react';
import { useLocation } from 'umi';

export function HomeHeader() {
  const { t } = useTranslate('header');
  const { pathname } = useLocation();
  const navigate = useNavigateWithFromState();
  const [currentPath, setCurrentPath] = useState('/home');

  const tagsData = useMemo(
    () => [
      { path: '/home', name: t('knowledgeBase'), icon: Library },
      { path: '/chat', name: t('chat'), icon: MessageSquareText },
      { path: '/search', name: t('search'), icon: Search },
      { path: '/flow', name: t('flow'), icon: Cpu },
      // { path: '/file', name: t('fileManager'), icon: FileIcon },
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

  // const currentPath = useMemo(() => {
  //   return tagsData.find((x) => pathname.startsWith(x.path))?.name || 'home';
  // }, [pathname, tagsData]);

  const handleChange = (path: SegmentedValue) => {
    // navigate(path as string);
    setCurrentPath(path as string);
  };

  const handleLogoClick = useCallback(() => {
    navigate('/');
  }, [navigate]);

  return (
    <section className="py-[12px] flex justify-between items-center">
      <div className="flex items-center gap-4">
        <img
          src={'/logo.svg'}
          alt="logo"
          className="w-[100] h-[100] mr-[12]"
          onClick={handleLogoClick}
        />
        <Button variant="secondary">
          <Github />
          21.5k stars
          <Star />
        </Button>
      </div>
      <div>
        <Segmented
          options={options}
          value={currentPath}
          onChange={handleChange}
          className="bg-colors-background-inverse-standard text-backgroundInverseStandard-foreground"
        ></Segmented>
      </div>
      <div className="flex items-center gap-4">
        <Container>
          V 0.13.0
          <Button variant="secondary" className="size-8">
            <ChevronDown />
          </Button>
        </Container>
        <Container className="px-3 py-2">
          <Avatar className="w-[30px] h-[30px]">
            <AvatarImage src="https://github.com/shadcn.png" />
            <AvatarFallback>CN</AvatarFallback>
          </Avatar>
          yifanwu92@gmail.com
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
