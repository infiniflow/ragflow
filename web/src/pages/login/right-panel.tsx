import { useTranslate } from '@/hooks/common-hooks';
import styles from './index.less';

const LoginRightPanel = () => {
  const { t } = useTranslate('login');
  return (
    <section className={styles.rightPanel}>
    </section>
  );
};

export default LoginRightPanel;
