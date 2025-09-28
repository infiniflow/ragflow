// json:{
//   "bottom": 554.3333333333334,
//   "img_id": "imagetemps-aa4bb8489c1611f0915a047c16ec874f",
//   "layout_type": "text",
//   "layoutno": "text-0",
//   "page_number": 1,
//   "position_tag": "@@1\t87.7\t506.0\t75.3\t554.3##",
//   "text": "0 中华人民共和国民法典第四百六十三条本编调整因合同产生的民事关系。\\n 1 中华人民共和国民法典第四百六十四条合同是民事主体之间设立、变更、终止民事法律关系的协议。\\n\\n婚姻、收养、监护等有关身份关系的协议，适用有关该身份关系的法律规定；没有规定的，可以根据其性质参照适用本编规定。\\n 2 中华人民共和国民法典第四百六十五条依法成立的合同，受法律保护。\\n\\n依法成立的合同，仅对当事人具有法律约束力，但是法律另有规定的除外。\\n 3 中华人民共和国民法典第四百六十六条当事人对合同条款的理解有争议的，应当依据本法第一百四十二条第一款的规定，确定争议条款的含义。\\n\\n合同文本采用两种以上文字订立并约定具有同等效力的，对各文本使用的词句推定具有相同含义。各文本使用的词句不一致的，应当根据合同的相关条款、性质、目的以及诚信原则等予以解释。\\n 4 中华人民共和国民法典第四百六十七条本法或者其他法律没有明文规定的合同，适用本编通则的规定，并可以参照适用本编或者其他法律最相类似合同的规定。\\n\\n在中华人民共和国境内履行的中外合资经营企业合同、中外合作经营企业合同、中外合作勘探开发自然资源合同，适用中华人民共和国法律。\\n 5 中华人民共和国民法典第四百六十八条非因合同产生的债权债务关系，适用有关该债权债务关系的法律规定；没有规定的，适用本编通则的有关规定，但是根据其性质不能适用的除外。\\n\\n6 中华人民共和国民法典第四百六十九条当事人订立合同，可以采用书面形式、口头形式或者其他形式。\\n\\n书面形式是合同书、信件、电报、电传、传真等可以有形地表现所载内容的形式。\\n\\n以电子数据交换、电子邮件等方式能够有形地表现所载内容，并可以随时调取查用的数据电文，视为书面形式。\\n 7 中华人民共和国民法典第四百七十条合同的内容由当事人约定，一般包括下列条款：\\n\\n（一）当事人的姓名或者名称和住所；\\n\\n（二）标的；\\n\\n（三）数量；\\n\\n（四）质量；\\n\\n（五）价款或者报酬；\\n\\n（六）履行期限、地点和方式；\\n\\n（七）违约责任；\\n\\n（八）解决争议的方法。\\n\\n当事人可以参照各类合同的示范文本订立合同。\\n 8 中华人民共和国民法典第四百七十一条当事人订立合同，可以采取要约、承诺方式或者其他方式。\\n 9 中华人民共和国民法典第四百七十二条要约是希望与他人订立合同的意思表示，该意思表示应当符合下列条件：\\n\\n（一）内容具体确定；\\n\\n（二）表明经受要约人承诺，要约人即受该意思表示约束。\\n 10 中华人民共和国民法典第四百七十三条要约邀请是希望他人向自己发出要约的表示。",
//   "top": 75.33333333333333,
//   "x0": 87.66666666666667,
//   "x1": 506

// }

