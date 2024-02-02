import { useState } from 'react';

interface IProps {
  children: (props: {
    showModal(): void;
    hideModal(): void;
    visible: boolean;
  }) => React.ReactNode;
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
