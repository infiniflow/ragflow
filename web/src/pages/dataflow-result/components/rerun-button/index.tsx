import SvgIcon from '@/components/svg-icon';
import { Button } from '@/components/ui/button';
import { CircleAlert } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useRerunDataflow } from '../../hooks';
interface RerunButtonProps {
  className?: string;
}
const RerunButton = (props: RerunButtonProps) => {
  const { t } = useTranslation();
  const { loading } = useRerunDataflow();
  const clickFunc = () => {
    console.log('click rerun button');
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
