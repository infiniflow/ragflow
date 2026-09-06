import { USER_GUIDE_URL } from '@/constants/user-guide';
import { useTranslation } from 'react-i18next';

function FooterLink({
  children,
  onClick,
  href,
  target,
  rel,
}: {
  children: React.ReactNode;
  onClick?: () => void;
  href: string;
  target?: string;
  rel?: string;
}) {
  return (
    <a
      href={href}
      target={target}
      rel={rel}
      onClick={onClick}
      className="text-text-secondary transition-colors hover:text-text-primary"
    >
      {children}
    </a>
  );
}

type MobileMenuFooterProps = {
  onClose: () => void;
};

export function MobileMenuFooter({ onClose }: MobileMenuFooterProps) {
  const { t } = useTranslation();

  return (
    <div className="shrink-0 border-t border-border-button px-4 py-4">
      <div className="flex flex-wrap items-center justify-center gap-x-2 gap-y-1 text-sm">
        <FooterLink
          href={USER_GUIDE_URL}
          target="_blank"
          rel="noreferrer noopener"
          onClick={onClose}
        >
          {t('header.instruction', { defaultValue: 'Инструкция' })}
        </FooterLink>
      </div>
    </div>
  );
}
