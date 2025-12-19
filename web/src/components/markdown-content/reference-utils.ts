import { IReference } from '@/interfaces/database/chat';
import { currentReg, showImage } from '@/utils/chat';

/**
 * Reference match data structure
 */
export interface ReferenceMatch {
  id: string;
  fullMatch: string;
  start: number;
  end: number;
}

/**
 * Grouped reference matches
 */
export type ReferenceGroup = ReferenceMatch[];

/**
 * Helper to find all reference matches with their positions
 */
export const findAllReferenceMatches = (text: string): ReferenceMatch[] => {
  const matches: ReferenceMatch[] = [];
  let match;
  while ((match = currentReg.exec(text)) !== null) {
    matches.push({
      id: match[1],
      fullMatch: match[0],
      start: match.index,
      end: match.index + match[0].length,
    });
  }
  return matches;
};

/**
 * Helper to group consecutive references
 */
export const groupConsecutiveReferences = (text: string): ReferenceGroup[] => {
  const matches = findAllReferenceMatches(text);
  const groups: ReferenceGroup[] = [];

  if (matches.length === 0) return groups;

  let currentGroup: ReferenceGroup = [matches[0]];

  for (let i = 1; i < matches.length; i++) {
    // If this match starts right after the previous one ended
    if (matches[i].start === currentGroup[currentGroup.length - 1].end) {
      currentGroup.push(matches[i]);
    } else {
      // Save current group and start a new one
      groups.push(currentGroup);
      currentGroup = [matches[i]];
    }
  }
  groups.push(currentGroup);

  return groups;
};

/**
 * Helper to check if all references in a group are images
 */
export const shouldShowCarousel = (
  group: ReferenceGroup,
  reference: IReference,
): boolean => {
  if (group.length < 2) return false; // Need at least 2 images for carousel

  return group.every((ref) => {
    const chunkIndex = Number(ref.id);
    const chunk = reference.chunks[chunkIndex];
    return chunk && showImage(chunk.doc_type);
  });
};
