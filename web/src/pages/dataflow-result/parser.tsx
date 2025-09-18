import { TimelineNode } from '@/components/originui/timeline';
import Spotlight from '@/components/spotlight';
import { Spin } from '@/components/ui/spin';
import classNames from 'classnames';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import FormatPreserveEditor from './components/parse-editer';
import RerunButton from './components/rerun-button';
import { useFetchParserList, useFetchPaserText } from './hooks';
interface IProps {
  isChange: boolean;
  setIsChange: (isChange: boolean) => void;
  step?: TimelineNode;
}
const ParserContainer = (props: IProps) => {
  const { isChange, setIsChange, step } = props;
  const { data: initialValue, rerun: onSave } = useFetchPaserText();
  const { t } = useTranslation();
  const { loading } = useFetchParserList();

  const [initialText, setInitialText] = useState(initialValue);
  const handleSave = (newContent: string) => {
    console.log('保存内容:', newContent);
    if (newContent !== initialText) {
      setIsChange(true);
      onSave(newContent);
    } else {
      setIsChange(false);
    }
    // Here, the API is called to send newContent to the backend
  };
  const handleReRunFunc = () => {
    setIsChange(false);
  };
  return (
    <>
      {isChange && (
        <div className=" absolute top-2 right-6">
          <RerunButton step={step} onRerun={handleReRunFunc} />
        </div>
      )}
      <div className={classNames('flex flex-col w-full')}>
        <Spin spinning={loading} className="" size="large">
          <div className="h-[50px] flex flex-col justify-end pb-[5px]">
            <div>
              <h2 className="text-[16px]">
                {t('dataflowParser.parseSummary')}
              </h2>
              <div className="text-[12px] text-text-secondary italic ">
                {t('dataflowParser.parseSummaryTip')}
              </div>
            </div>
          </div>
          <div className=" border rounded-lg p-[20px] box-border h-[calc(100vh-180px)] overflow-auto scrollbar-none">
            <FormatPreserveEditor
              initialValue={initialText}
              onSave={handleSave}
              className="!h-[calc(100vh-220px)]"
            />
            <Spotlight opcity={0.6} coverage={60} />
          </div>
        </Spin>
      </div>
    </>
  );
};
export default ParserContainer;
