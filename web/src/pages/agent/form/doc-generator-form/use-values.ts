import { useMemo } from 'react';
import { Node } from 'reactflow';
import { initialDocGeneratorValues } from '../../constant';

export const useValues = (node?: Node) => {
  const values = useMemo(() => {
    const supportedOutputFormats = ['pdf', 'docx', 'txt', 'markdown', 'html'];
    const nextValues = {
      ...initialDocGeneratorValues,
      ...(node?.data.form ?? {}),
    };

    return {
      output_format: supportedOutputFormats.includes(nextValues.output_format)
        ? nextValues.output_format
        : initialDocGeneratorValues.output_format,
      content: nextValues.content,
      filename: nextValues.filename,
      header_text: nextValues.header_text,
      footer_text: nextValues.footer_text,
      watermark_text: nextValues.watermark_text,
      add_page_numbers: nextValues.add_page_numbers,
      add_timestamp: nextValues.add_timestamp,
      font_size: Math.max(12, Number(nextValues.font_size) || 12),
      outputs: initialDocGeneratorValues.outputs,
    };
  }, [node?.data.form]);

  return values;
};
