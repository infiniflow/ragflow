import React, { useSyncExternalStore } from 'react';

export interface AnchorItem {
  key: string;
  href: string;
  title: string;
  children?: AnchorItem[];
}

interface SimpleAnchorProps {
  items: AnchorItem[];
  className?: string;
  style?: React.CSSProperties;
}

// Subscribe to URL hash changes
const subscribeHash = (callback: () => void) => {
  window.addEventListener('hashchange', callback);
  return () => window.removeEventListener('hashchange', callback);
};

const getHash = () => window.location.hash;

const Anchor: React.FC<SimpleAnchorProps> = ({
  items,
  className = '',
  style = {},
}) => {
  // Sync with URL hash changes, to highlight the active item
  const hash = useSyncExternalStore(subscribeHash, getHash);

  // Handle menu item click
  const handleClick = (
    e: React.MouseEvent<HTMLAnchorElement>,
    href: string,
  ) => {
    e.preventDefault();
    const targetId = href.replace('#', '');
    const targetElement = document.getElementById(targetId);

    if (targetElement) {
      // Update URL hash (triggers hashchange event)
      window.location.hash = href;
      // Smooth scroll to target
      targetElement.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }
  };

  if (items.length === 0) return null;

  return (
    <nav className={className} style={style}>
      <ul className="list-none p-0 m-0">
        {items.map((item) => (
          <li key={item.key} className="mb-2">
            <a
              href={item.href}
              onClick={(e) => handleClick(e, item.href)}
              className={`block px-3 py-1.5 no-underline rounded cursor-pointer transition-all duration-300 hover:text-accent-primary/70 ${
                hash === item.href
                  ? 'text-accent-primary bg-accent-primary-5'
                  : 'text-text-secondary bg-transparent'
              }`}
            >
              {item.title}
            </a>
            {item.children && item.children.length > 0 && (
              <ul className="list-none p-0 ml-4 mt-1">
                {item.children.map((child) => (
                  <li key={child.key} className="mb-1">
                    <a
                      href={child.href}
                      onClick={(e) => handleClick(e, child.href)}
                      className={`block px-3 py-1 text-sm no-underline rounded cursor-pointer transition-all duration-300 hover:text-accent-primary/70 ${
                        hash === child.href
                          ? 'text-accent-primary bg-accent-primary-5'
                          : 'text-text-secondary bg-transparent'
                      }`}
                    >
                      {child.title}
                    </a>
                  </li>
                ))}
              </ul>
            )}
          </li>
        ))}
      </ul>
    </nav>
  );
};

export default Anchor;
