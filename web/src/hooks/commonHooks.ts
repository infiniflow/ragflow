import { useState } from 'react';

export const useSetModalState = () => {
  const [visible, setVisible] = useState(false);

  const showModal = () => {
    setVisible(true);
  };
  const hideModal = () => {
    setVisible(false);
  };

  return { visible, showModal, hideModal };
};
