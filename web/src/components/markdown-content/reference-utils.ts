import { IReference } from '@/interfaces/database/chat';
import { currentReg, showImage } from '@/utils/chat';

export interface ReferenceMatch {
  id: string;
  fullMatch: string;
  start: number;
  end: number;
}

export type ReferenceGroup = ReferenceMatch[];

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
  // Construct a two-dimensional array to distinguish whether images are continuous.
  const groups: ReferenceGroup[] = [];

  if (matches.length === 0) return groups;

  let currentGroup: ReferenceGroup = [matches[0]];
  // A group with only one element contains non-contiguous images,
  // while a group with multiple elements contains contiguous images.
  for (let i = 1; i < matches.length; i++) {
    // If the end of the previous element equals the start of the current element,
    // it means that they are consecutive images.
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
