export interface CreateDirectoryFormValues {
  name: string;
  rule: string;
}

export type CommitFormValues = {
  comments: string;
};

export type WikiDiffLineType = 'added' | 'removed' | 'context' | 'header';

export type WikiDiffLine = {
  type: WikiDiffLineType;
  content: string;
};

export type WikiDiffHunk = {
  header: string;
  lines: WikiDiffLine[];
};
