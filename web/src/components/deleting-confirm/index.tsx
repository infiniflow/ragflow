import { ExclamationCircleFilled } from '@ant-design/icons';
import { Modal } from 'antd';

const { confirm } = Modal;

interface IProps {
  onOk?: (...args: any[]) => any;
  onCancel?: (...args: any[]) => any;
}

export const showDeleteConfirm = ({
  onOk,
  onCancel,
}: IProps): Promise<number> => {
  return new Promise((resolve, reject) => {
    confirm({
      title: 'Are you sure delete this item?',
      icon: <ExclamationCircleFilled />,
      content: 'Some descriptions',
      okText: 'Yes',
      okType: 'danger',
      cancelText: 'No',
      async onOk() {
        try {
          const ret = await onOk?.();
          resolve(ret);
          console.info(ret);
        } catch (error) {
          reject(error);
        }
      },
      onCancel() {
        onCancel?.();
      },
    });
  });
};

export default showDeleteConfirm;
