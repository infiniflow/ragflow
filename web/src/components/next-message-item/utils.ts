import { currentReg, parseCitationIndex } from '@/utils/chat';

export const extractNumbersFromMessageContent = (content: string) => {
  const matches = content.match(currentReg);
  if (matches) {
    const list = matches
      .map((match) => {
        const parsed = parseCitationIndex(match);
        return Number.isNaN(parsed) ? null : parsed;
      })
      .filter((num) => num !== null) as number[];

    return list;
  }
  return [];
};
