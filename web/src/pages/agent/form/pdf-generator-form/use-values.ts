import { useMemo } from 'react';
import { Node } from 'reactflow';
import { initialPDFGeneratorValues } from '../../constant';

export const useValues = (node?: Node) => {
  const values = useMemo(() => {
    const supportedOutputFormats = ['pdf', 'docx', 'txt', 'markdown', 'html'];
    const nextValues = {
      ...initialPDFGeneratorValues,
      ...(node?.data.form ?? {}),
    };

    return {
      output_format: supportedOutputFormats.includes(nextValues.output_format)
        ? nextValues.output_format
        : initialPDFGeneratorValues.output_format,
      content: nextValues.content,
      filename: nextValues.filename,
      header_text: nextValues.header_text,
      footer_text: nextValues.footer_text,
      watermark_text: nextValues.watermark_text,
      add_page_numbers: nextValues.add_page_numbers,
      add_timestamp: nextValues.add_timestamp,
      font_size: nextValues.font_size,
      outputs: initialPDFGeneratorValues.outputs,
    };
  }, [node?.data.form]);

  return values;
};
