import { TimelineNode } from '@/components/originui/timeline';
import Spotlight from '@/components/spotlight';
import { Spin } from '@/components/ui/spin';
import classNames from 'classnames';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import CheckboxSets from './components/chunk-result-bar/checkbox-sets';
import FormatPreserEditor from './components/parse-editer';
import RerunButton from './components/rerun-button';
import { TimelineNodeType } from './constant';
import { useFetchParserList } from './hooks';
import { IDslComponent } from './interface';
interface IProps {
  isChange: boolean;
  setIsChange: (isChange: boolean) => void;
  step?: TimelineNode;
  data: { value: IDslComponent; key: string };
  reRunLoading: boolean;
  reRunFunc: (data: { value: IDslComponent; key: string }) => void;
}
const ParserContainer = (props: IProps) => {
  const { isChange, setIsChange, step, data, reRunFunc, reRunLoading } = props;
  const { t } = useTranslation();
  const { loading } = useFetchParserList();
  const [selectedChunkIds, setSelectedChunkIds] = useState<string[]>([]);
  const initialValue = useMemo(() => {
    const outputs = data?.value?.obj?.params?.outputs;
    const key = outputs?.output_format?.value;
    const value = outputs[key]?.value;
    const type = outputs[key]?.type;
    console.log('outputs-->', outputs);
    return {
      key,
      type,
      value,
    };
  }, [data]);

  const [initialText, setInitialText] = useState(initialValue);
  const handleSave = (newContent: any) => {
    console.log('newContent-change-->', newContent, initialValue);
    if (JSON.stringify(newContent) !== JSON.stringify(initialValue)) {
      setIsChange(true);
      setInitialText(newContent);
    } else {
      setIsChange(false);
    }
    // Here, the API is called to send newContent to the backend
  };

  const handleReRunFunc = useCallback(() => {
    const newData: { value: IDslComponent; key: string } = {
      ...data,
      value: {
        ...data.value,
        obj: {
          ...data.value.obj,
          params: {
            ...(data.value?.obj?.params || {}),
            outputs: {
              ...(data.value?.obj?.params?.outputs || {}),
              [initialText.key]: {
                type: initialText.type,
                value: initialText.value,
              },
            },
          },
        },
      },
    };
    reRunFunc(newData);
    setIsChange(false);
  }, [data, initialText, reRunFunc, setIsChange]);

  const handleRemoveChunk = useCallback(async () => {
    if (selectedChunkIds.length > 0) {
      initialText.value = initialText.value.filter(
        (item: any, index: number) => !selectedChunkIds.includes(index + ''),
      );
      setSelectedChunkIds([]);
    }
  }, [selectedChunkIds, initialText]);

  const handleCheckboxClick = useCallback(
    (id: string | number, checked: boolean) => {
      console.log('handleCheckboxClick', id, checked, selectedChunkIds);
      setSelectedChunkIds((prev) => {
        if (checked) {
          return [...prev, id.toString()];
        } else {
          return prev.filter((item) => item.toString() !== id.toString());
        }
      });
    },
    [],
  );

  const selectAllChunk = useCallback(
    (checked: boolean) => {
      setSelectedChunkIds(
        checked ? initialText.value.map((x, index: number) => index) : [],
      );
    },
    [initialText.value],
  );

  const isChunck =
    step?.type === TimelineNodeType.characterSplitter ||
    step?.type === TimelineNodeType.titleSplitter ||
    step?.type === TimelineNodeType.splitter;
  return (
    <>
      {isChange && (
        <div className=" absolute top-2 right-6">
          <RerunButton
            step={step}
            onRerun={handleReRunFunc}
            loading={reRunLoading}
          />
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

          {isChunck && (
            <div className="pt-[5px] pb-[5px]">
              <CheckboxSets
                selectAllChunk={selectAllChunk}
                removeChunk={handleRemoveChunk}
                checked={selectedChunkIds.length === initialText.value.length}
                selectedChunkIds={selectedChunkIds}
              />
            </div>
          )}

          <div className=" border rounded-lg p-[20px] box-border h-[calc(100vh-180px)] w-[calc(100%-20px)] overflow-auto scrollbar-none">
            <FormatPreserEditor
              initialValue={initialText}
              onSave={handleSave}
              className={
                initialText.key !== 'json' ? '!h-[calc(100vh-220px)]' : ''
              }
              isChunck={isChunck}
              isDelete={
                step?.type === TimelineNodeType.characterSplitter ||
                step?.type === TimelineNodeType.titleSplitter ||
                step?.type === TimelineNodeType.splitter
              }
              handleCheckboxClick={handleCheckboxClick}
              selectedChunkIds={selectedChunkIds}
            />
            <Spotlight opcity={0.6} coverage={60} />
          </div>
        </Spin>
      </div>
    </>
  );
};
export default ParserContainer;
