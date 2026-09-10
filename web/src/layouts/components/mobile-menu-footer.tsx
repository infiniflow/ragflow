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
      className="text-text-secondary transition-colors hover:text-cable-brand"
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
    <div className="shrink-0 border-t border-cable-divider px-4 py-4">
      <div className="flex flex-wrap items-center justify-center gap-x-2 gap-y-1 text-sm">
        <FooterLink
          href="https://ragflow.io/docs/dev/category/user-guides"
          target="_blank"
          rel="noreferrer noopener"
          onClick={onClose}
        >
          {t('header.help')}
        </FooterLink>
      </div>
    </div>
  );
}
