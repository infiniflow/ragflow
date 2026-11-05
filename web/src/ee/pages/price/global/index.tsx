import { ReactNode, createContext, useContext } from 'react';

/**
 * Enterprise seam for the global upgrade modal. The enterprise build replaces
 * this file with the price modal providers; open source never enables billing,
 * so the provider renders its children unchanged.
 */

interface UpgradeModalContextType {
  isModalOpen: boolean;
  openModal: (modalType?: string, modalProps?: any) => void;
  closeModal: () => void;
}

const UpgradeModalContext = createContext<UpgradeModalContextType | undefined>(
  undefined,
);

export const UpgradeModalProvider = ({
  children,
}: {
  children?: ReactNode;
}) => <>{children}</>;

export const useUpgradeModal = () => {
  const context = useContext(UpgradeModalContext);
  return (
    context ?? {
      isModalOpen: false,
      openModal: () => {},
      closeModal: () => {},
    }
  );
};
