import { currentReg } from '@/utils/chat';

export const extractNumbersFromMessageContent = (content: string) => {
  const matches = content.match(currentReg);
  if (matches) {
    const list = matches
      .map((match) => {
        const numMatch = match.match(/\[ID:(\d+)\]/);
        return numMatch ? parseInt(numMatch[1], 10) : null;
      })
      .filter((num) => num !== null) as number[];

    return list;
  }
  return [];
};
