import { TimelineNode } from '@/components/originui/timeline';
import SvgIcon from '@/components/svg-icon';
import { Button } from '@/components/ui/button';
import { Modal } from '@/components/ui/modal/modal';
import { CircleAlert } from 'lucide-react';
import { useTranslation } from 'react-i18next';
interface RerunButtonProps {
  className?: string;
  step?: TimelineNode;
  onRerun?: () => void;
  loading?: boolean;
}
const RerunButton = (props: RerunButtonProps) => {
  const { className, step, onRerun, loading } = props;
  const { t } = useTranslation();
  const clickFunc = () => {
    console.log('click rerun button');
    Modal.show({
      visible: true,
      className: '!w-[560px]',
      title: t('dataflowParser.confirmRerun'),
      children: (
        <div
          dangerouslySetInnerHTML={{
            __html: t('dataflowParser.confirmRerunModalContent', {
              step: step?.title,
            }),
          }}
        ></div>
      ),
      okText: t('modal.okText'),
      cancelText: t('modal.cancelText'),
      onVisibleChange: (visible: boolean) => {
        if (!visible) {
          Modal.destroy();
        } else {
          onRerun?.();
          Modal.destroy();
        }
      },
    });
  };
  return (
    <div className="flex flex-col gap-2">
      <div className="text-xs text-text-primary flex items-center gap-1">
        <CircleAlert color="#d29e2d" strokeWidth={1} size={12} />
        {t('dataflowParser.rerunFromCurrentStepTip')}
      </div>
      <Button onClick={clickFunc} disabled={loading}>
        <SvgIcon name="rerun" width={16} />
        {t('dataflowParser.rerunFromCurrentStep')}
      </Button>
    </div>
  );
};

export default RerunButton;
