import ApiContent from '@/components/api-service/chat-overview-modal/api-content';

import styles from './index.module.less';

const ApiPage = () => {
  return (
    <div className={styles.apiWrapper}>
      <ApiContent idKey="dialogId"></ApiContent>
    </div>
  );
};

export default ApiPage;
