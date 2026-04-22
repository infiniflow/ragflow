import SvgIcon from '@/components/svg-icon';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card } from '@/components/ui/card';
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip';
import { Eye, Tag, Trash2 } from 'lucide-react';
import React, { memo } from 'react';
import type { Skill } from '../types';

interface SkillCardProps {
  skill: Skill;
  onView: (skill: Skill) => void;
  onDelete: (skillId: string, skillName: string, folderId?: string) => void;
  formatRelative: (timestamp: number) => string;
}

const SkillCard: React.FC<SkillCardProps> = ({
  skill,
  onView,
  onDelete,
  formatRelative,
}) => {
  const fileCount = skill.files.filter((f) => !f.is_dir).length;
  const dirCount = skill.files.filter((f) => f.is_dir).length;
  const filesLoading = skill.files.length === 0 && (skill as any)._folderId;

  return (
    <TooltipProvider>
      <Card
        className="cursor-pointer hover:shadow-md transition-all bg-bg-card border border-border rounded-xl p-4"
        onClick={() => onView(skill)}
      >
        <div className="flex gap-4">
          <div className="flex-shrink-0 mt-1">
            <SvgIcon name="home-icon/skill-folder" width={24} height={24} />
          </div>

          <div className="flex-1 min-w-0">
            <div className="flex justify-between items-start">
              <h5 className="font-semibold text-base m-0 mb-2 truncate pr-2">
                {skill.name}
              </h5>

              <div
                className="flex items-center gap-1"
                onClick={(e) => e.stopPropagation()}
              >
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon-xs"
                      onClick={(e: React.MouseEvent) => {
                        e.stopPropagation();
                        onView(skill);
                      }}
                    >
                      <Eye className="size-4" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>View</TooltipContent>
                </Tooltip>

                <AlertDialog>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <AlertDialogTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon-xs"
                          onClick={(e: React.MouseEvent) => e.stopPropagation()}
                        >
                          <Trash2 className="size-4 text-state-error" />
                        </Button>
                      </AlertDialogTrigger>
                    </TooltipTrigger>
                    <TooltipContent>Delete</TooltipContent>
                  </Tooltip>
                  <AlertDialogContent>
                    <AlertDialogHeader>
                      <AlertDialogTitle>Delete Skill</AlertDialogTitle>
                      <AlertDialogDescription>
                        Are you sure you want to delete this skill? This action
                        cannot be undone.
                      </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                      <AlertDialogCancel>Cancel</AlertDialogCancel>
                      <AlertDialogAction
                        onClick={() =>
                          onDelete(
                            skill.id,
                            skill.name,
                            (skill as any)._folderId,
                          )
                        }
                        className="bg-state-error hover:bg-state-error/90"
                      >
                        Delete
                      </AlertDialogAction>
                    </AlertDialogFooter>
                  </AlertDialogContent>
                </AlertDialog>
              </div>
            </div>

            {skill.description && (
              <p className="text-text-secondary text-sm mb-3 line-clamp-2">
                {skill.description}
              </p>
            )}

            <div className="flex flex-wrap gap-1 mb-2">
              {skill.metadata?.tags?.slice(0, 4).map((tag) => (
                <Badge key={tag} variant="secondary">
                  {tag}
                </Badge>
              ))}
              {skill.metadata?.tags && skill.metadata.tags.length > 4 && (
                <Badge variant="secondary">
                  +{skill.metadata.tags.length - 4}
                </Badge>
              )}
            </div>

            <div className="flex justify-between items-center mt-2">
              <span className="text-text-secondary text-xs">
                {filesLoading
                  ? '...'
                  : fileCount > 0
                    ? `${fileCount} files`
                    : ''}
              </span>

              <div className="flex items-center gap-2">
                {skill.metadata?.version && (
                  <Badge variant="outline" className="text-xs">
                    <Tag className="size-3 mr-1" />v{skill.metadata.version}
                  </Badge>
                )}
                <span className="text-text-secondary text-xs">
                  {formatRelative(skill.updated_at)}
                </span>
              </div>
            </div>
          </div>
        </div>
      </Card>
    </TooltipProvider>
  );
};

export default memo(SkillCard);
