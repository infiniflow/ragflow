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

import { cn } from '@/lib/utils';
import { t } from 'i18next';
import { ArrowBigLeft } from 'lucide-react';
import React from 'react';
import { useNavigate } from 'react-router';
import { Button } from '../ui/button';

interface BackButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  to?: string;
}

const BackButton: React.FC<BackButtonProps> = ({
  to,
  className,
  children,
  ...props
}) => {
  const navigate = useNavigate();

  const handleClick = () => {
    if (to) {
      navigate(to);
    } else {
      navigate(-1);
    }
  };

  return (
    <Button
      variant="ghost"
      className={cn(
        'gap-2 bg-bg-card border border-border-default hover:bg-border-button hover:text-text-primary',
        className,
      )}
      onClick={handleClick}
      {...props}
    >
      <ArrowBigLeft className="h-4 w-4" />
      {children || t('common.back')}
    </Button>
  );
};

export default BackButton;
