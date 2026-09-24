import { type Node } from '@xyflow/react';
import { useMemo } from 'react';
import { DocGeneratorOutputFormat } from '../../constant/doc-generator';
import { initialDocGeneratorValues } from '../../constant';

export const useValues = (node?: Node) => {
  const values = useMemo(() => {
    const nextValues = {
      ...initialDocGeneratorValues,
      ...(node?.data.form ?? {}),
    };

    return {
      ...nextValues,
      output_format: (
        Object.values(DocGeneratorOutputFormat) as string[]
      ).includes(nextValues.output_format)
        ? nextValues.output_format
        : initialDocGeneratorValues.output_format,
      include_download_info_in_content:
        nextValues.include_download_info_in_content ?? false,
      font_size: Math.max(12, Number(nextValues.font_size) || 12),
      outputs: initialDocGeneratorValues.outputs,
    };
  }, [node?.data.form]);

  return values;
};