import { Checkbox } from '@/components/ui/checkbox';
import { cn } from '@/lib/utils';
import { CheckedState } from '@radix-ui/react-checkbox';
import { useCallback, useEffect, useRef, useState } from 'react';
import { ChunkTextMode } from '../../constant';
import styles from '../../index.less';
export const parserKeyMap = {
  json: 'text',
  chunks: 'content_with_weight',
};
type IProps = {
  initialValue: {
    key: keyof typeof parserKeyMap;
    type: string;
    value: {
      [key: string]: string;
    }[];
  };
  isChunck?: boolean;
  handleCheck: (e: CheckedState, index: number) => void;
  selectedChunkIds: string[] | undefined;
  unescapeNewlines: (text: string) => string;
  escapeNewlines: (text: string) => string;
  onSave: (data: {
    value: {
      text: string;
    }[];
    key: string;
    type: string;
  }) => void;
  className?: string;
  textMode?: ChunkTextMode;
};
export const ArrayContainer = (props: IProps) => {
  const {
    initialValue,
    isChunck,
    handleCheck,
    selectedChunkIds,
    unescapeNewlines,
    escapeNewlines,
    onSave,
    className,
    textMode,
  } = props;

  const [content, setContent] = useState(initialValue);

  useEffect(() => {
    setContent(initialValue);
    console.log('initialValue json parse', initialValue);
  }, [initialValue]);

  const [activeEditIndex, setActiveEditIndex] = useState<number | undefined>(
    undefined,
  );
  const editDivRef = useRef<HTMLDivElement>(null);
  const handleEdit = useCallback(
    (e?: any, index?: number) => {
      console.log(e, e.target.innerText);
      setContent((pre) => ({
        ...pre,
        value: pre.value.map((item, i) => {
          if (i === index) {
            return {
              ...item,
              [parserKeyMap[content.key]]: e.target.innerText,
            };
          }
          return item;
        }),
      }));
      setActiveEditIndex(index);
    },
    [setContent, setActiveEditIndex],
  );
  const handleSave = useCallback(
    (e: any) => {
      console.log(e, e.target.innerText);
      const saveData = {
        ...content,
        value: content.value?.map((item, index) => {
          if (index === activeEditIndex) {
            return {
              ...item,
              [parserKeyMap[content.key]]: unescapeNewlines(e.target.innerText),
            };
          } else {
            return item;
          }
        }),
      };
      onSave(saveData);
      setActiveEditIndex(undefined);
    },
    [content, onSave],
  );

  useEffect(() => {
    if (activeEditIndex !== undefined && editDivRef.current) {
      editDivRef.current.focus();
      editDivRef.current.textContent =
        content.value[activeEditIndex][parserKeyMap[content.key]];
    }
  }, [activeEditIndex, content]);

  return (
    <>
      {content.value?.map((item, index) => {
        if (item[parserKeyMap[content.key]] === '') {
          return null;
        }
        return (
          <section
            key={index}
            className={
              isChunck
                ? 'bg-bg-card my-2 p-2 rounded-lg flex gap-1 items-start'
                : ''
            }
          >
            {isChunck && (
              <Checkbox
                onCheckedChange={(e) => {
                  handleCheck(e, index);
                }}
                checked={selectedChunkIds?.some(
                  (id) => id.toString() === index.toString(),
                )}
              ></Checkbox>
            )}
            {activeEditIndex === index && (
              <div
                ref={editDivRef}
                contentEditable={true}
                onBlur={handleSave}
                //   onKeyUp={handleChange}
                // dangerouslySetInnerHTML={{
                //   __html: DOMPurify.sanitize(
                //     escapeNewlines(
                //       content.value[index][parserKeyMap[content.key]],
                //     ),
                //   ),
                // }}
                className={cn(
                  'w-full bg-transparent text-text-secondary border-none focus-visible:border-none focus-visible:ring-0 focus-visible:ring-offset-0 focus-visible:outline-none p-0',
                  className,
                )}
              ></div>
            )}
            {activeEditIndex !== index && (
              <div
                className={cn(
                  'text-text-secondary overflow-auto scrollbar-auto whitespace-pre-wrap w-full',
                  {
                    [styles.contentEllipsis]:
                      textMode === ChunkTextMode.Ellipse,
                  },
                )}
                key={index}
                onClick={(e) => {
                  handleEdit(e, index);
                }}
              >
                {escapeNewlines(item[parserKeyMap[content.key]])}
              </div>
            )}
          </section>
        );
      })}
    </>
  );
};
