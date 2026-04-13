import { useState } from 'react';

export interface IModalManagerChildrenProps {
  showModal(): void;
  hideModal(): void;
  visible: boolean;
}
interface IProps {
  children: (props: IModalManagerChildrenProps) => React.ReactNode;
}

const ModalManager = ({ children }: IProps) => {
  const [visible, setVisible] = useState(false);

  const showModal = () => {
    setVisible(true);
  };
  const hideModal = () => {
    setVisible(false);
  };

  return children({ visible, showModal, hideModal });
};

export default ModalManager;
