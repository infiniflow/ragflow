/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

import { useEffect, useRef } from 'react';

// File selection belongs to the folder that was open when the user checked
// the rows. Folder navigation usually stays on the same page, so watching
// the folder id (including root, which is '') covers every path that changes
// it: folder clicks, breadcrumbs, and the missing-folder redirect.
export function useClearSelectionOnFolderChange(
  folderId: string,
  clearSelection: () => void,
) {
  const displayedFolderIdRef = useRef(folderId);

  useEffect(() => {
    if (displayedFolderIdRef.current === folderId) {
      return;
    }

    displayedFolderIdRef.current = folderId;
    clearSelection();
  }, [folderId, clearSelection]);
}
