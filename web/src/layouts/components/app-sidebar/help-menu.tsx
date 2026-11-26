import { IconFontFill } from '@/components/icon-font';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Domain } from '@/constants/common';
import { useGitHubStars } from '@/hooks/use-github-request';
import { useFetchSystemVersion } from '@/hooks/use-user-setting-request';
import {
  LucideArrowUpRight,
  LucideBookOpen,
  LucideCircleHelp,
  LucideInfo,
  LucideStar,
} from 'lucide-react';
import { useCallback, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { DiscordUrl, GitHubUrl, HelpUrl } from './constants';

const StarCountFormatter = new Intl.NumberFormat('en-US');

function GitHubMenuItem() {
  const { t } = useTranslation();
  const { data: stars } = useGitHubStars();

  const openGitHub = useCallback(() => {
    window.open(GitHubUrl, '_blank', 'noreferrer,noopener');
  }, []);

  return (
    <DropdownMenuItem className="px-3 py-2.5" onClick={openGitHub}>
      <span className="flex items-center gap-2">
        <IconFontFill name="GitHub" />
        {t('header.github')}
      </span>
      {typeof stars === 'number' && (
        <span className="flex items-center gap-1 text-accent-primary">
          <LucideStar />
          {StarCountFormatter.format(stars)}
        </span>
      )}
    </DropdownMenuItem>
  );
}

export function HelpMenu() {
  const { t } = useTranslation();
  const { version, fetchSystemVersion } = useFetchSystemVersion();

  useEffect(() => {
    if (location.host !== Domain) {
      fetchSystemVersion();
    }
  }, [fetchSystemVersion]);

  const openDocs = useCallback(() => {
    window.open(HelpUrl, '_blank', 'noreferrer,noopener');
  }, []);

  const openDiscord = useCallback(() => {
    window.open(DiscordUrl, '_blank', 'noreferrer,noopener');
  }, []);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('header.help')}
          title={t('header.help')}
          className="shrink-0"
        >
          <LucideCircleHelp className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        side="right"
        align="end"
        sideOffset={12}
        className="w-60 p-2"
      >
        <DropdownMenuItem className="px-3 py-2.5" onClick={openDocs}>
          <span className="flex items-center gap-2">
            <LucideBookOpen />
            {t('sidebar.docs')}
          </span>
          <LucideArrowUpRight />
        </DropdownMenuItem>

        <DropdownMenuSeparator />
        <DropdownMenuItem
          justifyBetween={false}
          className="px-3 py-2.5"
          onClick={openDiscord}
        >
          <IconFontFill name="a-DiscordIconSVGVectorIcon" />
          {t('header.discord')}
        </DropdownMenuItem>
        <GitHubMenuItem />

        {version && (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuLabel className="flex items-center justify-between gap-2 px-3 py-2.5 text-sm font-normal text-text-secondary">
              <span className="flex shrink-0 items-center gap-2">
                <LucideInfo />
              </span>
              <span className="min-w-0 truncate" title={version}>
                {version}
              </span>
            </DropdownMenuLabel>
          </>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
