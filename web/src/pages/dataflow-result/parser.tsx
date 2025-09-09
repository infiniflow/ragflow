import { Spin } from '@/components/ui/spin';
import classNames from 'classnames';
import { useTranslation } from 'react-i18next';
import { useFetchParserList } from './hooks';

const ParserContainer = () => {
  const { t } = useTranslation();
  const { loading } = useFetchParserList();
  return (
    <>
      <div className={classNames('flex flex-col w-3/5')}>
        <Spin spinning={loading} className="" size="large">
          <div className="h-[100px] flex flex-col justify-end pb-[5px]">
            <div>
              <h2 className="text-[24px]">{t('chunk.chunkResult')}</h2>
              <div className="text-[14px] text-text-secondary">
                {t('chunk.chunkResultTip')}
              </div>
            </div>
          </div>
          <div className=" rounded-[16px] bg-[#FFF]/10 pl-[20px] pb-[20px] pt-[20px] box-border	mb-2">
            parser
          </div>
        </Spin>
      </div>
    </>
  );
};
export default ParserContainer;
