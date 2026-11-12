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

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type PropsWithChildren,
} from 'react';

interface CompilationTemplateSidebarCollapseContextValue {
  collapsed: boolean;
  setCollapsed: (collapsed: boolean) => void;
  toggleCollapsed: () => void;
}

const CompilationTemplateSidebarCollapseContext =
  createContext<CompilationTemplateSidebarCollapseContextValue | null>(null);

// Shared by the compilation template editor and the primary AppSidebar, which
// hosts the expand entry while the secondary sidebar is hidden.
export function CompilationTemplateSidebarCollapseProvider({
  children,
}: PropsWithChildren) {
  const [collapsed, setCollapsed] = useState(false);

  const toggleCollapsed = useCallback(() => {
    setCollapsed((prev) => !prev);
  }, [setCollapsed]);

  const value = useMemo(
    () => ({ collapsed, setCollapsed, toggleCollapsed }),
    [collapsed, setCollapsed, toggleCollapsed],
  );

  return (
    <CompilationTemplateSidebarCollapseContext.Provider value={value}>
      {children}
    </CompilationTemplateSidebarCollapseContext.Provider>
  );
}

export function useCompilationTemplateSidebarCollapse() {
  const context = useContext(CompilationTemplateSidebarCollapseContext);
  if (!context) {
    throw new Error(
      'useCompilationTemplateSidebarCollapse must be used within CompilationTemplateSidebarCollapseProvider',
    );
  }
  return context;
}
