import { useCallback, useEffect, useState } from 'react';

type UseWikiEditorParams = {
  content: string;
  artifactSlug?: string;
};

export function useWikiEditor({ content, artifactSlug }: UseWikiEditorParams) {
  const [editedContent, setEditedContent] = useState(content);
  const [isDirty, setIsDirty] = useState(false);

  useEffect(() => {
    setEditedContent(content);
    setIsDirty(false);
  }, [content, artifactSlug]);

  const handleContentChange = useCallback(
    (value: string) => {
      setEditedContent(value);
      setIsDirty(value !== content);
    },
    [content],
  );

  const handleCancelEdit = useCallback(() => {
    setEditedContent(content);
    setIsDirty(false);
  }, [content]);

  const handleMarkAsSaved = useCallback(() => {
    setIsDirty(false);
  }, []);

  return {
    editedContent,
    isDirty,
    handleContentChange,
    handleCancelEdit,
    handleMarkAsSaved,
  };
}
