import { ExclamationCircleFilled } from '@ant-design/icons';
import { Modal } from 'antd';

const { confirm } = Modal;

interface IProps {
  onOk?: (...args: any[]) => any;
  onCancel?: (...args: any[]) => any;
}

export const showDeleteConfirm = ({ onOk, onCancel }: IProps) => {
  confirm({
    title: 'Are you sure delete this item?',
    icon: <ExclamationCircleFilled />,
    content: 'Some descriptions',
    okText: 'Yes',
    okType: 'danger',
    cancelText: 'No',
    onOk() {
      onOk?.();
    },
    onCancel() {
      onCancel?.();
    },
  });
};

export default showDeleteConfirm;
