import classNames from 'classnames';
import { useEffect } from 'react';
import { ISegmentedContentProps } from '../interface';

import LlmSettingItems from '@/components/llm-setting-items';
import {
  ChatVariableEnabledField,
  variableEnabledFieldMap,
} from '@/constants/chat';
import { Variable } from '@/interfaces/database/chat';
import { setInitialChatVariableEnabledFieldValue } from '@/utils/chat';
import styles from './index.less';

const ModelSetting = ({
  show,
  form,
  initialLlmSetting,
  visible,
}: ISegmentedContentProps & {
  initialLlmSetting?: Variable;
  visible?: boolean;
}) => {
  useEffect(() => {
    if (visible) {
      const values = Object.keys(variableEnabledFieldMap).reduce<
        Record<string, boolean>
      >((pre, field) => {
        pre[field] =
          initialLlmSetting === undefined
            ? setInitialChatVariableEnabledFieldValue(
                field as ChatVariableEnabledField,
              )
            : !!initialLlmSetting[
                variableEnabledFieldMap[
                  field as keyof typeof variableEnabledFieldMap
                ] as keyof Variable
              ];
        return pre;
      }, {});
      form.setFieldsValue(values);
    }
  }, [form, initialLlmSetting, visible]);

  return (
    <section
      className={classNames({
        [styles.segmentedHidden]: !show,
      })}
    >
      {visible && <LlmSettingItems prefix="llm_setting"></LlmSettingItems>}
    </section>
  );
};

export default ModelSetting;
