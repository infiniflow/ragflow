import reactStringReplace from 'react-string-replace';

// Shared [ERROR] highlighter for document progress logs. Used by the file
// list popover and the process log modal so both render the same red spans.
export const replaceLogText = (text: string) => {
  // Remove duplicate \n
  const nextText = text.replace(/(\n)\1+/g, '$1');

  const replacedText = reactStringReplace(
    nextText,
    /(\[ERROR\].+\s)/g,
    (match, i) => {
      return (
        <span key={i} className={'text-red-600'}>
          {match}
        </span>
      );
    },
  );

  return replacedText;
};
