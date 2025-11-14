import { TimelineNode } from '@/components/originui/timeline';
import Spotlight from '@/components/spotlight';
import { cn } from '@/lib/utils';
import classNames from 'classnames';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import ChunkResultBar from './components/chunk-result-bar';
import CheckboxSets from './components/chunk-result-bar/checkbox-sets';
import FormatPreserEditor from './components/parse-editer';
import RerunButton from './components/rerun-button';
import { TimelineNodeType } from './constant';
import { useChangeChunkTextMode } from './hooks';
import { IChunk, IDslComponent } from './interface';
interface IProps {
  isReadonly: boolean;
  isChange: boolean;
  setIsChange: (isChange: boolean) => void;
  step?: TimelineNode;
  data: { value: IDslComponent; key: string };
  reRunLoading: boolean;
  clickChunk: (chunk: IChunk) => void;
  summaryInfo: string;
  reRunFunc: (data: { value: IDslComponent; key: string }) => void;
}
const ParserContainer = (props: IProps) => {
  const {
    isChange,
    setIsChange,
    step,
    data,
    reRunFunc,
    reRunLoading,
    clickChunk,
    isReadonly,
    summaryInfo,
  } = props;
  const { t } = useTranslation();
  const [selectedChunkIds, setSelectedChunkIds] = useState<string[]>([]);
  const { changeChunkTextMode, textMode } = useChangeChunkTextMode();
  const initialValue = useMemo(() => {
    const outputs = data?.value?.obj?.params?.outputs;
    const key = outputs?.output_format?.value;
    if (!outputs || !key)
      return {
        key: '' as 'text' | 'html' | 'json' | 'chunks',
        type: '',
        value: [],
      };
    const value = outputs[key as keyof typeof outputs]?.value;
    const type = outputs[key as keyof typeof outputs]?.type;
    console.log('outputs-->', outputs, data, key, value);
    return {
      key: key as 'text' | 'html' | 'json' | 'chunks',
      type,
      value,
      params: data?.value?.obj?.params,
    };
  }, [data]);

  const [initialText, setInitialText] = useState(initialValue);

  useEffect(() => {
    setInitialText(initialValue);
  }, [initialValue]);
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
        (_item: any, index: number) => !selectedChunkIds.includes(index + ''),
      );
      setIsChange(true);
      setSelectedChunkIds([]);
    }
  }, [selectedChunkIds, initialText, setIsChange]);

  const handleCheckboxClick = useCallback(
    (id: string | number, checked: boolean) => {
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
        checked ? initialText.value.map((_x: any, index: number) => index) : [],
      );
    },
    [initialText.value],
  );

  const isChunck =
    step?.type === TimelineNodeType.characterSplitter ||
    step?.type === TimelineNodeType.titleSplitter;

  const handleCreateChunk = useCallback(
    (text: string) => {
      const newText = [...initialText.value, { text: text || ' ' }];
      setInitialText({
        ...initialText,
        value: newText as any,
      });
    },
    [initialText],
  );

  return (
    <>
      {isChange && !isReadonly && (
        <div className=" absolute top-2 right-6">
          <RerunButton
            step={step}
            onRerun={handleReRunFunc}
            loading={reRunLoading}
          />
        </div>
      )}
      <div className={classNames('flex flex-col w-full')}>
        {/* <Spin spinning={false} className="" size="large"> */}
        <div className="h-[50px] flex flex-col justify-end pb-[5px]">
          {!isChunck && (
            <div>
              <h2 className="text-[16px]">
                {t('dataflowParser.parseSummary')}
              </h2>
              <div className="text-[12px] text-text-secondary italic ">
                {/* {t('dataflowParser.parseSummaryTip')} */}
                {summaryInfo}
              </div>
            </div>
          )}
          {isChunck && (
            <div>
              <h2 className="text-[16px]">{t('dataflowParser.result')}</h2>
              <div className="text-[12px] text-text-secondary italic">
                {/* {t('chunk.chunkResultTip')} */}
              </div>
            </div>
          )}
        </div>

        {isChunck && (
          <div className="pt-[5px] pb-[5px] flex justify-between items-center">
            {!isReadonly && (
              <CheckboxSets
                selectAllChunk={selectAllChunk}
                removeChunk={handleRemoveChunk}
                checked={selectedChunkIds.length === initialText.value.length}
                selectedChunkIds={selectedChunkIds}
              />
            )}
            <ChunkResultBar
              isReadonly={isReadonly}
              changeChunkTextMode={changeChunkTextMode}
              createChunk={handleCreateChunk}
            />
          </div>
        )}

        <div
          className={cn(
            ' border rounded-lg p-[20px] box-border w-[calc(100%-20px)] overflow-auto scrollbar-auto',
            {
              'h-[calc(100vh-240px)]': isChunck,
              'h-[calc(100vh-180px)]': !isChunck,
            },
          )}
        >
          {initialText && (
            <FormatPreserEditor
              initialValue={initialText}
              onSave={handleSave}
              isReadonly={isReadonly}
              isChunck={isChunck}
              textMode={textMode}
              isDelete={
                step?.type === TimelineNodeType.characterSplitter ||
                step?.type === TimelineNodeType.titleSplitter
              }
              clickChunk={clickChunk}
              handleCheckboxClick={handleCheckboxClick}
              selectedChunkIds={selectedChunkIds}
            />
          )}
          <Spotlight opcity={0.6} coverage={60} />
        </div>
        {/* </Spin> */}
      </div>
    </>
  );
};
export default ParserContainer;
