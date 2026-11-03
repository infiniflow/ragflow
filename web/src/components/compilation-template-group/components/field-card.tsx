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

import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Pencil, Trash2 } from 'lucide-react';
import { useCallback } from 'react';

type FieldCardProps = {
  index: number;
  title?: string;
  description?: string;
  onEdit: (index: number) => void;
  onDelete: (index: number) => void;
};

export function FieldCard({
  index,
  title,
  description,
  onEdit,
  onDelete,
}: FieldCardProps) {
  const handleEdit = useCallback(() => {
    onEdit(index);
  }, [onEdit, index]);

  const handleDelete = useCallback(() => {
    onDelete(index);
  }, [onDelete, index]);

  return (
    <Card className="border-border-button bg-transparent group">
      <CardContent className="p-4 space-y-3">
        <div className="flex items-start justify-between gap-2">
          <div className="space-y-2 space-x-2 flex-1 min-w-0">
            {title && (
              <span className="line-clamp-1 text-sm font-medium text-text-primary">
                {title}
              </span>
            )}
          </div>
          <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity shrink-0">
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              onClick={handleEdit}
              className="text-text-secondary hover:text-text-primary"
            >
              <Pencil className="size-4" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              onClick={handleDelete}
              className="text-text-secondary hover:text-state-error"
            >
              <Trash2 className="size-4" />
            </Button>
          </div>
        </div>

        {description && (
          <p className="text-sm text-text-secondary line-clamp-3">
            {description}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
